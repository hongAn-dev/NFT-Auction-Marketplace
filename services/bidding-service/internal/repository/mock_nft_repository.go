package repository

import (
	"context"
	"sync"
	"time"

	"bidding-service/internal/domain"
)

// mockNFTRepository là cấu trúc lưu trữ dữ liệu NFT trong bộ nhớ tạm thời (RAM).
// Vì ứng dụng Go xử lý bất đồng bộ (Concurrent), chúng ta bắt buộc sử dụng sync.RWMutex
// để khóa (lock) dữ liệu khi ghi, tránh xung đột dữ liệu (data race).
type mockNFTRepository struct {
	mu   sync.RWMutex
	nfts map[string]*domain.NFT
}

// NewMockNFTRepository là một Constructor function khởi tạo đối tượng mockNFTRepository.
// Hàm này trả về một đối tượng thỏa mãn interface domain.NFTRepository.
func NewMockNFTRepository() domain.NFTRepository {
	// Seed một số dữ liệu mẫu ban đầu để dễ dàng test
	initialNFTs := make(map[string]*domain.NFT)
	initialNFTs["nft-1"] = &domain.NFT{
		ID:           "nft-1",
		Title:        "Cyber Punk Ninja",
		Description:  "A legendary digital warrior from the year 2077.",
		ImageUrl:     "https://assets.nft.com/cyber-punk-ninja.png",
		CreatorID:    "creator-101",
		OwnerID:      "creator-101",
		StartPrice:   1.5,
		CurrentPrice: 1.5,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	initialNFTs["nft-2"] = &domain.NFT{
		ID:           "nft-2",
		Title:        "Golden Ethereum Crown",
		Description:  "A gorgeous dynamic crown that reacts to market prices.",
		ImageUrl:     "https://assets.nft.com/eth-crown.png",
		CreatorID:    "creator-102",
		OwnerID:      "creator-102",
		StartPrice:   5.0,
		CurrentPrice: 5.5,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	return &mockNFTRepository{
		nfts: initialNFTs,
	}
}

func (r *mockNFTRepository) Create(ctx context.Context, nft *domain.NFT) error {
	r.mu.Lock()         // Chiếm quyền khóa ghi độc quyền
	defer r.mu.Unlock() // Tự động mở khóa khi hàm kết thúc (defer)

	if _, exists := r.nfts[nft.ID]; exists {
		return domain.ErrInvalidNFTData
	}

	r.nfts[nft.ID] = nft
	return nil
}

func (r *mockNFTRepository) GetByID(ctx context.Context, id string) (*domain.NFT, error) {
	r.mu.RLock()         // Khóa đọc (cho phép nhiều goroutine đọc đồng thời nhưng chặn ghi)
	defer r.mu.RUnlock() // Mở khóa đọc

	nft, exists := r.nfts[id]
	if !exists {
		return nil, domain.ErrNFTNotFound
	}
	return nft, nil
}

func (r *mockNFTRepository) GetAll(ctx context.Context) ([]*domain.NFT, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*domain.NFT, 0, len(r.nfts))
	for _, nft := range r.nfts {
		list = append(list, nft)
	}
	return list, nil
}

func (r *mockNFTRepository) Update(ctx context.Context, nft *domain.NFT) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nfts[nft.ID]; !exists {
		return domain.ErrNFTNotFound
	}

	r.nfts[nft.ID] = nft
	return nil
}

func (r *mockNFTRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nfts[id]; !exists {
		return domain.ErrNFTNotFound
	}

	delete(r.nfts, id)
	return nil
}
