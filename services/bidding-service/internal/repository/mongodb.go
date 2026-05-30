package repository

import (
	"context"
	"fmt"
	"os"
	"time"

	"bidding-service/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoMetadataRepository struct {
	collection *mongo.Collection
}

// InitMongoDB khởi tạo kết nối MongoDB và thiết lập Database, Collection mặc định.
func InitMongoDB(ctx context.Context) (*mongo.Client, *mongo.Collection, error) {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	dbName := os.Getenv("MONGO_DB")
	if dbName == "" {
		dbName = "nft_metadata"
	}

	fmt.Printf("🔍 Đang kết nối tới MongoDB tại: %s...\n", uri)

	// Thiết lập các tùy chọn kết nối (Connection Pool tối đa 100 kết nối nhàn rỗi)
	clientOptions := options.Client().ApplyURI(uri).SetMaxPoolSize(100)

	// Mở kết nối đến MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("không thể kết nối MongoDB: %w", err)
	}

	// Ping kiểm tra xem MongoDB thực sự hoạt động
	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("không thể ping tới MongoDB: %w", err)
	}

	collection := client.Database(dbName).Collection("nft_metadata")
	fmt.Println("✅ Kết nối MongoDB thành công và cấu hình Collection 'nft_metadata' hoàn tất!")

	return client, collection, nil
}

// NewMongoMetadataRepository tạo đối tượng repository MongoDB thỏa mãn interface domain.NFTMetadataRepository
func NewMongoMetadataRepository(collection *mongo.Collection) domain.NFTMetadataRepository {
	return &mongoMetadataRepository{
		collection: collection,
	}
}

func (r *mongoMetadataRepository) Upsert(ctx context.Context, metadata *domain.NFTMetadata) error {
	filter := bson.M{"nft_id": metadata.NFTID}

	// Lệnh Upsert: Cập nhật tài liệu cũ nếu tìm thấy, ngược lại chèn mới tài liệu
	opts := options.Update().SetUpsert(true)
	update := bson.M{
		"$set": bson.M{
			"nft_id":      metadata.NFTID,
			"rarity":      metadata.Rarity,
			"attributes":  metadata.Attributes,
			"created_by":  metadata.CreatedBy,
			"ipfs_hash":   metadata.IPFSHash,
			"updated_at":  time.Now(),
		},
		"$setOnInsert": bson.M{
			"created_at": time.Now(),
		},
	}

	_, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("lỗi Upsert MongoDB: %w", err)
	}

	return nil
}

func (r *mongoMetadataRepository) GetByNFTID(ctx context.Context, nftID string) (*domain.NFTMetadata, error) {
	filter := bson.M{"nft_id": nftID}

	var metadata domain.NFTMetadata
	err := r.collection.FindOne(ctx, filter).Decode(&metadata)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Trả về nil nếu chưa có metadata trong MongoDB
		}
		return nil, fmt.Errorf("lỗi FindOne MongoDB: %w", err)
	}

	return &metadata, nil
}

func (r *mongoMetadataRepository) Delete(ctx context.Context, nftID string) error {
	filter := bson.M{"nft_id": nftID}

	_, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("lỗi DeleteOne MongoDB: %w", err)
	}

	return nil
}
