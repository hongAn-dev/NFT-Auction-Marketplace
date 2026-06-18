package blockchain

import (
	"context"
	"fmt"
	"log"
	"time"

	"blockchain-service/internal/domain"
)

type SyncWorker struct {
	web3Client Web3Client
	nftRepo    domain.NFTRepository
}

func NewSyncWorker(web3Client Web3Client, nftRepo domain.NFTRepository) *SyncWorker {
	return &SyncWorker{
		web3Client: web3Client,
		nftRepo:    nftRepo,
	}
}

func (s *SyncWorker) Start(ctx context.Context) {
	log.Println("🔄 Blockchain Sync Worker background process đã khởi động!")

	s.web3Client.SubscribeToTransfers(ctx, func(tokenId int64, from string, to string, price float64) {
		log.Printf("📥 Sync Worker: Nhận sự kiện Transfer On-chain. TokenID: %d, From: %s, To: %s, Price: %.2f ETH\n", tokenId, from, to, price)

		// Map tokenID xoay vòng cho dữ liệu seed
		nftID := fmt.Sprintf("lot-seed-%03d", tokenId%50+1)
		
		nft, err := s.nftRepo.GetByID(ctx, nftID)
		if err != nil {
			log.Printf("⚠️ Sync Worker: Không tìm thấy NFT ID %s trong Postgres để đồng bộ: %v\n", nftID, err)
			return
		}

		nft.OwnerID = to
		if price > 0 {
			nft.CurrentPrice = price
		}
		nft.UpdatedAt = time.Now()

		err = s.nftRepo.Update(ctx, nft)
		if err != nil {
			log.Printf("❌ Sync Worker: Lỗi cập nhật NFT %s khi đồng bộ Blockchain: %v\n", nftID, err)
			return
		}

		log.Printf("✅ Sync Worker: Đồng bộ thành công NFT %s sang chủ sở hữu ví %s trong PostgreSQL.\n", nftID, to)
	})
}
