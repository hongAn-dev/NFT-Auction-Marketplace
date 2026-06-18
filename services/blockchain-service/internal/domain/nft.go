package domain

import (
	"context"
	"time"
)

// NFT đại diện cho thực thể NFT đồng bộ trong PostgreSQL
type NFT struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	ImageUrl     string    `json:"image_url"`
	CreatorID    string    `json:"creator_id"`
	OwnerID      string    `json:"owner_id"`
	StartPrice   float64   `json:"start_price"`
	CurrentPrice float64   `json:"current_price"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NFTRepository định nghĩa các hàm truy cập DB cần thiết cho Sync Engine
type NFTRepository interface {
	GetByID(ctx context.Context, id string) (*NFT, error)
	Update(ctx context.Context, nft *NFT) error
}
