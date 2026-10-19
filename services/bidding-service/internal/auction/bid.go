// Package auction: phiên đấu giá của sàn mô phỏng — đặt giá có ký quỹ, đóng phiên,
// đối soát khoản giữ (docs/23 §5). Logic ở đây không chạm DB hay HTTP trực tiếp:
// mọi thứ đi qua các interface để test được bằng fake.
package auction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"bidding-service/internal/domain"
)

// Escrow: ký quỹ ở auth-service (docs/23 §4). Mọi lệnh idempotent theo holdID.
type Escrow interface {
	Hold(ctx context.Context, holdID, userID string, amount int64) error
	Release(ctx context.Context, holdID string) error
}

// ErrInsufficientFunds: auth-service trả 402.
var ErrInsufficientFunds = errors.New("số dư khả dụng không đủ")

// Rejected: lượt đặt giá bị từ chối vì lý do nghiệp vụ — báo lại cho người dùng,
// không phải lỗi hệ thống.
type Rejected struct {
	Code   string // mã máy đọc, ví dụ SERIAL_OWNED_BY_OTHER; rỗng = INVALID_INPUT
	Reason string
}

func (r *Rejected) Error() string { return r.Reason }

func reject(format string, a ...any) error { return &Rejected{Reason: fmt.Sprintf(format, a...)} }

// TopicEscrowRelease: outbox nhả khoản giữ của người bị vượt giá. Outbox publisher
// gọi thẳng auth-service cho topic này (không qua RabbitMQ).
const TopicEscrowRelease = "escrow.release"

// BidStore: phần DB của đặt giá.
type BidStore interface {
	Lot(ctx context.Context, id string) (*domain.NFT, error)
	// Highest: lượt HELD cao nhất hiện tại, nil nếu chưa có.
	Highest(ctx context.Context, lotID string) (*domain.Bid, error)
	// CommitBid: MỘT transaction — cập nhật lot, thêm bid, đặt bid prevBidID thành
	// RELEASED (nếu có), ghi outbox.
	CommitBid(ctx context.Context, lot *domain.NFT, bid *domain.Bid, prevBidID string, events []domain.OutboxEvent) error
	// BidExistsForHold: lượt đặt giá của khoản giữ đã nằm trong DB chưa.
	BidExistsForHold(ctx context.Context, holdID string) (bool, error)
}

type Bidder struct {
	Store  BidStore
	Escrow Escrow
	Now    func() time.Time
	NewID  func() string
}

// maxAmount: 10^15 VND — còn xa giới hạn số nguyên chính xác của float64.
const maxAmount = 1e15

// Place đặt giá (docs/23 §5.2). Phía gọi giữ khoá Redis bid:lock:<lotId>.
func (b *Bidder) Place(ctx context.Context, lotID, userID string, amount float64) (*domain.Bid, error) {
	lot, err := b.Store.Lot(ctx, lotID)
	if errors.Is(err, domain.ErrNFTNotFound) {
		return nil, reject("không tìm thấy món đồ %s", lotID)
	}
	if err != nil {
		return nil, err
	}
	now := b.Now()
	switch {
	case lot.EndsAt == nil:
		return nil, reject("món đồ %s không mở đấu giá", lotID)
	case lot.Status != domain.LotActive:
		return nil, reject("phiên đấu giá %s không còn mở (%s)", lotID, lot.Status)
	case !now.Before(*lot.EndsAt):
		return nil, reject("phiên đấu giá %s đã kết thúc", lotID)
	case userID == "" || userID == lot.OwnerID:
		return nil, reject("người bán không được tự đặt giá món đồ của mình")
	case amount != math.Trunc(amount) || amount <= 0 || amount > maxAmount:
		return nil, reject("giá đặt phải là số nguyên VND dương")
	case amount <= lot.CurrentPrice:
		return nil, reject("giá đặt %.0f phải lớn hơn giá hiện tại %.0f", amount, lot.CurrentPrice)
	}

	bid := &domain.Bid{ID: b.NewID(), NFTID: lotID, UserID: userID, Amount: int64(amount),
		Status: domain.BidHeld, CreatedAt: now, UpdatedAt: now}
	bid.HoldID = bid.ID
	if err := b.Escrow.Hold(ctx, bid.HoldID, userID, bid.Amount); err != nil {
		if errors.Is(err, ErrInsufficientFunds) {
			return nil, reject("số dư khả dụng không đủ để giữ %d VND", bid.Amount)
		}
		return nil, fmt.Errorf("giữ ký quỹ: %w", err)
	}

	prev, err := b.Store.Highest(ctx, lotID)
	if err != nil {
		b.undo(ctx, bid.HoldID)
		return nil, err
	}
	events, err := bidEvents(bid, prev)
	if err != nil {
		b.undo(ctx, bid.HoldID)
		return nil, err
	}
	lot.CurrentPrice, lot.HighestBidID, lot.UpdatedAt = amount, bid.ID, now
	prevID := ""
	if prev != nil {
		prevID = prev.ID
	}
	if err := b.Store.CommitBid(ctx, lot, bid, prevID, events); err != nil {
		b.undo(ctx, bid.HoldID)
		return nil, fmt.Errorf("ghi lượt đặt giá: %w", err)
	}
	return bid, nil
}

// undo nhả khoản vừa giữ khi không ghi được lượt đặt giá. Lỗi có thể mập mờ (DB
// đã COMMIT nhưng phản hồi mất), nên kiểm lại trước: khoản có dòng bids thì giữ
// nguyên. Không kiểm được, hoặc nhả hỏng, thì để job đối soát xử lý — nó chỉ nhả
// khoản KHÔNG có dòng bids nào.
func (b *Bidder) undo(ctx context.Context, holdID string) {
	ctx = context.WithoutCancel(ctx)
	if exists, err := b.Store.BidExistsForHold(ctx, holdID); err != nil || exists {
		return
	}
	if err := b.Escrow.Release(ctx, holdID); err != nil {
		log.Printf("⚠️ nhả khoản giữ %s thất bại, để job đối soát xử lý: %v", holdID, err)
	}
}

func bidEvents(bid *domain.Bid, prev *domain.Bid) ([]domain.OutboxEvent, error) {
	placed, err := json.Marshal(map[string]any{"bid_id": bid.ID, "nft_id": bid.NFTID, "user_id": bid.UserID,
		"amount": bid.Amount, "created_at": bid.CreatedAt.Unix()})
	if err != nil {
		return nil, err
	}
	ev := []domain.OutboxEvent{{Topic: "bid.placed", Payload: placed}, {Topic: "redis:bid:updated", Payload: placed}}
	if prev != nil {
		rel, _ := json.Marshal(map[string]string{"hold_id": prev.HoldID, "bid_id": prev.ID})
		ev = append(ev, domain.OutboxEvent{Topic: TopicEscrowRelease, Payload: rel})
	}
	return ev, nil
}
