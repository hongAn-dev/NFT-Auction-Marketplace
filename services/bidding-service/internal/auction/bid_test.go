package auction

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"bidding-service/internal/domain"
)

var t0 = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

// fakeEscrow: auth-service giả. Ghi lại thao tác, giữ số dư từng người.
type fakeEscrow struct {
	balance  map[string]int64
	held     map[string]int64 // holdID → amount
	owner    map[string]string
	ops      []string
	holdErr  error
	released map[string]bool
}

func newEscrow(balances map[string]int64) *fakeEscrow {
	return &fakeEscrow{balance: balances, held: map[string]int64{}, owner: map[string]string{}, released: map[string]bool{}}
}

func (f *fakeEscrow) Hold(_ context.Context, id, user string, amount int64) error {
	f.ops = append(f.ops, "hold "+id)
	if f.holdErr != nil {
		return f.holdErr
	}
	if f.balance[user] < amount {
		return ErrInsufficientFunds
	}
	f.balance[user] -= amount
	f.held[id], f.owner[id] = amount, user
	return nil
}

func (f *fakeEscrow) Release(_ context.Context, id string) error {
	f.ops = append(f.ops, "release "+id)
	if !f.released[id] {
		f.balance[f.owner[id]] += f.held[id]
		f.released[id] = true
	}
	return nil
}

// fakeBidStore: DB giả cho đặt giá.
type fakeBidStore struct {
	lot       *domain.NFT
	bids      []*domain.Bid
	commitErr error
	events    []domain.OutboxEvent
	prevSeen  string
}

func (f *fakeBidStore) Lot(context.Context, string) (*domain.NFT, error) {
	if f.lot == nil {
		return nil, domain.ErrNFTNotFound
	}
	cp := *f.lot
	return &cp, nil
}

func (f *fakeBidStore) Highest(context.Context, string) (*domain.Bid, error) {
	for i := len(f.bids) - 1; i >= 0; i-- {
		if f.bids[i].Status == domain.BidHeld {
			return f.bids[i], nil
		}
	}
	return nil, nil
}

func (f *fakeBidStore) CommitBid(_ context.Context, lot *domain.NFT, bid *domain.Bid, prevBidID string, ev []domain.OutboxEvent) error {
	if f.commitErr != nil {
		return f.commitErr
	}
	for _, b := range f.bids {
		if b.ID == prevBidID {
			b.Status = domain.BidReleased
		}
	}
	f.prevSeen = prevBidID
	f.lot = lot
	f.bids = append(f.bids, bid)
	f.events = append(f.events, ev...)
	return nil
}

func activeLot() *domain.NFT {
	end := t0.Add(time.Hour)
	return &domain.NFT{ID: "lot-1", OwnerID: "seller", StartPrice: 1_000_000, CurrentPrice: 1_000_000,
		Status: domain.LotActive, PassportID: "0xpp", EndsAt: &end}
}

func newBidder(store *fakeBidStore, esc *fakeEscrow) *Bidder {
	n := 0
	return &Bidder{Store: store, Escrow: esc, Now: func() time.Time { return t0 },
		NewID: func() string { n++; return "bid-" + string(rune('0'+n)) }}
}

func rejected(t *testing.T, err error) {
	t.Helper()
	var r *Rejected
	if !errors.As(err, &r) {
		t.Fatalf("muốn lỗi Rejected, nhận %v", err)
	}
}

func TestBidHoldsMoneyAndOutbidReleases(t *testing.T) {
	store, esc := &fakeBidStore{lot: activeLot()}, newEscrow(map[string]int64{"alice": 5_000_000, "bob": 5_000_000})
	b := newBidder(store, esc)
	first, err := b.Place(context.Background(), "lot-1", "alice", 1_500_000)
	if err != nil {
		t.Fatal(err)
	}
	if first.HoldID != first.ID || esc.balance["alice"] != 3_500_000 || store.lot.CurrentPrice != 1_500_000 {
		t.Fatalf("lượt đầu phải giữ 1.5tr của alice: %+v %v", first, esc.balance)
	}
	if store.lot.OwnerID != "seller" {
		t.Fatal("đặt giá KHÔNG được đổi chủ (docs/23 §5.2)")
	}
	second, err := b.Place(context.Background(), "lot-1", "bob", 2_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if store.prevSeen != first.ID || store.lot.HighestBidID != second.ID {
		t.Fatalf("lượt của bob phải thay lượt cao nhất của alice: prev=%s highest=%s", store.prevSeen, store.lot.HighestBidID)
	}
	var topics []string
	for _, e := range store.events {
		topics = append(topics, e.Topic)
		if e.Topic == "nft.purchased" {
			t.Fatal("đặt giá không được phát nft.purchased")
		}
	}
	var rel map[string]string
	for _, e := range store.events {
		if e.Topic == TopicEscrowRelease {
			_ = json.Unmarshal(e.Payload, &rel)
		}
	}
	if rel["hold_id"] != first.HoldID {
		t.Fatalf("outbox phải nhả khoản giữ của alice, nhận %v (topics %v)", rel, topics)
	}
}

func TestBidRejections(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(l *domain.NFT) (string, float64){
		"hết giờ":          func(l *domain.NFT) (string, float64) { e := t0.Add(-time.Second); l.EndsAt = &e; return "alice", 2e6 },
		"lot cũ":           func(l *domain.NFT) (string, float64) { l.EndsAt = nil; return "alice", 2e6 },
		"không active":     func(l *domain.NFT) (string, float64) { l.Status = domain.LotSettling; return "alice", 2e6 },
		"người bán tự đặt": func(l *domain.NFT) (string, float64) { return "seller", 2e6 },
		"không cao hơn":    func(l *domain.NFT) (string, float64) { return "alice", 1_000_000 },
		"lẻ đồng":          func(l *domain.NFT) (string, float64) { return "alice", 1_500_000.5 },
	}
	for name, mutate := range cases {
		lot := activeLot()
		user, amount := mutate(lot)
		store, esc := &fakeBidStore{lot: lot}, newEscrow(map[string]int64{"alice": 9e6, "seller": 9e6})
		_, err := newBidder(store, esc).Place(ctx, "lot-1", user, amount)
		rejected(t, err)
		if len(esc.ops) != 0 {
			t.Errorf("%s: bị từ chối thì không được động tới ký quỹ, nhận %v", name, esc.ops)
		}
	}
}

func TestBidInsufficientFunds(t *testing.T) {
	store, esc := &fakeBidStore{lot: activeLot()}, newEscrow(map[string]int64{"alice": 100})
	_, err := newBidder(store, esc).Place(context.Background(), "lot-1", "alice", 2_000_000)
	rejected(t, err)
	if len(store.bids) != 0 {
		t.Fatal("thiếu tiền thì không ghi lượt đặt giá")
	}
}

// Ghi DB hỏng sau khi đã giữ tiền: phải nhả ngay khoản vừa giữ.
func TestBidCommitFailureReleasesHold(t *testing.T) {
	store, esc := &fakeBidStore{lot: activeLot(), commitErr: errors.New("db sập")}, newEscrow(map[string]int64{"alice": 5e6})
	if _, err := newBidder(store, esc).Place(context.Background(), "lot-1", "alice", 2_000_000); err == nil {
		t.Fatal("phải báo lỗi")
	}
	if esc.balance["alice"] != 5e6 || len(esc.ops) != 2 || esc.ops[1] != "release bid-1" {
		t.Fatalf("phải nhả lại khoản giữ: balance=%d ops=%v", esc.balance["alice"], esc.ops)
	}
}
