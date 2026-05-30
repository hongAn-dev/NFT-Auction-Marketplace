package usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	"bidding-service/internal/domain"
	"bidding-service/internal/repository"
)

type nftUsecase struct {
	nftRepo   domain.NFTRepository
	redisRepo *repository.RedisRepository
}

// NewNFTUsecase khởi tạo đối tượng nftUsecase nhận cả Postgres Repo và Redis Repo làm tham số.
func NewNFTUsecase(repo domain.NFTRepository, redisRepo *repository.RedisRepository) domain.NFTUsecase {
	return &nftUsecase{
		nftRepo:   repo,
		redisRepo: redisRepo,
	}
}

func (u *nftUsecase) CreateNFT(ctx context.Context, nft *domain.NFT) (*domain.NFT, error) {
	if nft.Title == "" || nft.StartPrice <= 0 {
		return nil, domain.ErrInvalidNFTData
	}

	nft.CreatedAt = time.Now()
	nft.UpdatedAt = time.Now()
	nft.CurrentPrice = nft.StartPrice
	if nft.Status == "" {
		nft.Status = "active"
	}

	err := u.nftRepo.Create(ctx, nft)
	if err != nil {
		return nil, err
	}

	// Khi tạo mới, ta có thể ghi vào cache ngay hoặc đợi đến lần GET đầu tiên.
	// Ở đây ta ghi vào cache luôn với TTL = 5 phút để tối ưu hóa!
	if u.redisRepo != nil {
		_ = u.redisRepo.SetNFTCache(ctx, nft, 5*time.Minute)
	}

	return nft, nil
}

func (u *nftUsecase) GetNFT(ctx context.Context, id string) (*domain.NFT, error) {
	// 1. Thử lấy dữ liệu từ Redis Cache trước (Cache-Aside Pattern)
	if u.redisRepo != nil {
		cachedNFT, err := u.redisRepo.GetNFTCache(ctx, id)
		if err == nil && cachedNFT != nil {
			fmt.Printf("⚡ Cache Hit: Lấy thông tin NFT %s thành công trực tiếp từ REDIS!\n", id)
			return cachedNFT, nil
		}
		if err != nil {
			log.Printf("⚠️ Lỗi đọc Cache Redis: %v\n", err)
		}
	}

	// 2. Cache Miss: Không có trong Redis, truy vấn trực tiếp từ PostgreSQL
	fmt.Printf("🔍 Cache Miss: Đang truy vấn NFT %s từ CSDL PostgreSQL...\n", id)
	nft, err := u.nftRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. Đọc DB thành công, ghi ngược lại dữ liệu vào Redis Cache với TTL = 5 phút
	if u.redisRepo != nil {
		err = u.redisRepo.SetNFTCache(ctx, nft, 5*time.Minute)
		if err != nil {
			log.Printf("⚠️ Lỗi ghi Cache Redis: %v\n", err)
		}
	}

	return nft, nil
}

func (u *nftUsecase) ListNFTs(ctx context.Context) ([]*domain.NFT, error) {
	// Đọc danh sách NFT tạm thời truy vấn trực tiếp từ DB để đảm bảo tính nhất quán chung
	return u.nftRepo.GetAll(ctx)
}

func (u *nftUsecase) UpdateNFT(ctx context.Context, id string, updatedData *domain.NFT) (*domain.NFT, error) {
	existing, err := u.nftRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if updatedData.Title != "" {
		existing.Title = updatedData.Title
	}
	if updatedData.Description != "" {
		existing.Description = updatedData.Description
	}
	if updatedData.ImageUrl != "" {
		existing.ImageUrl = updatedData.ImageUrl
	}
	if updatedData.Status != "" {
		existing.Status = updatedData.Status
	}
	if updatedData.CurrentPrice > 0 {
		existing.CurrentPrice = updatedData.CurrentPrice
	}
	if updatedData.OwnerID != "" {
		existing.OwnerID = updatedData.OwnerID
	}
	existing.UpdatedAt = time.Now()

	err = u.nftRepo.Update(ctx, existing)
	if err != nil {
		return nil, err
	}

	// ── XÓA CACHE CŨ (CACHE EVICTION) ─────────────────────────────
	// Bắt buộc phải xóa cache cũ trên Redis để tránh lỗi Stale Data (khách hàng đọc giá thầu cũ)
	if u.redisRepo != nil {
		err = u.redisRepo.DeleteNFTCache(ctx, id)
		if err != nil {
			log.Printf("⚠️ Lỗi xóa Cache Redis: %v\n", err)
		}
	}

	return existing, nil
}

func (u *nftUsecase) DeleteNFT(ctx context.Context, id string) error {
	err := u.nftRepo.Delete(ctx, id)
	if err != nil {
		return err
	}

	// Xóa cache khi xóa NFT
	if u.redisRepo != nil {
		_ = u.redisRepo.DeleteNFTCache(ctx, id)
	}

	return nil
}
