package domain

import (
	"context"
	"time"
)

// OutboxEvent lưu trữ sự kiện cần phát trong cùng một Transaction với thay đổi DB chính
type OutboxEvent struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Topic     string    `gorm:"not null" json:"topic"` // VD: "bid.placed", "nft.purchased", "redis:bid:updated"
	Payload   []byte    `gorm:"type:text;not null" json:"payload"`
	Processed bool      `gorm:"default:false;index" json:"processed"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OutboxRepository định nghĩa cổng giao tiếp dữ liệu cho OutboxEvent
type OutboxRepository interface {
	Save(ctx context.Context, event *OutboxEvent) error
	GetUnprocessed(ctx context.Context, limit int) ([]*OutboxEvent, error)
	MarkProcessed(ctx context.Context, ids []uint) error
}
