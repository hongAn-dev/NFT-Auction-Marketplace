package domain

import (
	"context"
	"time"
)

type SagaState string

const (
	SagaStarted     SagaState = "STARTED"
	SagaCompleted   SagaState = "COMPLETED"
	SagaFailed      SagaState = "FAILED"
	SagaCompensated SagaState = "COMPENSATED"
)

// SagaLog lưu trữ trạng thái của Transaction phân tán để thực hiện Rollback khi lỗi
type SagaLog struct {
	ID           string    `gorm:"primaryKey;type:varchar(100)" json:"id"` // Sử dụng chính Bid ID làm Saga ID
	NFTID        string    `gorm:"type:varchar(100);index" json:"nft_id"`
	UserID       string    `gorm:"type:varchar(100)" json:"user_id"`
	Amount       float64   `json:"amount"`
	PrevPrice    float64   `json:"prev_price"`
	PrevOwnerID  string    `gorm:"type:varchar(100)" json:"prev_owner_id"`
	State        SagaState `gorm:"type:varchar(20);default:'STARTED'" json:"state"`
	ErrorMessage string    `json:"error_message"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SagaRepository định nghĩa interface thao tác dữ liệu với SagaLog
type SagaRepository interface {
	Create(ctx context.Context, log *SagaLog) error
	Update(ctx context.Context, log *SagaLog) error
	GetByID(ctx context.Context, id string) (*SagaLog, error)
}
