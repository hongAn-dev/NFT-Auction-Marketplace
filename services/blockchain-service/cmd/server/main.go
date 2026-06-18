package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"blockchain-service/internal/blockchain"
	"blockchain-service/internal/domain"
	"blockchain-service/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	log.Println("🚀 Đang khởi động Blockchain Service...")

	// ── 1. KẾT NỐI POSTGRESQL (Để Sync Worker đồng bộ DB) ──────────
	db, err := initDB()
	if err != nil {
		log.Fatalf("❌ Blockchain Service: Lỗi kết nối PostgreSQL: %v", err)
	}

	// ── 2. KHỞI TẠO BLOCKCHAIN CLIENT ─────────────────────────────
	blockchainURL := os.Getenv("BLOCKCHAIN_URL")
	if blockchainURL == "" {
		blockchainURL = "http://localhost:8545"
	}
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
	if contractAddress == "" {
		contractAddress = "0x5FbDB2315678afecb367f032d93F642f64180aa3"
	}
	web3Client := blockchain.NewWeb3Client(blockchainURL, contractAddress)

	// ── 3. KHỞI CHẠY BLOCKCHAIN SYNC WORKER ─────────────────────────
	nftRepo := repository.NewPostgresNFTRepository(db)
	syncWorker := blockchain.NewSyncWorker(web3Client, nftRepo)
	go syncWorker.Start(context.Background())

	// ── 4. CẤU HÌNH GIN WEB SERVER ─────────────────────────────────
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// CORS Middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	api := r.Group("/api/v1/blockchain")
	{
		api.POST("/mint", func(c *gin.Context) {
			var req struct {
				TokenURI string `json:"token_uri" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Payload không hợp lệ"})
				return
			}

			txHash, err := web3Client.MintNFT(c.Request.Context(), req.TokenURI)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"tx_hash": txHash,
				"message": "Transaction đúc NFT đã được gửi lên Blockchain!",
			})
		})

		api.POST("/transfer", func(c *gin.Context) {
			var req struct {
				TokenID  int64   `json:"token_id" binding:"required"`
				NewOwner string  `json:"new_owner" binding:"required"`
				Price    float64 `json:"price" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Payload không hợp lệ"})
				return
			}

			txHash, err := web3Client.TransferNFT(c.Request.Context(), req.TokenID, req.NewOwner, req.Price)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"tx_hash": txHash,
				"message": "Transaction chuyển giao NFT đã được gửi quyết toán on-chain!",
			})
		})
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "blockchain-service"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "50052"
	}

	log.Printf("🔥 Blockchain Service HTTP Server đang chạy tại cổng: %s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("❌ Lỗi khởi chạy server: %v", err)
	}
}

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

	for i := 1; i <= 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("⚠️ Blockchain DB: Chưa kết nối được PostgreSQL (Lần %d/5): %v. Thử lại sau 3s...\n", i, err)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("không kết nối được PostgreSQL sau 5 lần: %w", err)
	}

	// Đảm bảo struct NFT map vào bảng "nfts" (GORM mặc định pluralize là "nfts")
	db.AutoMigrate(&domain.NFT{})
	return db, nil
}
