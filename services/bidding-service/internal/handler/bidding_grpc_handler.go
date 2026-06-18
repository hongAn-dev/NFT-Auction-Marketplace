package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bidding-service/internal/domain"
	"bidding-service/internal/repository"
	pb "bidding-service/proto"
	"gorm.io/gorm"
)

// BiddingGRPCHandler implements the pb.BiddingServiceServer interface.
type BiddingGRPCHandler struct {
	pb.UnimplementedBiddingServiceServer
	nftUsecase domain.NFTUsecase
	redisRepo  *repository.RedisRepository
	rabbitRepo *repository.RabbitMQRepository
	db         *gorm.DB
	outboxRepo domain.OutboxRepository
	sagaRepo   domain.SagaRepository
}

// NewBiddingGRPCHandler khởi tạo gRPC Handler với Postgres, Redis, RabbitMQ và các Outbox/Saga Repositories.
func NewBiddingGRPCHandler(
	nftUc domain.NFTUsecase,
	redisRepo *repository.RedisRepository,
	rabbitRepo *repository.RabbitMQRepository,
	db *gorm.DB,
	outboxRepo domain.OutboxRepository,
	sagaRepo domain.SagaRepository,
) *BiddingGRPCHandler {
	return &BiddingGRPCHandler{
		nftUsecase: nftUc,
		redisRepo:  redisRepo,
		rabbitRepo: rabbitRepo,
		db:         db,
		outboxRepo: outboxRepo,
		sagaRepo:   sagaRepo,
	}
}

// PlaceBid xử lý đặt thầu qua Transactional Outbox và Saga Orchestrator
func (h *BiddingGRPCHandler) PlaceBid(ctx context.Context, req *pb.PlaceBidRequest) (*pb.PlaceBidResponse, error) {
	fmt.Printf("📥 gRPC: Nhận yêu cầu PlaceBid từ User %s cho NFT %s với số tiền: %.2f\n", req.UserId, req.NftId, req.Amount)

	// ── KHÓA PHÂN TÁN REDIS (DISTRIBUTED LOCK) ────────────────────
	lockKey := fmt.Sprintf("bid:lock:%s", req.NftId)

	acquired, err := h.redisRepo.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("lỗi kiểm tra khóa phân tán Redis: %w", err)
	}

	if !acquired {
		fmt.Printf("🔒 gRPC: Bị khóa phân tán chặn đứng! NFT %s đang có người khác đặt thầu cùng lúc.\n", req.NftId)
		return &pb.PlaceBidResponse{
			Success: false,
			Message: "Hệ thống đang bận xử lý lượt đặt thầu khác cho NFT này, vui lòng thử lại sau!",
		}, nil
	}

	defer func() {
		_ = h.redisRepo.ReleaseLock(ctx, lockKey)
		fmt.Printf("🔓 gRPC: Đã tự động giải phóng khóa phân tán cho NFT %s.\n", req.NftId)
	}()

	// 1. Kiểm tra sự tồn tại của NFT
	nft, err := h.nftUsecase.GetNFT(ctx, req.NftId)
	if err != nil {
		if err == domain.ErrNFTNotFound {
			return &pb.PlaceBidResponse{
				Success: false,
				Message: "Không tìm thấy NFT chỉ định",
			}, nil
		}
		return nil, fmt.Errorf("lỗi kiểm tra NFT: %w", err)
	}

	// 2. Validate giá thầu phải lớn hơn giá hiện tại
	if req.Amount <= nft.CurrentPrice {
		return &pb.PlaceBidResponse{
			Success: false,
			Message: fmt.Sprintf("Giá thầu %.2f phải lớn hơn giá hiện tại %.2f", req.Amount, nft.CurrentPrice),
		}, nil
	}

	// Giả lập xử lý trễ 1 giây để kiểm thử khóa phân tán
	time.Sleep(1 * time.Second)

	// Tạo đối tượng Bid phản hồi
	bidID := fmt.Sprintf("bid-%d", time.Now().UnixNano())
	grpcBid := &pb.Bid{
		Id:        bidID,
		NftId:     req.NftId,
		UserId:    req.UserId,
		Amount:    req.Amount,
		CreatedAt: time.Now().Unix(),
	}

	// ── 3. THỰC THI TRANSACTIONAL OUTBOX & SAGA LOG TRONG 1 DB TRANSACTION ──
	errTx := h.db.Transaction(func(tx *gorm.DB) error {
		// Khởi tạo các repositories chạy trên transaction `tx`
		txNFTRepo := repository.NewPostgresNFTRepository(tx)
		txOutboxRepo := repository.NewPostgresOutboxRepository(tx)
		txSagaRepo := repository.NewPostgresSagaRepository(tx)

		// Lưu thông tin trước khi thay đổi để phục vụ Compensating Transaction (Rollback) của Saga
		prevPrice := nft.CurrentPrice
		prevOwner := nft.OwnerID

		// Cập nhật giá và chủ sở hữu mới trong DB
		nft.CurrentPrice = req.Amount
		nft.OwnerID = req.UserId
		nft.UpdatedAt = time.Now()

		if err := txNFTRepo.Update(ctx, nft); err != nil {
			return fmt.Errorf("lỗi cập nhật NFT: %w", err)
		}

		// Lưu Saga Log (Trạng thái STARTED)
		sagaLog := &domain.SagaLog{
			ID:          bidID,
			NFTID:       req.NftId,
			UserID:      req.UserId,
			Amount:      req.Amount,
			PrevPrice:   prevPrice,
			PrevOwnerID: prevOwner,
			State:       domain.SagaStarted,
		}
		if err := txSagaRepo.Create(ctx, sagaLog); err != nil {
			return fmt.Errorf("lỗi tạo Saga log: %w", err)
		}

		// Chuẩn bị payload sự kiện
		eventData := map[string]interface{}{
			"bid_id":     bidID,
			"nft_id":     req.NftId,
			"user_id":    req.UserId,
			"amount":     req.Amount,
			"created_at": grpcBid.CreatedAt,
		}
		payloadBytes, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("lỗi serialize payload: %w", err)
		}

		// Ghi Outbox Event cho RabbitMQ: "bid.placed" (Email Worker)
		outboxPlaced := &domain.OutboxEvent{
			Topic:   "bid.placed",
			Payload: payloadBytes,
		}
		if err := txOutboxRepo.Save(ctx, outboxPlaced); err != nil {
			return fmt.Errorf("lỗi ghi Outbox (bid.placed): %w", err)
		}

		// Ghi Outbox Event cho RabbitMQ: "nft.purchased" (Royalty Worker)
		outboxPurchased := &domain.OutboxEvent{
			Topic:   "nft.purchased",
			Payload: payloadBytes,
		}
		if err := txOutboxRepo.Save(ctx, outboxPurchased); err != nil {
			return fmt.Errorf("lỗi ghi Outbox (nft.purchased): %w", err)
		}

		// Ghi Outbox Event cho Redis Pub/Sub: "redis:bid:updated" (WS Realtime)
		outboxRedis := &domain.OutboxEvent{
			Topic:   "redis:bid:updated",
			Payload: payloadBytes,
		}
		if err := txOutboxRepo.Save(ctx, outboxRedis); err != nil {
			return fmt.Errorf("lỗi ghi Outbox (redis:bid:updated): %w", err)
		}

		return nil
	})

	if errTx != nil {
		return nil, fmt.Errorf("lỗi thực hiện đặt thầu transaction: %w", errTx)
	}

	// Xóa cache cũ trên Redis để tránh client đọc stale data
	if h.redisRepo != nil {
		_ = h.redisRepo.DeleteNFTCache(ctx, req.NftId)
	}

	fmt.Printf("✅ gRPC: Đặt thầu THÀNH CÔNG cho NFT %s (Saga ID %s). Đã lưu DB và tạo Outbox Events.\n", req.NftId, bidID)

	return &pb.PlaceBidResponse{
		Success: true,
		Bid:     grpcBid,
		Message: "Đặt giá thầu thành công! (Transactional Outbox & Saga đã được kích hoạt)",
	}, nil
}

// GetHighestBid lấy thông tin giá thầu cao nhất hiện tại của một NFT qua gRPC
func (h *BiddingGRPCHandler) GetHighestBid(ctx context.Context, req *pb.GetHighestBidRequest) (*pb.GetHighestBidResponse, error) {
	fmt.Printf("📥 gRPC: Nhận yêu cầu GetHighestBid cho NFT %s\n", req.NftId)

	nft, err := h.nftUsecase.GetNFT(ctx, req.NftId)
	if err != nil {
		if err == domain.ErrNFTNotFound {
			return &pb.GetHighestBidResponse{
				Success: false,
			}, nil
		}
		return nil, fmt.Errorf("lỗi truy vấn NFT: %w", err)
	}

	highestBid := &pb.Bid{
		Id:        "bid-current-highest",
		NftId:     nft.ID,
		UserId:    nft.OwnerID,
		Amount:    nft.CurrentPrice,
		CreatedAt: nft.UpdatedAt.Unix(),
	}

	return &pb.GetHighestBidResponse{
		Success:     true,
		HighestBid: highestBid,
	}, nil
}
