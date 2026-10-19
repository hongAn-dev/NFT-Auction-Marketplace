package settlement

import (
	"context"
	"errors"
	"testing"
	"time"

	"bidding-service/internal/domain"
)

var t0 = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

type transientErr struct{}

func (transientErr) Error() string   { return "mạng chập chờn" }
func (transientErr) Transient() bool { return true }

var errPermanent = errors.New("FailedPrecondition: [PP-007] người bán không còn là chủ")

// fakeChain: passport-service giả. status là trạng thái vụ bán trên chuỗi.
type fakeChain struct {
	status, deliveryDeadline string
	openErr, getErr, doneErr error
	cancelErr                error
	calls                    []string
}

func (f *fakeChain) OpenSale(_ context.Context, r OpenRequest) (*Sale, error) {
	f.calls = append(f.calls, "open "+r.AuctionRef)
	if f.openErr != nil {
		return nil, f.openErr
	}
	f.status = "OPEN"
	return &Sale{SaleID: "0xsale", PassportID: "0xpp", Status: "OPEN", DeadlineAt: t0.Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)}, nil
}

func (f *fakeChain) GetSale(context.Context, string) (*Sale, error) {
	f.calls = append(f.calls, "get")
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &Sale{SaleID: "0xsale", Status: f.status, DeadlineAt: t0.Add(7 * 24 * time.Hour).Format(time.RFC3339Nano),
		DeliveryDeadlineAt: f.deliveryDeadline}, nil
}

func (f *fakeChain) CompleteSale(context.Context, string) error {
	f.calls = append(f.calls, "complete")
	if f.doneErr != nil {
		return f.doneErr
	}
	f.status = "COMPLETED"
	return nil
}

func (f *fakeChain) CancelSale(_ context.Context, _, reason string) error {
	f.calls = append(f.calls, "cancel "+reason)
	if f.cancelErr != nil {
		return f.cancelErr
	}
	f.status = "CANCELLED"
	return nil
}

// fakeMoney: ghi thao tác tiền theo holdID.
type fakeMoney struct {
	ops []string
	err error
}

func (f *fakeMoney) Capture(_ context.Context, id, seller string) error {
	if f.err != nil {
		return f.err
	}
	f.ops = append(f.ops, "capture "+id+" "+seller)
	return nil
}

func (f *fakeMoney) Refund(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.ops = append(f.ops, "refund "+id)
	return nil
}

type fakeStore struct {
	s         *domain.Settlement
	lotStatus string
	lotOwner  string
	events    []domain.OutboxEvent
}

func (f *fakeStore) Due(context.Context, time.Time, int) ([]*domain.Settlement, error) {
	if f.s.Terminal() {
		return nil, nil
	}
	cp := *f.s
	return []*domain.Settlement{&cp}, nil
}

func (f *fakeStore) Get(context.Context, string) (*domain.Settlement, error) {
	cp := *f.s
	return &cp, nil
}

func (f *fakeStore) Lot(context.Context, string) (*domain.NFT, error) {
	return &domain.NFT{ID: "lot-1", BrandCode: "ROLEX", Model: "Submariner", Serial: "116610LN7Z3X9Q1",
		ImageUrl: "https://anh/1.jpg"}, nil
}

func (f *fakeStore) Save(_ context.Context, s *domain.Settlement, from string) (bool, error) {
	if f.s.State != from {
		return false, nil
	}
	cp := *s
	f.s = &cp
	return true, nil
}

func (f *fakeStore) Finish(ctx context.Context, s *domain.Settlement, from, lotStatus, owner string, ev []domain.OutboxEvent) (bool, error) {
	ok, err := f.Save(ctx, s, from)
	if ok {
		f.lotStatus, f.lotOwner, f.events = lotStatus, owner, append(f.events, ev...)
	}
	return ok, err
}

type world struct {
	chain *fakeChain
	money *fakeMoney
	store *fakeStore
	eng   *Engine
	now   time.Time
}

func newWorld() *world {
	w := &world{chain: &fakeChain{}, money: &fakeMoney{}, now: t0}
	w.store = &fakeStore{s: &domain.Settlement{ID: "stl-1", LotID: "lot-1", SellerID: "seller", WinnerID: "winner",
		Amount: 2_000_000, HoldID: "bid-9", AuctionRef: "lot-1-stl-1", State: domain.StlPendingOpen, ClosedAt: t0}}
	w.eng = &Engine{Store: w.store, Chain: w.chain, Money: w.money, Now: func() time.Time { return w.now }}
	return w
}

// tick: một vòng worker ở thời điểm w.now; bỏ qua lịch thử lại.
func (w *world) tick() {
	w.store.s.NextAttemptAt = time.Time{}
	w.eng.Tick(context.Background())
}

func (w *world) state() string { return w.store.s.State }

func (w *world) run(n int) {
	for i := 0; i < n && !w.store.s.Terminal(); i++ {
		w.tick()
	}
}

func noDoubleMoney(t *testing.T, m *fakeMoney) {
	t.Helper()
	caps, refs := 0, 0
	for _, op := range m.ops {
		switch {
		case len(op) > 7 && op[:7] == "capture":
			caps++
		case len(op) > 6 && op[:6] == "refund":
			refs++
		}
	}
	if caps > 0 && refs > 0 {
		t.Fatalf("BẤT BIẾN BỊ PHÁ: vừa capture vừa refund %v", m.ops)
	}
}

// Vòng đạt: mở → PASSED → người thắng xác nhận → COMPLETED → trả tiền người bán.
func TestHappyPath(t *testing.T) {
	w := newWorld()
	w.tick()
	if w.state() != domain.StlAwaitAuth || w.store.s.SaleID != "0xsale" || w.store.s.OpenedAt == nil {
		t.Fatalf("sau open-sale: %s %+v", w.state(), w.store.s)
	}
	if w.chain.calls[0] != "open lot-1-stl-1" {
		t.Fatalf("auction_ref sai: %v", w.chain.calls)
	}
	w.tick() // chuỗi vẫn OPEN, chưa tới hạn
	if w.state() != domain.StlAwaitAuth {
		t.Fatalf("còn OPEN thì chờ, nhận %s", w.state())
	}
	w.chain.status, w.chain.deliveryDeadline = "PASSED", t0.Add(14*24*time.Hour).Format(time.RFC3339Nano)
	w.tick()
	if w.state() != domain.StlAwaitDelivery || w.store.s.DeliveryDeadlineAt == nil || w.store.s.DecidedAt == nil {
		t.Fatalf("PASSED phải sang AWAIT_DELIVERY có hạn giao: %s", w.state())
	}
	if err := w.eng.ConfirmReceived(context.Background(), "stl-1", "nguoi-la"); !errors.Is(err, ErrNotWinner) {
		t.Fatalf("người khác không được xác nhận, nhận %v", err)
	}
	if err := w.eng.ConfirmReceived(context.Background(), "stl-1", "winner"); err != nil {
		t.Fatal(err)
	}
	if w.state() != domain.StlCompleting || w.store.s.ReceivedAt == nil {
		t.Fatalf("xác nhận phải sang COMPLETING: %s", w.state())
	}
	if len(w.money.ops) != 0 {
		t.Fatal("chưa thấy COMPLETED thì chưa được chuyển tiền")
	}
	w.tick()
	if w.state() != domain.StlSold || w.store.lotStatus != domain.LotSold || w.store.lotOwner != "winner" {
		t.Fatalf("phải SOLD, lot đổi chủ: %s %s %s", w.state(), w.store.lotStatus, w.store.lotOwner)
	}
	if len(w.money.ops) != 1 || w.money.ops[0] != "capture bid-9 seller" {
		t.Fatalf("phải capture đúng một lần cho người bán: %v", w.money.ops)
	}
	if len(w.store.events) != 1 || w.store.events[0].Topic != "nft.purchased" {
		t.Fatalf("SOLD phải phát nft.purchased: %v", w.store.events)
	}
	if w.store.s.CompletedAt == nil || w.store.s.SettledAt == nil {
		t.Fatal("thiếu mốc thời gian cho bảng độ trễ E3′")
	}
	if err := w.eng.ConfirmReceived(context.Background(), "stl-1", "winner"); err != nil {
		t.Fatalf("xác nhận lại sau khi xong phải idempotent, nhận %v", err)
	}
}

// Các nhánh hoàn tiền (docs/23 §6.1).
func TestRefundPaths(t *testing.T) {
	cases := []struct {
		name      string
		drive     func(w *world)
		wantOut   string
		wantChain string // lệnh chuỗi phải có, rỗng = không kiểm
	}{
		{"giám định FAIL", func(w *world) { w.chain.status = "FAILED" }, domain.OutcomeFailed, ""},
		{"quá hạn giám định", func(w *world) { w.now = t0.Add(8 * 24 * time.Hour) }, domain.OutcomeCancelled, "cancel AUTH_TIMEOUT"},
		{"thất lạc sau PASSED", func(w *world) {
			w.chain.status = "PASSED"
			w.tick()
			w.chain.status = "CANCELLED"
		}, domain.OutcomeLost, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld()
			w.tick()
			c.drive(w)
			w.run(5)
			if w.state() != domain.StlRefunded || w.store.lotStatus != domain.LotRefunded || w.store.s.Outcome != c.wantOut {
				t.Fatalf("muốn REFUNDED/refunded/%s, nhận %s/%s/%s", c.wantOut, w.state(), w.store.lotStatus, w.store.s.Outcome)
			}
			if len(w.money.ops) != 1 || w.money.ops[0] != "refund bid-9" {
				t.Fatalf("phải refund đúng một lần: %v", w.money.ops)
			}
			if c.wantChain != "" {
				found := false
				for _, call := range w.chain.calls {
					found = found || call == c.wantChain
				}
				if !found {
					t.Fatalf("muốn lệnh %q, nhận %v", c.wantChain, w.chain.calls)
				}
			}
		})
	}
}

// Chuỗi từ chối mở vụ bán (PP-007): hoàn tiền, lot CANCELLED, ghi lỗi.
func TestOpenRejected(t *testing.T) {
	w := newWorld()
	w.chain.openErr = errPermanent
	w.run(3)
	if w.state() != domain.StlRefunded || w.store.lotStatus != domain.LotCancelled || w.store.s.LastError == "" {
		t.Fatalf("muốn REFUNDED + lot cancelled + ghi lỗi, nhận %s %s %q", w.state(), w.store.lotStatus, w.store.s.LastError)
	}
}

// Lỗi tạm thời: giữ nguyên trạng thái, hẹn lại với độ chờ tăng dần.
func TestTransientErrorsBackOff(t *testing.T) {
	w := newWorld()
	w.chain.openErr = transientErr{}
	w.eng.Tick(context.Background())
	first := w.store.s.NextAttemptAt
	w.store.s.NextAttemptAt = time.Time{}
	w.eng.Tick(context.Background())
	second := w.store.s.NextAttemptAt
	if w.state() != domain.StlPendingOpen || w.store.s.Attempts != 2 || !second.After(first) {
		t.Fatalf("muốn PENDING_OPEN, 2 lần thử, chờ tăng dần: %s %d %v %v", w.state(), w.store.s.Attempts, first, second)
	}
	w.chain.openErr = nil
	w.tick()
	if w.state() != domain.StlAwaitAuth || w.store.s.Attempts != 0 {
		t.Fatalf("hết lỗi thì đi tiếp và xoá đếm: %s %d", w.state(), w.store.s.Attempts)
	}
	m := &fakeMoney{err: transientErr{}}
	w2 := newWorld()
	w2.money = m
	w2.eng.Money = m
	w2.chain.openErr = errPermanent
	w2.run(3)
	if w2.state() != domain.StlRefunding {
		t.Fatalf("hoàn tiền lỗi tạm thời thì ở lại REFUNDING, nhận %s", w2.state())
	}
}

// Đua: giám định báo thất lạc đúng lúc người thắng bấm "đã nhận" — CompleteSale bị
// chaincode từ chối (PP-005) vì vụ bán đã CANCELLED: phải hoàn tiền, không capture.
func TestCompleteLosesRaceToReportLost(t *testing.T) {
	w := newWorld()
	w.tick()
	w.chain.status = "PASSED"
	w.tick()
	_ = w.eng.ConfirmReceived(context.Background(), "stl-1", "winner")
	w.chain.status = "CANCELLED"
	w.chain.doneErr = errPermanent
	w.run(4)
	if w.state() != domain.StlRefunded || w.store.s.Outcome != domain.OutcomeLost {
		t.Fatalf("muốn REFUNDED/LOST, nhận %s/%s", w.state(), w.store.s.Outcome)
	}
	noDoubleMoney(t, w.money)
}

// Hết hạn giao hàng mà không ai lên tiếng: tự hoàn tất. Vụ bán không có hạn giao
// (tạo trước chaincode v0.3.0) thì không tự hoàn tất.
func TestDeliveryDeadline(t *testing.T) {
	w := newWorld()
	w.tick()
	w.chain.status, w.chain.deliveryDeadline = "PASSED", t0.Add(14*24*time.Hour).Format(time.RFC3339Nano)
	w.tick()
	w.now = t0.Add(15 * 24 * time.Hour)
	w.run(3)
	if w.state() != domain.StlSold {
		t.Fatalf("quá hạn giao phải tự hoàn tất, nhận %s", w.state())
	}

	old := newWorld()
	old.tick()
	old.chain.status = "PASSED"
	old.tick()
	old.now = t0.Add(400 * 24 * time.Hour)
	old.tick()
	if old.state() != domain.StlAwaitDelivery {
		t.Fatalf("vụ bán không có hạn giao không được tự hoàn tất, nhận %s", old.state())
	}
}

// Lỗi không thử lại được ngoài bước open-sale → NEEDS_ATTENTION, không chuyển tiền.
func TestPermanentErrorNeedsAttention(t *testing.T) {
	w := newWorld()
	w.tick()
	w.chain.getErr = errPermanent
	w.tick()
	if w.state() != domain.StlNeedsAttention || len(w.money.ops) != 0 || w.store.s.LastError == "" {
		t.Fatalf("muốn NEEDS_ATTENTION không chuyển tiền, nhận %s %v", w.state(), w.money.ops)
	}
}

// Bất biến tiền trên mọi chuỗi trạng thái chuỗi có thể gặp.
func TestNeverCaptureAndRefund(t *testing.T) {
	seqs := [][]string{
		{"OPEN", "PASSED", "CANCELLED", "COMPLETED"},
		{"OPEN", "FAILED", "COMPLETED"},
		{"PASSED", "COMPLETED", "CANCELLED"},
		{"CANCELLED", "PASSED", "COMPLETED"},
	}
	for _, seq := range seqs {
		w := newWorld()
		w.tick()
		for _, st := range seq {
			w.chain.status = st
			if st == "PASSED" {
				_ = w.eng.ConfirmReceived(context.Background(), "stl-1", "winner")
			}
			w.tick()
			w.tick()
		}
		noDoubleMoney(t, w.money)
	}
}

// Đồng hồ sàn nhanh hơn peer: CancelSale bị từ chối (PP-006 chưa tới hạn) trong
// khi chuỗi vẫn OPEN — phải thử lại sau, không phải NEEDS_ATTENTION.
func TestCancelTooEarlyRetries(t *testing.T) {
	w := newWorld()
	w.tick()
	w.now = t0.Add(8 * 24 * time.Hour)
	w.chain.cancelErr = errors.New("FailedPrecondition: [PP-006] vụ bán chưa tới hạn")
	w.tick() // AWAIT_AUTH → CANCELLING
	w.tick() // CancelSale bị từ chối, chuỗi vẫn OPEN
	if w.state() != domain.StlCancelling || w.store.s.Attempts != 1 {
		t.Fatalf("muốn ở lại CANCELLING và hẹn lại, nhận %s (lần thử %d)", w.state(), w.store.s.Attempts)
	}
	w.chain.cancelErr = nil
	w.run(3)
	if w.state() != domain.StlRefunded || w.store.s.Outcome != domain.OutcomeCancelled {
		t.Fatalf("hết lỗi thì huỷ và hoàn tiền, nhận %s/%s", w.state(), w.store.s.Outcome)
	}
}
