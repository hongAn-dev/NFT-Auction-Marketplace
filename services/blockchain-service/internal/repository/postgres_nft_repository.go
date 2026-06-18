package repository

import (
	"context"

	"blockchain-service/internal/domain"
	"gorm.io/gorm"
)

type postgresNFTRepository struct {
	db *gorm.DB
}

// NewPostgresNFTRepository khởi tạo Postgres NFT Repository cho blockchain-service
func NewPostgresNFTRepository(db *gorm.DB) domain.NFTRepository {
	return &postgresNFTRepository{
		db: db,
	}
}

func (r *postgresNFTRepository) GetByID(ctx context.Context, id string) (*domain.NFT, error) {
	var nft domain.NFT
	err := r.db.WithContext(ctx).First(&nft, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &nft, nil
}

func (r *postgresNFTRepository) Update(ctx context.Context, nft *domain.NFT) error {
	return r.db.WithContext(ctx).Save(nft).Error
}
