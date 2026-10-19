//go:build integration

// Test tích hợp trên Postgres thật (docs/24b, Task 5): khoá dòng, chỉ mục một phần
// và CAS chỉ kiểm được trên DB thật. Chạy:
//
//	TEST_DATABASE_DSN='host=127.0.0.1 port=55439 user=test password=test dbname=sanmp_test sslmode=disable' \
//	  go test -tags integration ./internal/repository/
package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"bidding-service/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var t0 = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Fatal("cần TEST_DATABASE_DSN trỏ tới Postgres thử (xem đầu tệp)")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"settlements", "bids", "outbox_events", "nfts"} {
		db.Exec("DROP TABLE IF EXISTS " + tbl)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func lot(id, passport string, ends time.Time) *domain.NFT {
	return &domain.NFT{ID: id, Title: id, OwnerID: "seller", CreatorID: "seller", StartPrice: 1e6, CurrentPrice: 1e6,
		Status: domain.LotActive, PassportID: passport, EndsAt: &ends, CreatedAt: t0, UpdatedAt: t0}
}

func bid(id, lotID, user string, amount int64) *domain.Bid {
	return &domain.Bid{ID: id, NFTID: lotID, UserID: user, Amount: amount, HoldID: id, Status: domain.BidHeld, CreatedAt: t0}
}

func TestCommitBidReleasesPrevious(t *testing.T) {
	db := testDB(t)
	s := &AuctionStore{DB: db}
	ctx := context.Background()
	l := lot("lot-1", "0xp1", t0.Add(time.Hour))
	if err := db.Create(l).Error; err != nil {
		t.Fatal(err)
	}
	l.CurrentPrice, l.HighestBidID = 2e6, "bid-1"
	if err := s.CommitBid(ctx, l, bid("bid-1", "lot-1", "alice", 2e6), "", []domain.OutboxEvent{{Topic: "bid.placed", Payload: []byte("{}")}}); err != nil {
		t.Fatal(err)
	}
	l.CurrentPrice, l.HighestBidID = 3e6, "bid-2"
	if err := s.CommitBid(ctx, l, bid("bid-2", "lot-1", "bob", 3e6), "bid-1", nil); err != nil {
		t.Fatal(err)
	}
	h, _ := s.Highest(ctx, "lot-1")
	var b1 domain.Bid
	db.First(&b1, "id = ?", "bid-1")
	var n int64
	db.Model(&domain.OutboxEvent{}).Count(&n)
	if h == nil || h.ID != "bid-2" || b1.Status != domain.BidReleased || n != 1 {
		t.Fatalf("highest=%v bid-1=%s outbox=%d", h, b1.Status, n)
	}
	if ok, _ := s.BidExistsForHold(ctx, "bid-2"); !ok {
		t.Fatal("bid-2 phải có dòng bids")
	}
	// Lot không còn active: lượt đặt giá phải hỏng nguyên khối (không có dòng bid).
	db.Model(&domain.NFT{}).Where("id = ?", "lot-1").Update("status", domain.LotSettling)
	if err := s.CommitBid(ctx, l, bid("bid-3", "lot-1", "carol", 4e6), "bid-2", nil); err == nil {
		t.Fatal("lot đã đóng thì CommitBid phải lỗi")
	}
	if ok, _ := s.BidExistsForHold(ctx, "bid-3"); ok {
		t.Fatal("transaction hỏng không được để lại dòng bid")
	}
}

func TestCloseLot(t *testing.T) {
	db := testDB(t)
	s := &AuctionStore{DB: db}
	ctx := context.Background()
	db.Create(lot("lot-bids", "0xpa", t0))
	db.Create(lot("lot-empty", "0xpb", t0))
	db.Create(lot("lot-later", "0xpc", t0.Add(time.Hour)))
	db.Create(bid("bid-a", "lot-bids", "alice", 2e6))
	db.Create(bid("bid-b", "lot-bids", "bob", 3e6))

	due, _ := s.DueLots(ctx, t0, 10)
	if len(due) != 2 {
		t.Fatalf("muốn 2 lot tới hạn, nhận %v", due)
	}
	st, err := s.CloseLot(ctx, "lot-bids", t0)
	if err != nil || st == nil {
		t.Fatalf("đóng lot có lượt đặt giá phải ra settlement: %v %v", st, err)
	}
	if st.WinnerID != "bob" || st.Amount != 3e6 || st.HoldID != "bid-b" || st.SellerID != "seller" ||
		st.State != domain.StlPendingOpen || st.AuctionRef != "lot-bids-"+st.ID {
		t.Fatalf("settlement sai: %+v", st)
	}
	var l domain.NFT
	db.First(&l, "id = ?", "lot-bids")
	var w domain.Bid
	db.First(&w, "id = ?", "bid-b")
	if l.Status != domain.LotSettling || w.Status != domain.BidWon {
		t.Fatalf("lot %s, bid thắng %s", l.Status, w.Status)
	}
	if again, _ := s.CloseLot(ctx, "lot-bids", t0); again != nil {
		t.Fatal("đóng lại lần hai không được tạo settlement mới")
	}
	if st, _ := s.CloseLot(ctx, "lot-empty", t0); st != nil {
		t.Fatal("lot không ai đặt giá không có settlement")
	}
	var empty domain.NFT // biến mới: gorm thêm điều kiện khoá chính nếu struct đã có ID
	db.First(&empty, "id = ?", "lot-empty")
	if empty.Status != domain.LotUnsold {
		t.Fatalf("lot không ai đặt giá phải unsold, nhận %s", empty.Status)
	}
	if st, _ := s.CloseLot(ctx, "lot-later", t0); st != nil {
		t.Fatal("lot chưa tới hạn không được đóng")
	}
}

func TestSettlementCASAndFinish(t *testing.T) {
	db := testDB(t)
	s := &AuctionStore{DB: db}
	ctx := context.Background()
	db.Create(lot("lot-1", "0xp1", t0))
	db.Model(&domain.NFT{}).Where("id = ?", "lot-1").Update("status", domain.LotSettling)
	st := &domain.Settlement{ID: "stl-1", LotID: "lot-1", SellerID: "seller", WinnerID: "bob", HoldID: "bid-b",
		State: domain.StlPendingOpen, NextAttemptAt: t0}
	db.Create(st)

	if due, _ := s.Due(ctx, t0, 10); len(due) != 1 {
		t.Fatalf("muốn 1 settlement tới hạn, nhận %d", len(due))
	}
	next := *st
	next.State, next.SaleID, next.NextAttemptAt = domain.StlAwaitAuth, "0xsale", t0.Add(time.Minute)
	if ok, _ := s.Save(ctx, &next, domain.StlCompleting); ok {
		t.Fatal("CAS với trạng thái cũ sai phải không ghi")
	}
	if ok, err := s.Save(ctx, &next, domain.StlPendingOpen); !ok || err != nil {
		t.Fatalf("CAS đúng phải ghi: %v", err)
	}
	if due, _ := s.Due(ctx, t0, 10); len(due) != 0 {
		t.Fatal("chưa tới next_attempt_at thì không được lấy ra")
	}
	got, _ := s.Get(ctx, "stl-1")
	if got.State != domain.StlAwaitAuth || got.SaleID != "0xsale" {
		t.Fatalf("Save phải ghi cả dòng: %+v", got)
	}

	sold := *got
	sold.State = domain.StlSold
	ok, err := s.Finish(ctx, &sold, domain.StlAwaitAuth, domain.LotSold, "bob",
		[]domain.OutboxEvent{{Topic: "nft.purchased", Payload: []byte("{}")}})
	if !ok || err != nil {
		t.Fatalf("Finish: %v %v", ok, err)
	}
	var l domain.NFT
	db.First(&l, "id = ?", "lot-1")
	var n int64
	db.Model(&domain.OutboxEvent{}).Where("topic = ?", "nft.purchased").Count(&n)
	if l.Status != domain.LotSold || l.OwnerID != "bob" || n != 1 {
		t.Fatalf("lot %s chủ %s outbox %d", l.Status, l.OwnerID, n)
	}
	if ok, _ := s.Finish(ctx, &sold, domain.StlAwaitAuth, domain.LotSold, "bob", nil); ok {
		t.Fatal("Finish lần hai (CAS cũ) không được ghi")
	}
	if st, _ := s.SettlementOfLot(ctx, "lot-1"); st == nil || st.ID != "stl-1" {
		t.Fatal("SettlementOfLot phải tìm thấy saga của lot")
	}
}

// docs/23 §5.1: một hộ chiếu chỉ nằm trong một lot đang mở / đang thanh toán.
func TestOnePassportOneOpenLot(t *testing.T) {
	db := testDB(t)
	if err := db.Create(lot("lot-1", "0xsame", t0)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(lot("lot-2", "0xsame", t0)).Error; err == nil {
		t.Fatal("lot thứ hai cùng hộ chiếu khi lot đầu còn mở phải bị chặn")
	}
	db.Model(&domain.NFT{}).Where("id = ?", "lot-1").Update("status", domain.LotSold)
	if err := db.Create(lot("lot-3", "0xsame", t0)).Error; err != nil {
		t.Fatalf("lot đầu đã bán thì được đăng lại (bán lại): %v", err)
	}
	if err := db.Create(lot("lot-4", "", t0)).Error; err != nil {
		t.Fatal("lot cũ không có hộ chiếu không bị chỉ mục chặn")
	}
	if err := db.Create(lot("lot-5", "", t0)).Error; err != nil {
		t.Fatal("nhiều lot không có hộ chiếu vẫn được")
	}
}

// Review cuối 24b #1: khoá Redis có thể hết hạn giữa lượt đặt giá. Thứ tự đặt giá
// phải được giữ ở DB: lượt đặt giá dựa trên "người dẫn đầu" cũ, hoặc không cao hơn
// giá hiện tại, phải hỏng nguyên khối (Bidder nhả khoản giữ của nó).
func TestCommitBidRejectsStaleLeader(t *testing.T) {
	db := testDB(t)
	s := &AuctionStore{DB: db}
	ctx := context.Background()
	l := lot("lot-1", "0xp1", t0.Add(time.Hour))
	db.Create(l)
	b := *l
	b.CurrentPrice, b.HighestBidID = 3e6, "bid-b"
	if err := s.CommitBid(ctx, &b, bid("bid-b", "lot-1", "bob", 3e6), "", nil); err != nil {
		t.Fatal(err)
	}
	// Lượt của A tính "người dẫn đầu" lúc chưa có B (prev rỗng) và giá thấp hơn.
	a := *l
	a.CurrentPrice, a.HighestBidID = 2e6, "bid-a"
	if err := s.CommitBid(ctx, &a, bid("bid-a", "lot-1", "alice", 2e6), "", nil); err == nil {
		t.Fatal("lượt đặt giá dựa trên người dẫn đầu cũ phải bị từ chối")
	}
	// Đúng người dẫn đầu nhưng giá không cao hơn cũng bị từ chối.
	c := *l
	c.CurrentPrice, c.HighestBidID = 3e6, "bid-c"
	if err := s.CommitBid(ctx, &c, bid("bid-c", "lot-1", "carol", 3e6), "bid-b", nil); err == nil {
		t.Fatal("giá không cao hơn giá hiện tại phải bị từ chối")
	}
	var got domain.NFT
	db.First(&got, "id = ?", "lot-1")
	var bb domain.Bid
	db.First(&bb, "id = ?", "bid-b")
	if got.CurrentPrice != 3e6 || got.HighestBidID != "bid-b" || bb.Status != domain.BidHeld {
		t.Fatalf("giá và người dẫn đầu phải giữ nguyên: %v %s %s", got.CurrentPrice, got.HighestBidID, bb.Status)
	}
	for _, id := range []string{"bid-a", "bid-c"} {
		if ok, _ := s.BidExistsForHold(ctx, id); ok {
			t.Fatalf("%s bị từ chối không được để lại dòng bids", id)
		}
	}
}
