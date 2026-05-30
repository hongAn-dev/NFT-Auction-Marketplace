package domain

import (
	"context"
	"errors"
	"time"
)

// Khai báo các lỗi nghiệp vụ (Business Errors) liên quan đến NFT
var (
	ErrNFTNotFound   = errors.New("nft not found")
	ErrInvalidNFTData = errors.New("invalid nft data")
)

// NFT đại diện cho một vật phẩm mã hóa trên hệ thống đấu giá.
// Chúng ta sử dụng struct để mô hình hóa thực thể (Entity) này.
type NFT struct {
	ID          string    `json:"id"`          // Mã định danh duy nhất (UUID dạng string)
	Title       string    `json:"title"`       // Tên của tác phẩm NFT
	Description string    `json:"description"` // Mô tả chi tiết về tác phẩm
	ImageUrl    string    `json:"image_url"`   // Đường dẫn ảnh NFT (thường lưu ở S3)
	CreatorID   string    `json:"creator_id"`  // Mã người sáng tạo ra NFT
	OwnerID     string    `json:"owner_id"`    // Mã người sở hữu hiện tại
	StartPrice  float64   `json:"start_price"` // Giá khởi điểm đấu giá
	CurrentPrice float64  `json:"current_price"` // Giá hiện tại (giá thầu cao nhất)
	Status      string    `json:"status"`      // Trạng thái: "draft", "active", "sold", "canceled"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NFTRepository là một Interface (Cổng giao tiếp dữ liệu).
// Lớp Usecase (Business Logic) sẽ chỉ gọi qua Interface này mà KHÔNG cần quan tâm
// dữ liệu được lấy từ đâu (Postgres, MongoDB, hay bộ nhớ RAM).
// Điều này giúp tách biệt hoàn toàn Logic nghiệp vụ khỏi Công nghệ lưu trữ (Database).
type NFTRepository interface {
	Create(ctx context.Context, nft *NFT) error
	GetByID(ctx context.Context, id string) (*NFT, error)
	GetAll(ctx context.Context) ([]*NFT, error)
	Update(ctx context.Context, nft *NFT) error
	Delete(ctx context.Context, id string) error
}

// NFTUsecase định nghĩa các hành động nghiệp vụ mà Client có thể thực hiện đối với NFT.
type NFTUsecase interface {
	CreateNFT(ctx context.Context, nft *NFT) (*NFT, error)
	GetNFT(ctx context.Context, id string) (*NFT, error)
	ListNFTs(ctx context.Context) ([]*NFT, error)
	UpdateNFT(ctx context.Context, id string, nft *NFT) (*NFT, error)
	DeleteNFT(ctx context.Context, id string) error
}
