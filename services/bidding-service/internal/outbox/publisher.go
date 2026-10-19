package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"bidding-service/internal/auction"
	"bidding-service/internal/clients"
	"bidding-service/internal/domain"
	"bidding-service/internal/repository"
)

// Releaser: nhả khoản ký quỹ ở auth-service (docs/23 §5.2).
type Releaser interface {
	Release(ctx context.Context, holdID string) error
}

// Publisher quản lý tiến trình gửi ngầm các event từ bảng Outbox sang RabbitMQ/Redis
type Publisher struct {
	outboxRepo domain.OutboxRepository
	rabbitRepo *repository.RabbitMQRepository
	redisRepo  *repository.RedisRepository
	escrow     Releaser
}

// NewPublisher khởi tạo đối tượng Publisher
func NewPublisher(
	outboxRepo domain.OutboxRepository,
	rabbitRepo *repository.RabbitMQRepository,
	redisRepo *repository.RedisRepository,
	escrow Releaser,
) *Publisher {
	return &Publisher{
		outboxRepo: outboxRepo,
		rabbitRepo: rabbitRepo,
		redisRepo:  redisRepo,
		escrow:     escrow,
	}
}

// Start khởi chạy vòng lặp polling sự kiện của Outbox Publisher
func (p *Publisher) Start(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	log.Println("🔄 Outbox Publisher background worker đã khởi động!")

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Outbox Publisher background worker đang dừng...")
			return
		case <-ticker.C:
			p.processEvents(ctx)
		}
	}
}

func (p *Publisher) processEvents(ctx context.Context) {
	// Lấy tối đa 10 events chưa được xử lý
	events, err := p.outboxRepo.GetUnprocessed(ctx, 10)
	if err != nil {
		log.Printf("⚠️ Outbox Publisher: Lỗi truy vấn bảng outbox: %v\n", err)
		return
	}

	if len(events) == 0 {
		return
	}

	var processedIDs []uint
	for _, event := range events {
		var payload map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			log.Printf("⚠️ Outbox Publisher: Lỗi giải mã payload event ID %d: %v\n", event.ID, err)
			continue
		}

		success := true
		// Định tuyến sự kiện theo Topic
		if event.Topic == auction.TopicEscrowRelease {
			// Nhả tiền người bị vượt giá. 409 = khoản đã chốt từ trước → coi như xong.
			holdID, _ := payload["hold_id"].(string)
			if err := p.escrow.Release(ctx, holdID); err != nil && !errors.Is(err, clients.ErrAlreadySettled) {
				log.Printf("⚠️ Outbox Publisher: nhả khoản giữ %s lỗi, thử lại sau: %v\n", holdID, err)
				success = false
			}
		} else if event.Topic == "redis:bid:updated" {
			if p.redisRepo != nil {
				nftID, _ := payload["nft_id"].(string)
				errPub := p.redisRepo.PublishBidEvent(ctx, nftID, payload)
				if errPub != nil {
					log.Printf("⚠️ Outbox Publisher: Lỗi phát sự kiện lên Redis Pub/Sub cho Event ID %d: %v\n", event.ID, errPub)
					success = false
				}
			}
		} else {
			// Phát sự kiện lên RabbitMQ
			if p.rabbitRepo != nil {
				errPub := p.rabbitRepo.PublishEvent(ctx, event.Topic, payload)
				if errPub != nil {
					log.Printf("⚠️ Outbox Publisher: Lỗi phát sự kiện lên RabbitMQ (Topic: %s) cho Event ID %d: %v\n", event.Topic, event.ID, errPub)
					success = false
				}
			}
		}

		if success {
			processedIDs = append(processedIDs, event.ID)
		}
	}

	if len(processedIDs) > 0 {
		if err := p.outboxRepo.MarkProcessed(ctx, processedIDs); err != nil {
			log.Printf("⚠️ Outbox Publisher: Lỗi cập nhật trạng thái đã xử lý trong DB: %v\n", err)
		} else {
			log.Printf("✅ Outbox Publisher: Đã publish thành công %d sự kiện từ Outbox.\n", len(processedIDs))
		}
	}
}
