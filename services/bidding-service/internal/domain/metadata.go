package domain

import (
	"context"
	"time"
)

// NFTMetadata đại diện cho thông tin thuộc tính động, phi cấu trúc của NFT.
// Các trường này có cấu trúc thay đổi tùy thuộc bộ sưu tập, cực kỳ thích hợp lưu trữ NoSQL MongoDB.
type NFTMetadata struct {
	NFTID       string                 `json:"nft_id" bson:"nft_id"`             // Khóa liên kết với bảng nfts trong Postgres
	Rarity      string                 `json:"rarity" bson:"rarity"`             // Độ hiếm: "Common", "Rare", "Epic", "Legendary"
	Attributes  map[string]interface{} `json:"attributes" bson:"attributes"`     // Các thuộc tính động dạng Key-Value (ví dụ: background: red, power: 100)
	CreatedBy   string                 `json:"created_by" bson:"created_by"`     // Người sáng tạo
	IPFSHash    string                 `json:"ipfs_hash" bson:"ipfs_hash"`       // Địa chỉ lưu trữ tệp phi tập trung (IPFS)
	CreatedAt   time.Time              `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" bson:"updated_at"`
}

// NFTMetadataRepository định nghĩa cổng giao tiếp lưu trữ NFT Metadata trên MongoDB.
type NFTMetadataRepository interface {
	Upsert(ctx context.Context, metadata *NFTMetadata) error
	GetByNFTID(ctx context.Context, nftID string) (*NFTMetadata, error)
	Delete(ctx context.Context, nftID string) error
}
