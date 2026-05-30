package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQRepository quản lý kết nối và việc phát thông điệp lên RabbitMQ Broker
type RabbitMQRepository struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

// InitRabbitMQ kết nối tới RabbitMQ và khai báo Exchange mặc định của hệ thống
func InitRabbitMQ() (*RabbitMQRepository, error) {
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	fmt.Printf("🔍 Đang kết nối tới RabbitMQ tại: %s...\n", rabbitmqURL)

	var conn *amqp.Connection
	var err error

	// Thử kết nối lại tối đa 5 lần (Retry Mechanism) đề phòng RabbitMQ khởi động chậm hơn Go Service
	for i := 1; i <= 5; i++ {
		conn, err = amqp.Dial(rabbitmqURL)
		if err == nil {
			break
		}
		log.Printf("⚠️ Không thể kết nối tới RabbitMQ (Lần thử %d/5): %v. Thử lại sau 3 giây...\n", i, err)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("lỗi kết nối tới RabbitMQ sau 5 lần thử: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("không thể mở channel trên RabbitMQ: %w", err)
	}

	// ── KHAI BÁO EXCHANGE CHÍNH CỦA HỆ THỐNG ─────────────────────
	// nft.events: Exchange loại direct (định tuyến chính xác theo routing key)
	exchangeName := "nft.events"
	fmt.Printf("🔧 Đang khai báo Exchange '%s' (direct)...\n", exchangeName)
	err = channel.ExchangeDeclare(
		exchangeName, // Tên Exchange
		"direct",     // Loại Exchange
		true,         // Bền vững (Durable) - Thông điệp được lưu xuống đĩa, không mất khi RabbitMQ restart
		false,        // Tự động xóa (Auto-deleted)
		false,        // Nội bộ (Internal)
		false,        // Không chờ phản hồi (No-wait)
		nil,          // Các đối số cấu hình bổ sung
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("lỗi khai báo Exchange '%s': %w", err)
	}

	fmt.Println("✅ Kết nối RabbitMQ thành công và khai báo Exchange 'nft.events' hoàn tất!")

	return &RabbitMQRepository{
		conn:    conn,
		channel: channel,
	}, nil
}

// PublishEvent phát đi một sự kiện bất đồng bộ lên RabbitMQ Exchange
func (r *RabbitMQRepository) PublishEvent(ctx context.Context, routingKey string, eventData interface{}) error {
	// Tuần tự hóa (Serialize) dữ liệu sự kiện thành JSON bytes
	body, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("lỗi marshal dữ liệu sự kiện: %w", err)
	}

	exchangeName := "nft.events"
	fmt.Printf("📤 RabbitMQ: Phát sự kiện lên Exchange '%s' với routing key '%s'\n", exchangeName, routingKey)

	// Thực hiện đẩy thông điệp lên exchange
	err = r.channel.PublishWithContext(
		ctx,
		exchangeName, // Exchange nhận thông điệp
		routingKey,   // Routing Key định tuyến
		false,        // Bắt buộc (Mandatory)
		false,        // Ngay lập tức (Immediate)
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // Cực kỳ quan trọng: Thông điệp bền vững (ghi đĩa), không bị mất nếu broker restart
			Timestamp:    time.Now(),
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("lỗi publish sự kiện lên RabbitMQ: %w", err)
	}

	return nil
}

// Close ngắt kết nối an toàn với RabbitMQ Broker
func (r *RabbitMQRepository) Close() {
	if r.channel != nil {
		r.channel.Close()
	}
	if r.conn != nil {
		r.conn.Close()
	}
}
