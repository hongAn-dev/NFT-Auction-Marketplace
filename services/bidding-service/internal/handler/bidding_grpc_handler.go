package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"bidding-service/internal/auction"
	"bidding-service/internal/domain"
	"bidding-service/internal/settlement"
	pb "bidding-service/proto"
)

// SettlementReader: saga của một lot (nil nếu lot chưa đóng phiên).
type SettlementReader interface {
	SettlementOfLot(ctx context.Context, lotID string) (*domain.Settlement, error)
}

// BiddingGRPCHandler: gRPC cho api-gateway (docs/23 §5.2, §6.1).
type BiddingGRPCHandler struct {
	pb.UnimplementedBiddingServiceServer
	Bidder      *auction.Bidder
	Engine      *settlement.Engine
	Settlements SettlementReader
	Lock        auction.Locker
	// Invalidate: xoá cache Redis của lot sau khi đặt giá (nil = bỏ qua).
	Invalidate func(ctx context.Context, lotID string)
}

// PlaceBid: đặt giá có ký quỹ. Lý do nghiệp vụ (hết giờ, thiếu tiền…) trả
// success=false kèm message; chỉ lỗi hệ thống mới là lỗi gRPC.
func (h *BiddingGRPCHandler) PlaceBid(ctx context.Context, req *pb.PlaceBidRequest) (*pb.PlaceBidResponse, error) {
	key := auction.LockKey(req.NftId)
	ok, err := h.Lock.AcquireLock(ctx, key, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("khoá phân tán Redis: %w", err)
	}
	if !ok {
		return &pb.PlaceBidResponse{Success: false,
			Message: "Hệ thống đang xử lý lượt đặt giá khác cho món đồ này, vui lòng thử lại."}, nil
	}
	defer func() { _ = h.Lock.ReleaseLock(context.WithoutCancel(ctx), key) }()

	bid, err := h.Bidder.Place(ctx, req.NftId, req.UserId, req.Amount)
	var rej *auction.Rejected
	if errors.As(err, &rej) {
		return &pb.PlaceBidResponse{Success: false, Message: rej.Reason}, nil
	}
	if err != nil {
		slog.Error("đặt giá lỗi", "nft_id", req.NftId, "user_id", req.UserId, "error", err)
		return nil, err
	}
	if h.Invalidate != nil {
		h.Invalidate(ctx, req.NftId)
	}
	return &pb.PlaceBidResponse{Success: true, Message: "Đặt giá thành công, tiền đã được giữ ký quỹ.",
		Bid: &pb.Bid{Id: bid.ID, NftId: bid.NFTID, UserId: bid.UserID, Amount: float64(bid.Amount), CreatedAt: bid.CreatedAt.Unix()}}, nil
}

// GetHighestBid: lượt đặt giá cao nhất còn giữ tiền; chưa có thì user_id rỗng và
// amount là giá khởi điểm.
func (h *BiddingGRPCHandler) GetHighestBid(ctx context.Context, req *pb.GetHighestBidRequest) (*pb.GetHighestBidResponse, error) {
	lot, err := h.Bidder.Store.Lot(ctx, req.NftId)
	if errors.Is(err, domain.ErrNFTNotFound) {
		return &pb.GetHighestBidResponse{Success: false}, nil
	}
	if err != nil {
		return nil, err
	}
	out := &pb.Bid{NftId: lot.ID, Amount: lot.CurrentPrice, CreatedAt: lot.UpdatedAt.Unix()}
	if b, err := h.Bidder.Store.Highest(ctx, req.NftId); err == nil && b != nil {
		out.Id, out.UserId, out.Amount, out.CreatedAt = b.ID, b.UserID, float64(b.Amount), b.CreatedAt.Unix()
	}
	return &pb.GetHighestBidResponse{Success: true, HighestBid: out}, nil
}

// ConfirmReceived: người thắng bấm "Đã nhận hàng" (docs/23 §6.1).
func (h *BiddingGRPCHandler) ConfirmReceived(ctx context.Context, req *pb.ConfirmReceivedRequest) (*pb.ConfirmReceivedResponse, error) {
	s, err := h.Settlements.SettlementOfLot(ctx, req.NftId)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return &pb.ConfirmReceivedResponse{Success: false, Message: "Phiên đấu giá chưa đóng."}, nil
	}
	switch err := h.Engine.ConfirmReceived(ctx, s.ID, req.UserId); {
	case errors.Is(err, settlement.ErrNotWinner), errors.Is(err, settlement.ErrWrongState):
		return &pb.ConfirmReceivedResponse{Success: false, Message: err.Error(), State: s.State}, nil
	case err != nil:
		return nil, err
	}
	now, err := h.Engine.Store.Get(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	return &pb.ConfirmReceivedResponse{Success: true, Message: "Đã ghi nhận. Sàn sẽ hoàn tất vụ bán trên chuỗi rồi trả tiền cho người bán.",
		State: now.State}, nil
}
