// Package settlement: saga thanh toán sau khi đóng phiên (docs/23 §6).
//
// Bảng settlements là hàng đợi bền của saga: worker đọc các dòng chưa kết thúc tới
// hạn và làm đúng một bước cho mỗi dòng. Mọi chuyển trạng thái là CAS (`WHERE
// state = cũ`), nên hai worker chạy song song không làm một bước hai lần. Mọi lệnh
// gửi ra ngoài (chuỗi, ký quỹ) đều idempotent, nên làm lại sau khi sập là an toàn.
//
// Quy tắc tiền: chuỗi quyết định trước, tiền chuyển sau. Capture chỉ sau khi chuỗi
// đã COMPLETED; refund chỉ sau khi chuỗi đã FAILED / CANCELLED hoặc từ chối mở vụ
// bán. Hai nhánh nằm trên hai đường rẽ của máy trạng thái, không bao giờ gặp nhau.
package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"bidding-service/internal/domain"
)

// Sale: phần vụ bán trên chuỗi mà saga cần.
type Sale struct {
	SaleID             string `json:"sale_id"`
	PassportID         string `json:"passport_id"`
	Status             string `json:"status"`
	DeadlineAt         string `json:"deadline_at"`
	DeliveryDeadlineAt string `json:"delivery_deadline_at"`
}

type OpenRequest struct {
	AuctionRef, Brand, Model, Serial, SellerID, WinnerID, PhotosHash string
	DeadlineDays                                                     int
}

// Chain: blockchain-service (docs/23 §3).
type Chain interface {
	OpenSale(ctx context.Context, r OpenRequest) (*Sale, error)
	GetSale(ctx context.Context, saleID string) (*Sale, error)
	CompleteSale(ctx context.Context, saleID string) error
	CancelSale(ctx context.Context, saleID, reason string) error
}

// Money: ký quỹ ở auth-service.
type Money interface {
	Capture(ctx context.Context, holdID, sellerID string) error
	Refund(ctx context.Context, holdID string) error
}

type Store interface {
	// Due: dòng chưa kết thúc có next_attempt_at <= now.
	Due(ctx context.Context, now time.Time, limit int) ([]*domain.Settlement, error)
	Get(ctx context.Context, id string) (*domain.Settlement, error)
	Lot(ctx context.Context, lotID string) (*domain.NFT, error)
	// Save: ghi cả dòng nếu state hiện tại == from (CAS). false = đã có người đổi.
	Save(ctx context.Context, s *domain.Settlement, from string) (bool, error)
	// Finish: như Save, cộng cập nhật lot (status, owner nếu khác rỗng) và ghi
	// outbox — trong MỘT transaction.
	Finish(ctx context.Context, s *domain.Settlement, from, lotStatus, newOwner string, events []domain.OutboxEvent) (bool, error)
}

// IsTransient: lỗi đáng thử lại (mạng, 503). Client HTTP gắn phương thức
// Transient() cho các lỗi đó.
func IsTransient(err error) bool {
	var t interface{ Transient() bool }
	return errors.As(err, &t) && t.Transient()
}

var (
	ErrNotWinner  = errors.New("chỉ người thắng phiên mới xác nhận đã nhận hàng")
	ErrWrongState = errors.New("vụ bán chưa ở bước giao hàng")
)

const (
	// DeadlineDays: hạn giám định gửi kèm OpenSale (docs/23 §5.3).
	DeadlineDays = 7
	pollEvery    = 10 * time.Second
	maxBackoff   = 5 * time.Minute
	authTimeout  = "AUTH_TIMEOUT"
)

type Engine struct {
	Store Store
	Chain Chain
	Money Money
	Now   func() time.Time
}

// Tick: một vòng worker.
func (e *Engine) Tick(ctx context.Context) {
	due, err := e.Store.Due(ctx, e.Now(), 50)
	if err != nil {
		log.Printf("⚠️ saga: đọc settlements tới hạn: %v", err)
		return
	}
	for _, s := range due {
		if err := e.Step(ctx, s); err != nil {
			log.Printf("⚠️ saga %s (%s): %v", s.ID, s.State, err)
		}
	}
}

// Step: làm một bước cho một settlement. Chỉ trả lỗi khi không GHI được DB.
func (e *Engine) Step(ctx context.Context, s *domain.Settlement) error {
	switch s.State {
	case domain.StlPendingOpen:
		return e.open(ctx, s)
	case domain.StlAwaitAuth:
		return e.awaitAuth(ctx, s)
	case domain.StlAwaitDelivery:
		return e.awaitDelivery(ctx, s)
	case domain.StlCompleting:
		return e.completing(ctx, s)
	case domain.StlCancelling:
		return e.cancelling(ctx, s)
	case domain.StlRefunding:
		return e.refunding(ctx, s)
	}
	return nil
}

// ---- chuyển trạng thái ----------------------------------------------------

func (e *Engine) move(ctx context.Context, s *domain.Settlement, to string, edit func(*domain.Settlement)) error {
	from := s.State
	next := *s
	if edit != nil {
		edit(&next)
	}
	next.State, next.Attempts, next.NextAttemptAt = to, 0, e.Now()
	if to == from {
		next.NextAttemptAt = e.Now().Add(pollEvery)
	}
	_, err := e.Store.Save(ctx, &next, from)
	return err
}

func (e *Engine) wait(ctx context.Context, s *domain.Settlement) error {
	return e.move(ctx, s, s.State, nil)
}

// retry: lỗi tạm thời — giữ trạng thái, hẹn lại 1 s, 2 s, 4 s … tối đa 5 phút.
func (e *Engine) retry(ctx context.Context, s *domain.Settlement, cause error) error {
	next := *s
	next.Attempts++
	next.LastError = cause.Error()
	d := time.Second << min(next.Attempts-1, 20)
	if d > maxBackoff {
		d = maxBackoff
	}
	next.NextAttemptAt = e.Now().Add(d)
	_, err := e.Store.Save(ctx, &next, s.State)
	return err
}

// fail: lỗi không thử lại được → chờ admin, KHÔNG chuyển tiền.
func (e *Engine) fail(ctx context.Context, s *domain.Settlement, cause error) error {
	return e.move(ctx, s, domain.StlNeedsAttention, func(n *domain.Settlement) { n.LastError = cause.Error() })
}

func (e *Engine) refund(ctx context.Context, s *domain.Settlement, outcome string, cause error) error {
	return e.move(ctx, s, domain.StlRefunding, func(n *domain.Settlement) {
		n.Outcome = outcome
		if n.DecidedAt == nil {
			now := e.Now()
			n.DecidedAt = &now
		}
		if cause != nil {
			n.LastError = cause.Error()
		}
	})
}

func parseTime(v string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return nil
	}
	return &t
}

// ---- từng bước --------------------------------------------------------------

func (e *Engine) open(ctx context.Context, s *domain.Settlement) error {
	lot, err := e.Store.Lot(ctx, s.LotID)
	if err != nil {
		return err
	}
	photos := sha256.Sum256([]byte(lot.ImageUrl))
	sale, err := e.Chain.OpenSale(ctx, OpenRequest{
		AuctionRef: s.AuctionRef, Brand: lot.BrandCode, Model: lot.Model, Serial: lot.Serial,
		SellerID: s.SellerID, WinnerID: s.WinnerID, PhotosHash: "0x" + hex.EncodeToString(photos[:]),
		DeadlineDays: DeadlineDays,
	})
	switch {
	case IsTransient(err):
		return e.retry(ctx, s, err)
	case err != nil:
		// Ví dụ PP-007: người bán không còn là chủ. Chuỗi không mở vụ bán nào → hoàn tiền.
		return e.refund(ctx, s, domain.OutcomeOpenRejected, err)
	}
	return e.move(ctx, s, domain.StlAwaitAuth, func(n *domain.Settlement) {
		now := e.Now()
		n.SaleID, n.PassportID, n.OpenedAt, n.DeadlineAt = sale.SaleID, sale.PassportID, &now, parseTime(sale.DeadlineAt)
	})
}

// read: GetSale với cùng cách xử lý lỗi cho mọi bước.
func (e *Engine) read(ctx context.Context, s *domain.Settlement) (*Sale, error, bool) {
	sale, err := e.Chain.GetSale(ctx, s.SaleID)
	if IsTransient(err) {
		return nil, e.retry(ctx, s, err), false
	}
	if err != nil {
		return nil, e.fail(ctx, s, err), false
	}
	return sale, nil, true
}

func (e *Engine) awaitAuth(ctx context.Context, s *domain.Settlement) error {
	sale, err, ok := e.read(ctx, s)
	if !ok {
		return err
	}
	switch sale.Status {
	case "PASSED":
		return e.move(ctx, s, domain.StlAwaitDelivery, func(n *domain.Settlement) {
			now := e.Now()
			n.DecidedAt, n.DeliveryDeadlineAt = &now, parseTime(sale.DeliveryDeadlineAt)
		})
	case "FAILED":
		return e.refund(ctx, s, domain.OutcomeFailed, nil)
	case "CANCELLED":
		return e.refund(ctx, s, domain.OutcomeCancelled, nil)
	case "OPEN":
		if dl := parseTime(sale.DeadlineAt); dl != nil && !e.Now().Before(*dl) {
			return e.move(ctx, s, domain.StlCancelling, nil)
		}
		return e.wait(ctx, s)
	}
	return e.fail(ctx, s, fmt.Errorf("vụ bán %s ở trạng thái bất thường %q khi chờ giám định", s.SaleID, sale.Status))
}

func (e *Engine) awaitDelivery(ctx context.Context, s *domain.Settlement) error {
	sale, err, ok := e.read(ctx, s)
	if !ok {
		return err
	}
	switch sale.Status {
	case "CANCELLED": // giám định báo thất lạc (ReportLost)
		return e.refund(ctx, s, domain.OutcomeLost, nil)
	case "COMPLETED":
		return e.move(ctx, s, domain.StlCompleting, nil)
	case "PASSED":
		// Im lặng tới hạn giao = đã nhận (docs/23 §2 quyết định 8). Vụ bán không có hạn
		// giao (trước chaincode v0.3.0) thì chỉ người thắng xác nhận được.
		if s.DeliveryDeadlineAt != nil && !e.Now().Before(*s.DeliveryDeadlineAt) {
			return e.move(ctx, s, domain.StlCompleting, nil)
		}
		return e.wait(ctx, s)
	}
	return e.fail(ctx, s, fmt.Errorf("vụ bán %s ở trạng thái bất thường %q khi chờ giao hàng", s.SaleID, sale.Status))
}

func (e *Engine) completing(ctx context.Context, s *domain.Settlement) error {
	sale, err, ok := e.read(ctx, s)
	if !ok {
		return err
	}
	switch sale.Status {
	case "CANCELLED": // thất lạc được báo trước khi CompleteSale kịp commit
		return e.refund(ctx, s, domain.OutcomeLost, nil)
	case "PASSED":
		err := e.Chain.CompleteSale(ctx, s.SaleID)
		if IsTransient(err) {
			return e.retry(ctx, s, err)
		}
		if err != nil {
			// Chaincode từ chối (ví dụ PP-005 vì vụ bán vừa bị huỷ). Đọc lại ở vòng sau
			// rồi rẽ theo trạng thái thật; không thử CompleteSale mãi.
			return e.retry(ctx, s, err)
		}
	case "COMPLETED":
	default:
		return e.fail(ctx, s, fmt.Errorf("vụ bán %s ở trạng thái bất thường %q khi hoàn tất", s.SaleID, sale.Status))
	}
	// Chuỗi đã COMPLETED (CompleteSale chỉ trả về sau khi giao dịch commit).
	if err := e.Money.Capture(ctx, s.HoldID, s.SellerID); err != nil {
		if IsTransient(err) {
			return e.retry(ctx, s, err)
		}
		return e.fail(ctx, s, err)
	}
	payload, _ := json.Marshal(map[string]any{"bid_id": s.HoldID, "nft_id": s.LotID, "user_id": s.WinnerID,
		"amount": s.Amount, "created_at": e.Now().Unix()})
	next := *s
	now := e.Now()
	next.State, next.Attempts, next.LastError = domain.StlSold, 0, ""
	if next.CompletedAt == nil {
		next.CompletedAt = &now
	}
	next.SettledAt = &now
	_, err = e.Store.Finish(ctx, &next, s.State, domain.LotSold, s.WinnerID,
		[]domain.OutboxEvent{{Topic: "nft.purchased", Payload: payload}})
	return err
}

func (e *Engine) cancelling(ctx context.Context, s *domain.Settlement) error {
	err := e.Chain.CancelSale(ctx, s.SaleID, authTimeout)
	if IsTransient(err) {
		return e.retry(ctx, s, err)
	}
	if err == nil {
		return e.refund(ctx, s, domain.OutcomeCancelled, nil)
	}
	// Bị từ chối — ví dụ giám định ghi kết quả sát hạn. Đọc trạng thái thật rồi rẽ.
	sale, rerr, ok := e.read(ctx, s)
	if !ok {
		return rerr
	}
	switch sale.Status {
	case "PASSED":
		return e.move(ctx, s, domain.StlAwaitDelivery, func(n *domain.Settlement) {
			now := e.Now()
			n.DecidedAt, n.DeliveryDeadlineAt = &now, parseTime(sale.DeliveryDeadlineAt)
		})
	case "FAILED":
		return e.refund(ctx, s, domain.OutcomeFailed, nil)
	case "CANCELLED":
		return e.refund(ctx, s, domain.OutcomeCancelled, nil)
	case "OPEN":
		// Đồng hồ sàn nhanh hơn peer (PP-006 chưa tới hạn): thử lại sau.
		return e.retry(ctx, s, err)
	}
	return e.fail(ctx, s, err)
}

func (e *Engine) refunding(ctx context.Context, s *domain.Settlement) error {
	if err := e.Money.Refund(ctx, s.HoldID); err != nil {
		if IsTransient(err) {
			return e.retry(ctx, s, err)
		}
		return e.fail(ctx, s, err) // 409: khoản đã CAPTURED — không bao giờ được xảy ra
	}
	lotStatus := domain.LotRefunded
	if s.Outcome == domain.OutcomeOpenRejected {
		lotStatus = domain.LotCancelled
	}
	next := *s
	now := e.Now()
	next.State, next.Attempts, next.SettledAt = domain.StlRefunded, 0, &now
	_, err := e.Store.Finish(ctx, &next, s.State, lotStatus, "", nil)
	return err
}

// ConfirmReceived: người thắng bấm "Đã nhận hàng" (docs/23 §6.1). Gọi lại khi đã
// qua bước này thì không lỗi.
func (e *Engine) ConfirmReceived(ctx context.Context, settlementID, userID string) error {
	s, err := e.Store.Get(ctx, settlementID)
	if err != nil {
		return err
	}
	if s.WinnerID != userID {
		return ErrNotWinner
	}
	switch s.State {
	case domain.StlCompleting, domain.StlSold:
		return nil
	case domain.StlAwaitDelivery:
	default:
		return ErrWrongState
	}
	now := e.Now()
	return e.move(ctx, s, domain.StlCompleting, func(n *domain.Settlement) { n.ReceivedAt = &now })
}
