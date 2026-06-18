package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"bidding-service/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
)

// Orchestrator quản lý quy trình phối hợp các giao dịch phân tán và thực hiện Rollback khi lỗi
type Orchestrator struct {
	db         *gorm.DB
	sagaRepo   domain.SagaRepository
	nftRepo    domain.NFTRepository
	web3Client BlockchainClient
	conn       *amqp.Connection
	channel    *amqp.Channel
}

// NewOrchestrator khởi tạo Saga Orchestrator
func NewOrchestrator(
	db *gorm.DB,
	sagaRepo domain.SagaRepository,
	nftRepo domain.NFTRepository,
	web3Client BlockchainClient,
	rabbitURL string,
) (*Orchestrator, error) {
	// Kết nối RabbitMQ
	var conn *amqp.Connection
	var err error
	for i := 1; i <= 5; i++ {
		conn, err = amqp.Dial(rabbitURL)
		if err == nil {
			break
		}
		log.Printf("⚠️ Saga Orchestrator: Chưa kết nối được RabbitMQ (Lần %d/5): %v. Thử lại sau 3s...\n", i, err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("không thể mở channel RabbitMQ: %w", err)
	}

	return &Orchestrator{
		db:         db,
		sagaRepo:   sagaRepo,
		nftRepo:    nftRepo,
		web3Client: web3Client,
		conn:       conn,
		channel:    ch,
	}, nil
}

// Close giải phóng tài nguyên
func (o *Orchestrator) Close() {
	if o.channel != nil {
		o.channel.Close()
	}
	if o.conn != nil {
		o.conn.Close()
	}
}

// Start khởi chạy worker lắng nghe callback kết quả từ các service khác
func (o *Orchestrator) Start(ctx context.Context) error {
	exchangeName := "nft.events"
	queueName := "saga.bidding.callbacks"

	// Khai báo queue nhận callback
	_, err := o.channel.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("lỗi khai báo queue saga callbacks: %w", err)
	}

	// Bind queue với các routing key phản hồi kết quả
	_ = o.channel.QueueBind(queueName, "royalty.processed", exchangeName, false, nil)
	_ = o.channel.QueueBind(queueName, "royalty.failed", exchangeName, false, nil)

	msgs, err := o.channel.Consume(queueName, "saga_orchestrator", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("lỗi consume queue saga callbacks: %w", err)
	}

	log.Println("🔄 Saga Orchestrator đã khởi chạy thành công!")

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("🛑 Saga Orchestrator đang dừng...")
				return
			case d, ok := <-msgs:
				if !ok {
					return
				}
				o.handleCallback(ctx, d)
			}
		}
	}()

	return nil
}

func (o *Orchestrator) handleCallback(ctx context.Context, d amqp.Delivery) {
	var payload struct {
		SagaID string `json:"saga_id"`
		Error  string `json:"error"`
	}

	if err := json.Unmarshal(d.Body, &payload); err != nil {
		log.Printf("⚠️ Saga Orchestrator: Lỗi giải mã callback payload: %v\n", err)
		_ = d.Nack(false, false)
		return
	}

	log.Printf("📥 Saga Orchestrator: Nhận callback [%s] cho Saga ID %s\n", d.RoutingKey, payload.SagaID)

	sagaLog, err := o.sagaRepo.GetByID(ctx, payload.SagaID)
	if err != nil {
		log.Printf("⚠️ Saga Orchestrator: Không tìm thấy Saga log cho ID %s: %v\n", payload.SagaID, err)
		_ = d.Ack(false)
		return
	}

	if d.RoutingKey == "royalty.processed" {
		// Bổ sung bước Quyết toán On-chain bằng Blockchain Client
		log.Printf("🔗 Saga: Đang thực thi On-chain Settlement chuyển giao NFT cho Saga ID %s...\n", payload.SagaID)
		
		// Map Token ID xoay vòng để giả lập (tokenId 1 đến 50) từ NFT ID
		tokenId := int64(1)
		var extractedID int64
		_, errParse := fmt.Sscanf(sagaLog.NFTID, "lot-seed-%d", &extractedID)
		if errParse == nil {
			tokenId = extractedID
		}

		txHash, errBC := o.web3Client.TransferNFT(ctx, tokenId, sagaLog.UserID, sagaLog.Amount)
		if errBC != nil {
			log.Printf("❌ Saga On-chain Settlement Failed: Lỗi blockchain. Bắt đầu bù trừ (Rollback) cho Saga ID: %s. Lỗi: %v\n", payload.SagaID, errBC)
			o.rollbackSaga(ctx, sagaLog, errBC.Error())
		} else {
			// TRANSACTION THÀNH CÔNG HOÀN TOÀN
			sagaLog.State = domain.SagaCompleted
			sagaLog.ErrorMessage = fmt.Sprintf("Blockchain Tx: %s", txHash)
			sagaLog.UpdatedAt = time.Now()
			_ = o.sagaRepo.Update(ctx, sagaLog)
			log.Printf("✅ Saga Completed: Đấu giá thành công tuyệt đối và chuyển giao On-chain thành công! Tx Hash: %s\n", txHash)
		}
	} else if d.RoutingKey == "royalty.failed" {
		// GIAO DỊCH THẤT BẠI -> THỰC HIỆN ROLLBACK (COMPENSATING TRANSACTION)
		log.Printf("❌ Saga Failed: Bắt đầu giao dịch bù trừ (Rollback) cho Saga ID: %s. Lý do: %s\n", payload.SagaID, payload.Error)
		o.rollbackSaga(ctx, sagaLog, payload.Error)
	}

	_ = d.Ack(false)
}

func (o *Orchestrator) rollbackSaga(ctx context.Context, sagaLog *domain.SagaLog, errorReason string) {
	errRollback := o.db.Transaction(func(tx *gorm.DB) error {
		// Lấy NFT ra để cập nhật lại thông tin cũ
		var nft domain.NFT
		if err := tx.First(&nft, "id = ?", sagaLog.NFTID).Error; err != nil {
			return err
		}

		// Khôi phục giá cũ và chủ sở hữu danh nghĩa cũ
		nft.CurrentPrice = sagaLog.PrevPrice
		nft.OwnerID = sagaLog.PrevOwnerID
		nft.UpdatedAt = time.Now()

		if err := tx.Save(&nft).Error; err != nil {
			return err
		}

		// Cập nhật log trạng thái Saga thành COMPENSATED
		sagaLog.State = domain.SagaCompensated
		sagaLog.ErrorMessage = errorReason
		sagaLog.UpdatedAt = time.Now()
		
		if err := tx.Save(sagaLog).Error; err != nil {
			return err
		}

		return nil
	})

	if errRollback != nil {
		log.Printf("❌ Saga Orchestrator: Lỗi thực hiện compensating transaction cho Saga ID %s: %v\n", sagaLog.ID, errRollback)
	} else {
		log.Printf("🔄 Saga Compensated: Đã hoàn trả trạng thái NFT %s về giá %.2f và chủ sở hữu %s thành công!\n", 
			sagaLog.NFTID, sagaLog.PrevPrice, sagaLog.PrevOwnerID)
	}
}
