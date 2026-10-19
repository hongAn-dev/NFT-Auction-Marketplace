package domain

import "time"

// Trạng thái lot (docs/23 §5.1).
const (
	LotActive    = "active"
	LotUnsold    = "unsold"   // hết giờ, không ai đặt giá
	LotSettling  = "settling" // đã đóng phiên, saga đang chạy
	LotSold      = "sold"
	LotRefunded  = "refunded"  // vụ bán hỏng trên chuỗi, đã hoàn tiền người thắng
	LotCancelled = "cancelled" // chuỗi từ chối mở vụ bán (ví dụ người bán không còn là chủ)
)

// Trạng thái lượt đặt giá.
const (
	BidHeld     = "HELD"     // tiền đang giữ ký quỹ
	BidReleased = "RELEASED" // bị vượt giá, đã nhả
	BidWon      = "WON"      // lượt thắng khi đóng phiên; tiền giao cho saga
)

// Bid: một lượt đặt giá. HoldID = ID, là khoá idempotency của khoản ký quỹ.
type Bid struct {
	ID        string    `gorm:"primaryKey;type:varchar(100)" json:"id"`
	NFTID     string    `gorm:"type:varchar(100);index" json:"nft_id"`
	UserID    string    `gorm:"type:varchar(100)" json:"user_id"`
	Amount    int64     `json:"amount"` // VND
	HoldID    string    `gorm:"type:varchar(100);uniqueIndex" json:"hold_id"`
	Status    string    `gorm:"type:varchar(20)" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Trạng thái saga thanh toán (docs/23 §6.1).
const (
	StlPendingOpen    = "PENDING_OPEN"
	StlAwaitAuth      = "AWAIT_AUTH"
	StlAwaitDelivery  = "AWAIT_DELIVERY"
	StlCompleting     = "COMPLETING"
	StlCancelling     = "CANCELLING"
	StlRefunding      = "REFUNDING"
	StlSold           = "SOLD"
	StlRefunded       = "REFUNDED"
	StlNeedsAttention = "NEEDS_ATTENTION"
)

// Lý do hoàn tiền — quyết định trạng thái cuối của lot.
const (
	OutcomeFailed       = "FAILED"        // giám định không đạt
	OutcomeCancelled    = "CANCELLED"     // quá hạn giám định
	OutcomeLost         = "LOST"          // giám định báo thất lạc sau PASSED
	OutcomeOpenRejected = "OPEN_REJECTED" // chuỗi từ chối OpenSale
)

// Settlement: saga thanh toán của một phiên đóng có người thắng. Bảng này chính là
// hàng đợi bền của saga: worker đọc các dòng chưa kết thúc và làm bước kế tiếp.
type Settlement struct {
	ID                 string     `gorm:"primaryKey;type:varchar(100)" json:"id"`
	LotID              string     `gorm:"type:varchar(100);uniqueIndex" json:"lot_id"`
	SellerID           string     `gorm:"type:varchar(100)" json:"seller_id"`
	WinnerID           string     `gorm:"type:varchar(100)" json:"winner_id"`
	Amount             int64      `json:"amount"`
	HoldID             string     `gorm:"type:varchar(100)" json:"hold_id"`
	AuctionRef         string     `gorm:"type:varchar(128)" json:"auction_ref"`
	SaleID             string     `gorm:"type:varchar(70)" json:"sale_id"`
	PassportID         string     `gorm:"type:varchar(70)" json:"passport_id"`
	State              string     `gorm:"type:varchar(20);index" json:"state"`
	Outcome            string     `gorm:"type:varchar(20)" json:"outcome"`
	LastError          string     `json:"last_error"`
	Attempts           int        `json:"attempts"`
	NextAttemptAt      time.Time  `gorm:"index" json:"next_attempt_at"`
	DeadlineAt         *time.Time `json:"deadline_at"`
	DeliveryDeadlineAt *time.Time `json:"delivery_deadline_at"`
	// Mốc thời gian từng bước — nguồn của bảng độ trễ E3′.
	ClosedAt    time.Time  `json:"closed_at"`
	OpenedAt    *time.Time `json:"opened_at"`
	DecidedAt   *time.Time `json:"decided_at"` // thấy PASSED / FAILED / CANCELLED
	ReceivedAt  *time.Time `json:"received_at"`
	CompletedAt *time.Time `json:"completed_at"`
	SettledAt   *time.Time `json:"settled_at"` // tiền đã capture / refund
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Terminal: saga không còn bước nào.
func (s *Settlement) Terminal() bool {
	return s.State == StlSold || s.State == StlRefunded || s.State == StlNeedsAttention
}
