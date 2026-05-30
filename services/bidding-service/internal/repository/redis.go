package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"bidding-service/internal/domain"
	"github.com/redis/go-redis/v9"
)

var (
	ErrLockNotAcquired = errors.New("could not acquire distributed lock")
)

// RedisRepository đại diện cho cổng giao tiếp và các thao tác trên bộ nhớ đệm Redis
type RedisRepository struct {
	client *redis.Client
}

// InitRedis khởi tạo kết nối và cấu hình Redis Client cục bộ
func InitRedis() (*RedisRepository, error) {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	fmt.Printf("🔍 Đang kết nối tới Redis tại: %s...\n", addr)

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // Không dùng mật khẩu ở chế độ dev
		DB:       0,  // DB mặc định
		PoolSize: 20, // Số lượng kết nối duy trì tối đa trong pool
	})

	// Thử gửi lệnh PING để kiểm tra xem Redis đã thực sự hoạt động chưa
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.Ping(ctx).Err()
	if err != nil {
		return nil, fmt.Errorf("không thể ping tới Redis: %w", err)
	}

	fmt.Println("✅ Kết nối Redis thành công!")
	return &RedisRepository{client: client}, nil
}

// ── REDIS CACHING (CACHE-ASIDE PATTERN) ─────────────────────────

// GetNFTCache lấy thông tin NFT từ Redis.
// Trả về nil nếu bị Cache Miss (Không tìm thấy).
func (r *RedisRepository) GetNFTCache(ctx context.Context, id string) (*domain.NFT, error) {
	key := fmt.Sprintf("cache:nft:%s", id)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache Miss
		}
		return nil, err
	}

	// Đọc thành công (Cache Hit), tiến hành giải tuần tự hóa (deserialize) từ chuỗi JSON
	var nft domain.NFT
	err = json.Unmarshal([]byte(val), &nft)
	if err != nil {
		return nil, fmt.Errorf("lỗi unmarshal cache: %w", err)
	}

	return &nft, nil
}

// SetNFTCache lưu thông tin NFT vào Redis với thời gian sống (TTL).
func (r *RedisRepository) SetNFTCache(ctx context.Context, nft *domain.NFT, ttl time.Duration) error {
	key := fmt.Sprintf("cache:nft:%s", nft.ID)

	// Tuần tự hóa đối tượng struct Go thành chuỗi bytes JSON để lưu vào Redis
	data, err := json.Marshal(nft)
	if err != nil {
		return fmt.Errorf("lỗi marshal cache: %w", err)
	}

	err = r.client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("lỗi ghi cache vào Redis: %w", err)
	}

	return nil
}

// DeleteNFTCache xóa cache của một NFT.
// Được gọi khi dữ liệu thay đổi (Cache Eviction) để chống lỗi dữ liệu cũ (Stale Data).
func (r *RedisRepository) DeleteNFTCache(ctx context.Context, id string) error {
	key := fmt.Sprintf("cache:nft:%s", id)
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("lỗi xóa cache trên Redis: %w", err)
	}
	return nil
}

// ── REDIS DISTRIBUTED LOCK (KHÓA PHÂN TÁN SETNX) ──────────────────

// AcquireLock thử giành Khóa phân tán sử dụng câu lệnh SET với cờ NX (Not Exists).
// Trả về true nếu giành khóa thành công, false nếu khóa đang bị chiếm.
func (r *RedisRepository) AcquireLock(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	// value đại diện cho định danh của lock. Ở đây dùng UnixNano đơn giản.
	value := fmt.Sprintf("%d", time.Now().UnixNano())

	// Lệnh SetNX tương đương: SET key value NX PX <ttl_ms>
	// NX: Chỉ thiết lập nếu khóa chưa tồn tại
	// ttl: Tự giải phóng khóa sau thời gian này (tránh deadlock nếu app bị sập)
	success, err := r.client.SetNX(ctx, lockKey, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("lỗi thực thi SetNX trên Redis: %w", err)
	}

	return success, nil
}

// ReleaseLock giải phóng Khóa phân tán bằng cách xóa key tương ứng trên Redis.
func (r *RedisRepository) ReleaseLock(ctx context.Context, lockKey string) error {
	err := r.client.Del(ctx, lockKey).Err()
	if err != nil {
		return fmt.Errorf("lỗi giải phóng khóa phân tán: %w", err)
	}
	return nil
}

// ── REDIS PUB/SUB BRIDGE ─────────────────────────────────────────

// PublishBidEvent phát một sự kiện đặt thầu lên Redis Pub/Sub làm cầu nối sang NestJS WebSocket Gateway.
func (r *RedisRepository) PublishBidEvent(ctx context.Context, nftId string, eventData interface{}) error {
	channel := fmt.Sprintf("bid:updated:%s", nftId)
	data, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("lỗi marshal dữ liệu sự kiện: %w", err)
	}

	err = r.client.Publish(ctx, channel, data).Err()
	if err != nil {
		return fmt.Errorf("lỗi phát sự kiện lên Redis Pub/Sub: %w", err)
	}

	fmt.Printf("📡 Redis Pub/Sub: Đã phát sự kiện thầu mới lên kênh '%s'\n", channel)
	return nil
}

