package handler

import (
	"context"
	"testing"
	"time"

	"bidding-service/internal/auction"
	"bidding-service/internal/domain"
	"bidding-service/internal/settlement"
	pb "bidding-service/proto"
)

var t0 = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

type lockAll struct{ busy bool }

func (l *lockAll) AcquireLock(context.Context, string, time.Duration) (bool, error) {
	return !l.busy, nil
}
func (l *lockAll) ReleaseLock(context.Context, string) error { return nil }

type oneLot struct {
	lot *domain.NFT
	stl *domain.Settlement
}

func (o *oneLot) Lot(context.Context, string) (*domain.NFT, error)     { cp := *o.lot; return &cp, nil }
func (o *oneLot) Highest(context.Context, string) (*domain.Bid, error) { return nil, nil }
func (o *oneLot) CommitBid(context.Context, *domain.NFT, *domain.Bid, string, []domain.OutboxEvent) error {
	return nil
}
func (o *oneLot) BidExistsForHold(context.Context, string) (bool, error) { return false, nil }
func (o *oneLot) SettlementOfLot(context.Context, string) (*domain.Settlement, error) {
	return o.stl, nil
}

// settlement.Store tối thiểu cho ConfirmReceived.
func (o *oneLot) Due(context.Context, time.Time, int) ([]*domain.Settlement, error) { return nil, nil }
func (o *oneLot) Get(context.Context, string) (*domain.Settlement, error) {
	cp := *o.stl
	return &cp, nil
}
func (o *oneLot) Save(_ context.Context, s *domain.Settlement, from string) (bool, error) {
	if o.stl.State != from {
		return false, nil
	}
	cp := *s
	o.stl = &cp
	return true, nil
}
func (o *oneLot) Finish(context.Context, *domain.Settlement, string, string, string, []domain.OutboxEvent) (bool, error) {
	return false, nil
}

type okEscrow struct{ holds []string }

func (e *okEscrow) Hold(_ context.Context, id, _ string, _ int64) error {
	e.holds = append(e.holds, id)
	return nil
}
func (e *okEscrow) Release(context.Context, string) error { return nil }

func newHandler(store *oneLot, lock *lockAll, esc *okEscrow) *BiddingGRPCHandler {
	now := func() time.Time { return t0 }
	return &BiddingGRPCHandler{
		Bidder:      &auction.Bidder{Store: store, Escrow: esc, Now: now, NewID: func() string { return "bid-1" }},
		Engine:      &settlement.Engine{Store: store, Now: now},
		Settlements: store,
		Lock:        lock,
	}
}

func lotEnding(d time.Duration) *domain.NFT {
	end := t0.Add(d)
	return &domain.NFT{ID: "lot-1", OwnerID: "seller", CurrentPrice: 1e6, Status: domain.LotActive, EndsAt: &end}
}

func TestPlaceBidOutcomes(t *testing.T) {
	ctx := context.Background()
	esc := &okEscrow{}
	h := newHandler(&oneLot{lot: lotEnding(time.Hour)}, &lockAll{}, esc)
	res, err := h.PlaceBid(ctx, &pb.PlaceBidRequest{NftId: "lot-1", UserId: "alice", Amount: 2e6})
	if err != nil || !res.Success || res.Bid.Id != "bid-1" || len(esc.holds) != 1 {
		t.Fatalf("đặt giá hợp lệ phải thành công và giữ tiền: %v %v", res, err)
	}
	res, err = newHandler(&oneLot{lot: lotEnding(-time.Hour)}, &lockAll{}, &okEscrow{}).
		PlaceBid(ctx, &pb.PlaceBidRequest{NftId: "lot-1", UserId: "alice", Amount: 2e6})
	if err != nil || res.Success || res.Message == "" {
		t.Fatalf("phiên đã kết thúc: muốn success=false kèm lý do (không phải lỗi gRPC), nhận %v %v", res, err)
	}
	res, _ = newHandler(&oneLot{lot: lotEnding(time.Hour)}, &lockAll{busy: true}, &okEscrow{}).
		PlaceBid(ctx, &pb.PlaceBidRequest{NftId: "lot-1", UserId: "alice", Amount: 2e6})
	if res.Success {
		t.Fatal("khoá đang bận thì không được đặt giá")
	}
}

func TestConfirmReceived(t *testing.T) {
	ctx := context.Background()
	store := &oneLot{lot: lotEnding(-time.Hour), stl: &domain.Settlement{ID: "stl-1", LotID: "lot-1",
		WinnerID: "winner", State: domain.StlAwaitDelivery}}
	h := newHandler(store, &lockAll{}, &okEscrow{})
	if res, _ := h.ConfirmReceived(ctx, &pb.ConfirmReceivedRequest{NftId: "lot-1", UserId: "nguoi-la"}); res.Success {
		t.Fatal("người khác không được xác nhận")
	}
	res, err := h.ConfirmReceived(ctx, &pb.ConfirmReceivedRequest{NftId: "lot-1", UserId: "winner"})
	if err != nil || !res.Success || res.State != domain.StlCompleting {
		t.Fatalf("người thắng xác nhận phải sang COMPLETING: %v %v", res, err)
	}
	none := newHandler(&oneLot{lot: lotEnding(time.Hour)}, &lockAll{}, &okEscrow{})
	if res, _ := none.ConfirmReceived(ctx, &pb.ConfirmReceivedRequest{NftId: "lot-1", UserId: "winner"}); res.Success {
		t.Fatal("lot chưa đóng phiên thì không có gì để xác nhận")
	}
}
