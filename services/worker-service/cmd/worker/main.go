package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Royalty đại diện cho thực thể phí tác quyền nghệ sĩ, được lưu vào PostgreSQL.
type Royalty struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	NFTID      string    `gorm:"type:varchar(100);index" json:"nft_id"`
	BidID      string    `gorm:"type:varchar(100)" json:"bid_id"`
	UserID     string    `gorm:"type:varchar(100)" json:"user_id"`
	BidAmount  float64   `gorm:"type:numeric" json:"bid_amount"`
	RoyaltyFee float64   `gorm:"type:numeric" json:"royalty_fee"` // 10% của số tiền thầu
	CreatedAt  time.Time `json:"created_at"`
}

func main() {
	fmt.Println("🚀 Đang khởi chạy dịch vụ Go Worker Service...")

	// ── 1. KẾT NỐI POSTGRESQL (Để Royalty Worker ghi DB) ───────────
	db, err := initDB()
	if err != nil {
		log.Fatalf("❌ Worker: Lỗi kết nối PostgreSQL: %v", err)
	}

	// ── 2. KẾT NỐI RABBITMQ ───────────────────────────────────────
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	var conn *amqp.Connection
	for i := 1; i <= 5; i++ {
		conn, err = amqp.Dial(rabbitmqURL)
		if err == nil {
			break
		}
		log.Printf("⚠️ Worker: Chưa thể kết nối RabbitMQ (Lần %d/5): %v. Thử lại sau 3s...\n", i, err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("❌ Worker: Lỗi kết nối tới RabbitMQ: %v", err)
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("❌ Worker: Không thể mở channel trên RabbitMQ: %v", err)
	}
	defer channel.Close()

	// ── 3. CẤU HÌNH ĐƯỜNG ỐNG SỰ KIỆN & DEAD-LETTER QUEUE (DLQ) ───
	
	// A. Khai báo DLX (Dead Letter Exchange)
	dlxName := "nft.dlx"
	err = channel.ExchangeDeclare(dlxName, "fanout", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("❌ Worker: Lỗi khai báo DLX: %v", err)
	}

	// B. Khai báo DLQ (Dead Letter Queue)
	dlqName := "nft.dlq"
	_, err = channel.QueueDeclare(dlqName, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("❌ Worker: Lỗi khai báo DLQ: %v", err)
	}

	// C. Bind DLQ với DLX
	err = channel.QueueBind(dlqName, "", dlxName, false, nil)
	if err != nil {
		log.Fatalf("❌ Worker: Lỗi bind DLQ vào DLX: %v", err)
	}

	// D. Cấu hình Queue Arguments để liên kết với DLX
	// Nếu bất kỳ thông điệp nào ở hàng đợi chính bị NACK (requeue=false), 
	// nó sẽ tự động được gửi tới nft.dlx để lưu vào nft.dlq.
	queueArgs := amqp.Table{
		"x-dead-letter-exchange": dlxName,
	}

	// E. Khai báo Exchange chính
	exchangeName := "nft.events"
	err = channel.ExchangeDeclare(exchangeName, "direct", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("❌ Worker: Lỗi khai báo Exchange chính: %v", err)
	}

	// F. Cấu hình Queue Email Notifications
	emailQueue := "email.notifications"
	_, err = channel.QueueDeclare(emailQueue, true, false, false, false, queueArgs)
	if err != nil {
		log.Fatalf("❌ Lỗi khai báo queue '%s': %v", emailQueue, err)
	}
	// Bind cho sự kiện đặt thầu (bid.placed) và giao dịch thành công (nft.purchased)
	_ = channel.QueueBind(emailQueue, "bid.placed", exchangeName, false, nil)
	_ = channel.QueueBind(emailQueue, "nft.purchased", exchangeName, false, nil)

	// G. Cấu hình Queue Royalty Processing
	royaltyQueue := "royalty.processing"
	_, err = channel.QueueDeclare(royaltyQueue, true, false, false, false, queueArgs)
	if err != nil {
		log.Fatalf("❌ Lỗi khai báo queue '%s': %v", royaltyQueue, err)
	}
	// Chỉ bind cho sự kiện giao dịch thành công (nft.purchased)
	_ = channel.QueueBind(royaltyQueue, "nft.purchased", exchangeName, false, nil)

	fmt.Println("✅ Worker: Cấu hình Exchange, Queue và DLQ liên kết hoàn tất!")

	// ── 4. CHẠY SONG SONG CÁC WORKERS CONSUMERS (Goroutines) ──────
	
	// A. EMAIL WORKER CONSUMER
	go runEmailWorker(channel, emailQueue)

	// B. ROYALTY FEE WORKER CONSUMER
	go runRoyaltyWorker(channel, royaltyQueue, db)

	// Giữ cho luồng chính luôn chạy ngầm
	select {}
}

// 📧 EMAIL WORKER: In log giả lập gửi email thông báo
func runEmailWorker(channel *amqp.Channel, queueName string) {
	msgs, err := channel.Consume(queueName, "email_worker", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("❌ Email Worker: Lỗi bắt đầu consume: %v", err)
	}

	fmt.Println("📧 Email Worker đã sẵn sàng lắng nghe sự kiện...")
	for d := range msgs {
		var event map[string]interface{}
		err := json.Unmarshal(d.Body, &event)
		if err != nil {
			log.Printf("⚠️ Email Worker: Lỗi unmarshal thông điệp. Đang đẩy vào DLQ! Lỗi: %v\n", err)
			_ = d.Nack(false, false) // NACK và requeue=false -> tự động nhảy vào DLQ!
			continue
		}

		// Giả lập gửi email thành công
		fmt.Printf("📧 [EMAIL WORKER] Đang gửi email cho sự kiện '%s'...\n", d.RoutingKey)
		fmt.Printf("📧 ---> [Email Sent] Chúc mừng User %v đã thực hiện thành công giao dịch cho NFT %v với số tiền %.2f!\n",
			event["user_id"], event["nft_id"], event["amount"])
		
		_ = d.Ack(false) // Xác nhận đã xử lý xong và xóa khỏi queue chính
	}
}

// 💰 ROYALTY WORKER: Tính phí tác quyền 10% và ghi vào PostgreSQL thực tế!
func runRoyaltyWorker(channel *amqp.Channel, queueName string, db *gorm.DB) {
	msgs, err := channel.Consume(queueName, "royalty_worker", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("❌ Royalty Worker: Lỗi bắt đầu consume: %v", err)
	}

	publishOutcome := func(routingKey string, payload interface{}) {
		body, _ := json.Marshal(payload)
		err := channel.Publish(
			"nft.events",
			routingKey,
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        body,
			},
		)
		if err != nil {
			log.Printf("⚠️ Royalty Worker: Lỗi publish callback event %s: %v\n", routingKey, err)
		} else {
			log.Printf("📢 Royalty Worker: Đã publish callback event %s thành công.\n", routingKey)
		}
	}

	fmt.Println("💰 Royalty Worker đã sẵn sàng lắng nghe sự kiện...")
	for d := range msgs {
		var event struct {
			BidID     string  `json:"bid_id"`
			NFTID     string  `json:"nft_id"`
			UserID    string  `json:"user_id"`
			Amount    float64 `json:"amount"`
			CreatedAt int64   `json:"created_at"`
		}

		err := json.Unmarshal(d.Body, &event)
		if err != nil {
			log.Printf("⚠️ Royalty Worker: Lỗi unmarshal. Đẩy vào DLQ! Lỗi: %v\n", err)
			_ = d.Nack(false, false)
			// Trả về callback thất bại của Saga
			publishOutcome("royalty.failed", map[string]interface{}{
				"saga_id": event.BidID,
				"bid_id":  event.BidID,
				"nft_id":  event.NFTID,
				"error":   "Unmarshal payload error",
			})
			continue
		}

		// Validate: Nếu số tiền thầu nhỏ hơn hoặc bằng 0 -> Phế phẩm, đẩy thẳng vào DLQ!
		if event.Amount <= 0 {
			log.Printf("⚠️ Royalty Worker: Giá trị giao dịch %.2f không hợp lệ! Đẩy vào DLQ.\n", event.Amount)
			_ = d.Nack(false, false)
			publishOutcome("royalty.failed", map[string]interface{}{
				"saga_id": event.BidID,
				"bid_id":  event.BidID,
				"nft_id":  event.NFTID,
				"error":   fmt.Sprintf("Invalid amount: %.2f", event.Amount),
			})
			continue
		}

		// Tính toán phí tác quyền 10% theo quy định
		royaltyFee := event.Amount * 0.10

		royaltyRecord := &Royalty{
			NFTID:      event.NFTID,
			BidID:      event.BidID,
			UserID:     event.UserID,
			BidAmount:  event.Amount,
			RoyaltyFee: royaltyFee,
			CreatedAt:  time.Now(),
		}

		// Ghi thực tế bản ghi phí tác quyền vào PostgreSQL
		result := db.Create(royaltyRecord)
		if result.Error != nil {
			log.Printf("❌ Royalty Worker: Lỗi ghi DB Postgres. Bỏ qua và NACK để xử lý lại: %v\n", result.Error)
			_ = d.Nack(false, true) // NACK và requeue=true để thử lại sau khi DB ổn định
			continue
		}

		fmt.Printf("💰 [ROYALTY WORKER] Tính phí tác quyền thành công cho NFT '%s'!\n", event.NFTID)
		fmt.Printf("💰 ---> [Phí tác quyền 10%%]: %.2f từ số tiền giao dịch %.2f của User %s. Đã ghi nhận PostgreSQL thành công!\n",
			royaltyFee, event.Amount, event.UserID)

		_ = d.Ack(false)

		// Trả về callback thành công của Saga
		publishOutcome("royalty.processed", map[string]interface{}{
			"saga_id": event.BidID,
			"bid_id":  event.BidID,
			"nft_id":  event.NFTID,
		})
	}
}

// Hàm khởi tạo PostgreSQL kết nối an toàn cho Worker
func initDB() (*gorm.DB, error) {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "nft_user"
	}
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		password = "secret_password"
	}
	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "nft_auction"
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Ho_Chi_Minh", host, user, password, dbname, port)

	var db *gorm.DB
	var err error

	// Thử kết nối lại đề phòng Postgres DB khởi động chậm
	for i := 1; i <= 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("⚠️ Worker DB: Chưa thể kết nối PostgreSQL (Lần %d/5): %v. Thử lại sau 3s...\n", i, err)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("không thể kết nối PostgreSQL sau 5 lần thử: %w", err)
	}

	// Tự động Migrate cấu trúc bảng royalties
	fmt.Println("🔧 Worker DB: Đang đồng bộ hóa cấu trúc bảng royalties (AutoMigrate)...")
	err = db.AutoMigrate(&Royalty{})
	if err != nil {
		return nil, fmt.Errorf("lỗi AutoMigrate royalties: %w", err)
	}
	fmt.Println("✅ Worker DB: Đồng bộ bảng royalties thành công!")

	return db, nil
}
