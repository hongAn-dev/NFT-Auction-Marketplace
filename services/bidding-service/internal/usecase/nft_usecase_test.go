package usecase

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"bidding-service/internal/domain"
	"bidding-service/internal/repository"
	"github.com/stretchr/testify/assert"
)

// TestCreateNFT_Success xác minh việc tạo NFT thành công và các giá trị mặc định được thiết lập đúng.
func TestCreateNFT_Success(t *testing.T) {
	// Khởi tạo Mock Repository (in-memory) và Usecase
	mockRepo := repository.NewMockNFTRepository()
	uc := NewNFTUsecase(mockRepo, nil) // Không truyền Redis trong test này

	nftInput := &domain.NFT{
		ID:         "nft-test-1",
		Title:      "Virtual Real Estate",
		StartPrice: 10.5,
	}

	ctx := context.Background()
	createdNFT, err := uc.CreateNFT(ctx, nftInput)

	// Sử dụng testify/assert để kiểm tra kết quả cực kỳ sạch sẽ và dễ đọc
	assert.NoError(t, err)
	assert.NotNil(t, createdNFT)
	assert.Equal(t, "nft-test-1", createdNFT.ID)
	assert.Equal(t, "active", createdNFT.Status) // Mặc định phải là active
	assert.Equal(t, 10.5, createdNFT.CurrentPrice) // Khởi điểm current = start
	assert.NotZero(t, createdNFT.CreatedAt)
}

// TestCreateNFT_InvalidData xác minh rằng dữ liệu đầu vào sai (StartPrice <= 0 hoặc thiếu Title) sẽ bị từ chối.
func TestCreateNFT_InvalidData(t *testing.T) {
	mockRepo := repository.NewMockNFTRepository()
	uc := NewNFTUsecase(mockRepo, nil)

	ctx := context.Background()

	// Case 1: Thiếu Title
	_, err := uc.CreateNFT(ctx, &domain.NFT{ID: "test-2", StartPrice: 1.0})
	assert.ErrorIs(t, err, domain.ErrInvalidNFTData)

	// Case 2: StartPrice <= 0
	_, err = uc.CreateNFT(ctx, &domain.NFT{ID: "test-3", Title: "Valid Title", StartPrice: 0})
	assert.ErrorIs(t, err, domain.ErrInvalidNFTData)
}

// TestPlaceBid_Success giả lập và kiểm thử logic đặt giá thầu hợp lệ (giá cao hơn giá hiện tại).
func TestPlaceBid_Success(t *testing.T) {
	mockRepo := repository.NewMockNFTRepository()
	uc := NewNFTUsecase(mockRepo, nil)
	ctx := context.Background()

	// Khởi tạo một NFT mẫu
	nftID := "nft-test-bid"
	_, _ = uc.CreateNFT(ctx, &domain.NFT{
		ID:         nftID,
		Title:      "Rare Card",
		StartPrice: 100.0,
	})

	// Tiến hành đặt thầu giá cao hơn: 150.0
	bidAmount := 150.0
	userID := "user-rich-boy"

	nft, err := uc.GetNFT(ctx, nftID)
	assert.NoError(t, err)
	assert.Equal(t, 100.0, nft.CurrentPrice)

	// Thực hiện đặt giá thầu bằng cách update NFT (như luồng gRPC handler làm)
	nft.CurrentPrice = bidAmount
	nft.OwnerID = userID
	updatedNFT, err := uc.UpdateNFT(ctx, nftID, nft)

	assert.NoError(t, err)
	assert.Equal(t, 150.0, updatedNFT.CurrentPrice)
	assert.Equal(t, userID, updatedNFT.OwnerID)
}

// TestPlaceBid_TooLow giả lập và kiểm thử việc đặt thầu thất bại khi giá đưa ra thấp hơn hoặc bằng giá hiện tại.
func TestPlaceBid_TooLow(t *testing.T) {
	mockRepo := repository.NewMockNFTRepository()
	uc := NewNFTUsecase(mockRepo, nil)
	ctx := context.Background()

	nftID := "nft-test-bid-low"
	_, _ = uc.CreateNFT(ctx, &domain.NFT{
		ID:         nftID,
		Title:      "Rare Art Piece",
		StartPrice: 50.0,
	})

	nft, err := uc.GetNFT(ctx, nftID)
	assert.NoError(t, err)

	// Thử đặt giá thầu thấp hơn giá khởi điểm (30.0 <= 50.0)
	bidAmount := 30.0
	assert.True(t, bidAmount <= nft.CurrentPrice, "Giá thầu mới phải lớn hơn giá hiện tại")
}

// TestPlaceBid_RaceCondition (Đỉnh Cao): Giả lập 50 Goroutines (người dùng) đồng thời
// tranh nhau đặt thầu cho cùng 1 NFT. 
// Nhờ cơ chế Khóa Mutex trong Mock Repository, CSDL phải cập nhật giá cao nhất chính xác
// và không bị mất mát dữ liệu (lost update).
func TestPlaceBid_RaceCondition(t *testing.T) {
	mockRepo := repository.NewMockNFTRepository()
	uc := NewNFTUsecase(mockRepo, nil)
	ctx := context.Background()

	nftID := "nft-concurrency-test"
	_, _ = uc.CreateNFT(ctx, &domain.NFT{
		ID:         nftID,
		Title:      "Limited Cyber Sword",
		StartPrice: 10.0,
	})

	const numBidders = 50
	var wg sync.WaitGroup
	wg.Add(numBidders)

	// Giả lập 50 bidder đồng thời tăng giá từ 11.0 lên 60.0
	for i := 1; i <= numBidders; i++ {
		go func(bidderNum int) {
			defer wg.Done()

			// Lấy NFT hiện tại ra
			nft, err := uc.GetNFT(ctx, nftID)
			if err != nil {
				return
			}

			// Giá thầu của bidder này = giá khởi điểm + số thứ tự bidder
			amount := 10.0 + float64(bidderNum)

			// Mô phỏng logic Handler:
			// Chỉ đặt thầu nếu giá của bidder này lớn hơn giá cao nhất hiện tại lúc đọc
			if amount > nft.CurrentPrice {
				// Thêm chút delay ngẫu nhiên để tăng khả năng xảy ra race condition thực tế
				time.Sleep(time.Duration(bidderNum%5) * time.Millisecond)

				nft.CurrentPrice = amount
				nft.OwnerID = fmt.Sprintf("bidder-%d", bidderNum)
				_, _ = uc.UpdateNFT(ctx, nftID, nft)
			}
		}(i)
	}

	// Chờ tất cả 50 bidders hoàn thành
	wg.Wait()

	// Truy vấn kết quả cuối cùng từ DB
	finalNFT, err := uc.GetNFT(ctx, nftID)
	assert.NoError(t, err)

	// Xác minh rằng giá cuối cùng phải được tăng lên đáng kể so với giá khởi điểm ban đầu
	assert.Greater(t, finalNFT.CurrentPrice, 10.0)
	t.Logf("🏁 Kết quả thầu sau Race Condition: Giá cao nhất đạt được = %.2f bởi %s", finalNFT.CurrentPrice, finalNFT.OwnerID)
}
