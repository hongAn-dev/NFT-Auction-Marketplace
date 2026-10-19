package auction

import (
	"context"
	"errors"
	"testing"
	"time"

	"bidding-service/internal/domain"
)

type fakeCloseStore struct {
	due    []string
	closed []string
	err    map[string]error
}

func (f *fakeCloseStore) DueLots(context.Context, time.Time, int) ([]string, error) {
	return f.due, nil
}

func (f *fakeCloseStore) CloseLot(_ context.Context, id string, _ time.Time) (*domain.Settlement, error) {
	if err := f.err[id]; err != nil {
		return nil, err
	}
	f.closed = append(f.closed, id)
	return nil, nil
}

type fakeLocker struct {
	busy     map[string]bool
	released []string
}

func (f *fakeLocker) AcquireLock(_ context.Context, key string, _ time.Duration) (bool, error) {
	return !f.busy[key], nil
}

func (f *fakeLocker) ReleaseLock(_ context.Context, key string) error {
	f.released = append(f.released, key)
	return nil
}

// Đóng phiên lấy CÙNG khoá Redis với PlaceBid: lot đang có người đặt giá thì để lần
// sau; một lot lỗi không chặn các lot khác.
func TestCloserUsesBidLock(t *testing.T) {
	store := &fakeCloseStore{due: []string{"lot-1", "lot-2", "lot-3"}, err: map[string]error{"lot-3": errors.New("db")}}
	lock := &fakeLocker{busy: map[string]bool{"bid:lock:lot-2": true}}
	c := &Closer{Store: store, Lock: lock, Now: func() time.Time { return t0 }}
	c.Tick(context.Background())
	if len(store.closed) != 1 || store.closed[0] != "lot-1" {
		t.Fatalf("chỉ lot-1 được đóng, nhận %v", store.closed)
	}
	if len(lock.released) != 2 {
		t.Fatalf("khoá đã lấy phải được trả (lot-1, lot-3), nhận %v", lock.released)
	}
}

type fakeStale struct {
	stale []string
	bids  map[string]bool
}

func (f *fakeStale) Stale(context.Context, time.Duration) ([]string, error) { return f.stale, nil }
func (f *fakeStale) BidExistsForHold(_ context.Context, id string) (bool, error) {
	return f.bids[id], nil
}

// Đối soát: khoản HELD cũ mà không có dòng bids nào (tiến trình chết giữa giữ tiền
// và ghi DB) thì nhả; khoản có bid thì để yên.
func TestReconcilerReleasesOrphanHolds(t *testing.T) {
	esc := newEscrow(map[string]int64{})
	src := &fakeStale{stale: []string{"bid-orphan", "bid-ok"}, bids: map[string]bool{"bid-ok": true}}
	(&Reconciler{Escrow: esc, Source: src, Bids: src, MinAge: 5 * time.Minute}).Tick(context.Background())
	if len(esc.ops) != 1 || esc.ops[0] != "release bid-orphan" {
		t.Fatalf("chỉ nhả khoản mồ côi, nhận %v", esc.ops)
	}
}
