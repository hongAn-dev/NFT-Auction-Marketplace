package repository

import (
	"context"

	"bidding-service/internal/domain"
	"gorm.io/gorm"
)

type postgresSagaRepository struct {
	db *gorm.DB
}

// NewPostgresSagaRepository khởi tạo SagaRepository tương tác Postgres qua GORM
func NewPostgresSagaRepository(db *gorm.DB) domain.SagaRepository {
	return &postgresSagaRepository{
		db: db,
	}
}

func (r *postgresSagaRepository) Create(ctx context.Context, logRecord *domain.SagaLog) error {
	return r.db.WithContext(ctx).Create(logRecord).Error
}

func (r *postgresSagaRepository) Update(ctx context.Context, logRecord *domain.SagaLog) error {
	return r.db.WithContext(ctx).Save(logRecord).Error
}

func (r *postgresSagaRepository) GetByID(ctx context.Context, id string) (*domain.SagaLog, error) {
	var logRecord domain.SagaLog
	err := r.db.WithContext(ctx).First(&logRecord, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &logRecord, nil
}

// WithTx trả về repo hoạt động trên Transaction chỉ định của GORM
func (r *postgresSagaRepository) WithTx(tx *gorm.DB) domain.SagaRepository {
	return &postgresSagaRepository{
		db: tx,
	}
}
