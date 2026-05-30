package handler

import (
	"context"
	"fmt"
	"log"
	"time"

	"bidding-service/internal/domain"
	"bidding-service/internal/repository"
	pb "bidding-service/proto"
)

// BiddingGRPCHandler implements the pb.BiddingServiceServer interface.
type BiddingGRPCHandler struct {
	pb.UnimplementedBiddingServiceServer
	nftUsecase domain.NFTUsecase
	redisRepo  *repository.RedisRepository
	rabbitRepo *repository.RabbitMQRepository
}

// NewBiddingGRPCHandler khởi tạo gRPC Handler với Postgres, Redis và RabbitMQ.
func NewBiddingGRPCHandler(
	nftUc domain.NFTUsecase,
	redisRepo *repository.RedisRepository,
	rabbitRepo *repository.RabbitMQRepository,
) *BiddingGRPCHandler {
	return &BiddingGRPCHandler{
		nftUsecase: nftUc,
		redisRepo:  redisRepo,
		rabbitRepo: rabbitRepo,
	}
}

// PlaceBid xử lý đặt thầu và phát sự kiện bất đồng bộ lên RabbitMQ
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

	// 3. Cập nhật giá thầu cao nhất mới và chủ sở hữu danh nghĩa mới vào Postgres
	nft.CurrentPrice = req.Amount
	nft.OwnerID = req.UserId
	_, err = h.nftUsecase.UpdateNFT(ctx, req.NftId, nft)
	if err != nil {
		return nil, fmt.Errorf("không thể cập nhật giá thầu mới vào DB: %w", err)
	}

	// 4. Tạo đối tượng Bid phản hồi
	grpcBid := &pb.Bid{
		Id:        fmt.Sprintf("bid-%d", time.Now().UnixNano()),
		NftId:     req.NftId,
		UserId:    req.UserId,
		Amount:    req.Amount,
		CreatedAt: time.Now().Unix(),
	}

	fmt.Printf("✅ gRPC: Đặt thầu THÀNH CÔNG cho NFT %s. Giá mới: %.2f\n", req.NftId, req.Amount)

	// ── 4.5 PHÁT SỰ KIỆN LÊN REDIS PUB/SUB BRIDGE (REAL-TIME WEBSOCKET) ──
	if h.redisRepo != nil {
		eventPayload := map[string]interface{}{
			"bid_id":     grpcBid.Id,
			"nft_id":     req.NftId,
			"user_id":    req.UserId,
			"amount":     req.Amount,
			"created_at": grpcBid.CreatedAt,
		}
		errPub := h.redisRepo.PublishBidEvent(ctx, req.NftId, eventPayload)
		if errPub != nil {
			log.Printf("⚠️ gRPC: Lỗi phát sự kiện lên Redis Pub/Sub: %v\n", errPub)
		}
	}

	// ── 5. PHÁT SỰ KIỆN BẤT ĐỒNG BỘ LÊN RABBITMQ EXCHANGE (EVENT-DRIVEN) ──
	if h.rabbitRepo != nil {
		eventData := map[string]interface{}{
			"bid_id":     grpcBid.Id,
			"nft_id":     req.NftId,
			"user_id":    req.UserId,
			"amount":     req.Amount,
			"created_at": grpcBid.CreatedAt,
		}

		// A. Luôn phát sự kiện đặt giá thầu: routing key "bid.placed"
		errPub := h.rabbitRepo.PublishEvent(ctx, "bid.placed", eventData)
		if errPub != nil {
			log.Printf("⚠️ gRPC: Lỗi phát sự kiện 'bid.placed': %v\n", errPub)
		}

		// B. Để test cả 2 Worker (Email + Royalty), ta giả lập phát kèm sự kiện
		// giao dịch thành công "nft.purchased" để kích hoạt Royalty Worker tính tiền!
		errPub = h.rabbitRepo.PublishEvent(ctx, "nft.purchased", eventData)
		if errPub != nil {
			log.Printf("⚠️ gRPC: Lỗi phát sự kiện 'nft.purchased': %v\n", errPub)
		}
	}

	return &pb.PlaceBidResponse{
		Success: true,
		Bid:     grpcBid,
		Message: "Đặt giá thầu thành công và các sự kiện đã được phát lên RabbitMQ!",
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
