package repository

import (
	"context"

	"bidding-service/internal/domain"
	"gorm.io/gorm"
)

type postgresOutboxRepository struct {
	db *gorm.DB
}

// NewPostgresOutboxRepository khởi tạo Postgres implementation cho OutboxRepository
func NewPostgresOutboxRepository(db *gorm.DB) domain.OutboxRepository {
	return &postgresOutboxRepository{
		db: db,
	}
}

func (r *postgresOutboxRepository) Save(ctx context.Context, event *domain.OutboxEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *postgresOutboxRepository) GetUnprocessed(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	var events []*domain.OutboxEvent
	err := r.db.WithContext(ctx).
		Where("processed = ?", false).
		Limit(limit).
		Order("created_at asc").
		Find(&events).Error
	return events, err
}

func (r *postgresOutboxRepository) MarkProcessed(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&domain.OutboxEvent{}).
		Where("id IN ?", ids).
		Update("processed", true).Error
}

// WithTx trả về một repo hoạt động trên Transaction chỉ định của GORM
func (r *postgresOutboxRepository) WithTx(tx *gorm.DB) domain.OutboxRepository {
	return &postgresOutboxRepository{
		db: tx,
	}
}
