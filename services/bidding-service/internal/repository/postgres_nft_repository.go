package repository

import (
	"context"
	"errors"

	"bidding-service/internal/domain"
	"gorm.io/gorm"
)

type postgresNFTRepository struct {
	db *gorm.DB
}

// NewPostgresNFTRepository khởi tạo đối tượng Repository tương tác với Postgres qua GORM.
// Hàm này trả về một đối tượng thỏa mãn interface domain.NFTRepository.
func NewPostgresNFTRepository(db *gorm.DB) domain.NFTRepository {
	return &postgresNFTRepository{
		db: db,
	}
}

func (r *postgresNFTRepository) Create(ctx context.Context, nft *domain.NFT) error {
	// GORM tự động tạo câu lệnh INSERT INTO nfts (...) VALUES (...)
	// Sử dụng WithContext để truyền context đi suốt quá trình truy vấn (để hỗ trợ timeout/cancel)
	result := r.db.WithContext(ctx).Create(nft)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (r *postgresNFTRepository) GetByID(ctx context.Context, id string) (*domain.NFT, error) {
	var nft domain.NFT
	// SELECT * FROM nfts WHERE id = ? LIMIT 1
	result := r.db.WithContext(ctx).First(&nft, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNFTNotFound
		}
		return nil, result.Error
	}
	return &nft, nil
}

func (r *postgresNFTRepository) GetAll(ctx context.Context) ([]*domain.NFT, error) {
	var nfts []*domain.NFT
	// SELECT * FROM nfts
	result := r.db.WithContext(ctx).Find(&nfts)
	if result.Error != nil {
		return nil, result.Error
	}
	return nfts, nil
}

func (r *postgresNFTRepository) Update(ctx context.Context, nft *domain.NFT) error {
	// Save sẽ thực hiện câu lệnh UPDATE nfts SET ... WHERE id = ...
	// Nếu bản ghi chưa tồn tại, nó sẽ thực hiện INSERT. Tuy nhiên usecase của chúng ta đã kiểm tra tồn tại rồi.
	result := r.db.WithContext(ctx).Save(nft)
	if result.Error != nil {
		return result.Error
	}
	// Kiểm tra xem có dòng nào thực tế bị tác động không
	if result.RowsAffected == 0 {
		return domain.ErrNFTNotFound
	}
	return nil
}

func (r *postgresNFTRepository) Delete(ctx context.Context, id string) error {
	// DELETE FROM nfts WHERE id = ?
	result := r.db.WithContext(ctx).Delete(&domain.NFT{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNFTNotFound
	}
	return nil
}
