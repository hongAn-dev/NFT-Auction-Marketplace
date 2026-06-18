package repository

import (
	"fmt"
	"os"
	"time"

	"bidding-service/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB khởi tạo kết nối cơ sở dữ liệu PostgreSQL thông qua GORM.
// Nó đọc cấu hình từ biến môi trường và thiết lập Connection Pool tiêu chuẩn.
func InitDB() (*gorm.DB, error) {
	// Đọc cấu hình từ biến môi trường (Environment Variables) hoặc dùng giá trị mặc định
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "nft_user")
	password := getEnv("DB_PASSWORD", "secret_password")
	dbname := getEnv("DB_NAME", "nft_auction")

	// Xây dựng chuỗi kết nối DSN (Data Source Name)
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Ho_Chi_Minh",
		host, user, password, dbname, port,
	)

	fmt.Printf("🔍 Đang kết nối tới PostgreSQL tại: %s:%s...\n", host, port)

	// Cấu hình Logger cho GORM để in log các câu truy vấn SQL ra console khi chạy ở chế độ dev
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	}

	// Thực hiện mở kết nối đến CSDL
	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối tới cơ sở dữ liệu: %w", err)
	}

	// ── Thiết lập Connection Pool ─────────────────────────────────
	// GORM sử dụng database/sql pool của Go ở bên dưới
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("không thể lấy sql.DB từ GORM: %w", err)
	}

	// SetMaxIdleConns thiết lập số lượng tối đa các kết nối nhàn rỗi trong pool.
	sqlDB.SetMaxIdleConns(10)

	// SetMaxOpenConns thiết lập số lượng kết nối tối đa được mở tới CSDL cùng lúc.
	sqlDB.SetMaxOpenConns(50)

	// SetConnMaxLifetime thiết lập thời gian tối đa một kết nối có thể tồn tại trước khi bị đóng và tái tạo.
	sqlDB.SetConnMaxLifetime(time.Hour)

	fmt.Println("✅ Kết nối PostgreSQL thành công và cấu hình Connection Pool hoàn tất!")

	// ── Auto Migration (Tự động tạo/cập nhật cấu trúc bảng) ──────
	// GORM sẽ quét qua struct domain.NFT và tự động tạo bảng "nfts" với đầy đủ các cột tương ứng.
	// Bạn không cần viết bất kỳ lệnh CREATE TABLE nào bằng SQL thô!
	fmt.Println("🔧 Đang đồng bộ hóa cấu trúc bảng (AutoMigrate)...")
	err = db.AutoMigrate(&domain.NFT{}, &domain.OutboxEvent{}, &domain.SagaLog{})
	if err != nil {
		return nil, fmt.Errorf("lỗi trong quá trình AutoMigrate: %w", err)
	}
	fmt.Println("✅ Đồng bộ hóa cấu trúc bảng thành công!")

	// Thực hiện gieo mầm dữ liệu (Seeding) 50 tác phẩm nghệ thuật
	SeedNFTs(db)

	return db, nil
}

// SeedNFTs gieo mầm dữ liệu 50 tác phẩm nghệ thuật đẹp mắt vào cơ sở dữ liệu nếu bảng trống.
func SeedNFTs(db *gorm.DB) {
	var count int64
	db.Model(&domain.NFT{}).Count(&count)
	if count > 0 {
		fmt.Printf("ℹ️ CSDL đã có sẵn %d tác phẩm. Bỏ qua bước Seeding.\n", count)
		return
	}

	fmt.Println("🌱 Đang thực hiện gieo mầm dữ liệu (Seeding) 50 tác phẩm nghệ thuật cao cấp...")

	images := []string{
		"https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?q=80&w=600&auto=format&fit=crop",
		"https://images.unsplash.com/photo-1604871000636-074fa5117945?q=80&w=600&auto=format&fit=crop",
		"https://images.unsplash.com/photo-1579783900882-c0d3dad7b119?q=80&w=600&auto=format&fit=crop",
		"https://images.unsplash.com/photo-1541701494587-cb58502866ab?q=80&w=600&auto=format&fit=crop",
		"https://images.unsplash.com/photo-1550684848-fac1c5b4e853?q=80&w=600&auto=format&fit=crop",
		"https://images.unsplash.com/photo-1563089145-599997674d42?q=80&w=600&auto=format&fit=crop",
		"https://images.unsplash.com/photo-1634017839464-5c339ebe3cb4?q=80&w=600&auto=format&fit=crop",
	}

	titles := []string{"Brutalist", "Monochrome", "Geometric", "Abstract", "Ethereal", "Minimalist", "Linear", "Prismatic", "Tension", "Fluid"}
	nouns := []string{"Structure", "Tension", "Space", "Monolith", "Canvas", "Dynamic", "Shadow", "Harmony", "Fragment", "Vortex"}
	artists := []string{"architect_de_ruyter", "lyra_studio", "code_sculptor", "monolith_labs", "brutalist_mind"}

	for i := 1; i <= 50; i++ {
		title := fmt.Sprintf("%s %s No. %d", titles[i%len(titles)], nouns[i%len(nouns)], i)
		description := fmt.Sprintf("Lot %d: Một tác phẩm nghệ thuật kỹ thuật số đỉnh cao được giám tuyển kỹ lưỡng, khai thác các chủ đề về sự tương tác giữa %s %s và không gian hình học đa chiều.", i, titles[i%len(titles)], nouns[i%len(nouns)])
		imageUrl := images[i%len(images)]
		creator := artists[i%len(artists)]
		startPrice := float64(450 + (i*125)%2500)

		nft := domain.NFT{
			ID:           fmt.Sprintf("lot-seed-%03d", i),
			Title:        title,
			Description:  description,
			ImageUrl:     imageUrl,
			CreatorID:    creator,
			OwnerID:      creator,
			StartPrice:   startPrice,
			CurrentPrice: startPrice,
			Status:       "active",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		if err := db.Create(&nft).Error; err != nil {
			fmt.Printf("⚠️ Không thể tạo tác phẩm lot-seed-%03d: %v\n", i, err)
		}
	}
	fmt.Println("✅ Đã tạo thành công 50 tác phẩm nghệ thuật mẫu!")
}

// Hàm trợ giúp đọc biến môi trường kèm giá trị dự phòng (fallback)
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
