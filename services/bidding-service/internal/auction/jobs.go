package auction

import (
	"context"
	"log"
	"time"

	"bidding-service/internal/domain"
)

// Locker: khoá phân tán Redis — RedisRepository có sẵn hai hàm này.
type Locker interface {
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
}

// LockKey: khoá dùng chung giữa PlaceBid và đóng phiên.
func LockKey(lotID string) string { return "bid:lock:" + lotID }

// CloseStore: phần DB của đóng phiên.
type CloseStore interface {
	// DueLots: lot active có ends_at <= now.
	DueLots(ctx context.Context, now time.Time, limit int) ([]string, error)
	// CloseLot: MỘT transaction, khoá dòng lot (FOR UPDATE SKIP LOCKED), kiểm lại
	// ends_at; không có lượt đặt giá → unsold; có → settling + dòng settlements
	// PENDING_OPEN + bid thắng thành WON. Trả settlement (nil nếu unsold / đã đóng).
	CloseLot(ctx context.Context, lotID string, now time.Time) (*domain.Settlement, error)
}

// Closer: job đóng phiên mỗi vài giây (docs/23 §5.3).
type Closer struct {
	Store CloseStore
	Lock  Locker
	Now   func() time.Time
}

func (c *Closer) Tick(ctx context.Context) {
	now := c.Now()
	ids, err := c.Store.DueLots(ctx, now, 50)
	if err != nil {
		log.Printf("⚠️ đóng phiên: đọc lot tới hạn: %v", err)
		return
	}
	for _, id := range ids {
		ok, err := c.Lock.AcquireLock(ctx, LockKey(id), 10*time.Second)
		if err != nil || !ok {
			continue // đang có người đặt giá — lần sau
		}
		s, err := c.Store.CloseLot(ctx, id, now)
		_ = c.Lock.ReleaseLock(ctx, LockKey(id))
		switch {
		case err != nil:
			log.Printf("⚠️ đóng phiên %s: %v", id, err)
		case s != nil:
			log.Printf("🔨 đóng phiên %s: người thắng %s, %d VND, settlement %s", id, s.WinnerID, s.Amount, s.ID)
		}
	}
}

// StaleHolds: auth-service liệt kê khoản HELD cũ hơn minAge.
type StaleHolds interface {
	Stale(ctx context.Context, minAge time.Duration) ([]string, error)
}

// BidIndex: khoản giữ có dòng bids đi kèm hay không.
type BidIndex interface {
	BidExistsForHold(ctx context.Context, holdID string) (bool, error)
}

// Reconciler: nhả khoản giữ mồ côi (docs/23 §5.2 bước 4).
type Reconciler struct {
	Escrow Escrow
	Source StaleHolds
	Bids   BidIndex
	MinAge time.Duration
}

func (r *Reconciler) Tick(ctx context.Context) {
	ids, err := r.Source.Stale(ctx, r.MinAge)
	if err != nil {
		log.Printf("⚠️ đối soát ký quỹ: %v", err)
		return
	}
	for _, id := range ids {
		has, err := r.Bids.BidExistsForHold(ctx, id)
		if err != nil || has {
			continue
		}
		if err := r.Escrow.Release(ctx, id); err != nil {
			log.Printf("⚠️ đối soát: nhả %s: %v", id, err)
		}
	}
}
