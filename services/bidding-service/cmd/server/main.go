package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	_ "bidding-service/docs"
	"bidding-service/internal/handler"
	"bidding-service/internal/logger"
	"bidding-service/internal/outbox"
	"bidding-service/internal/repository"
	"bidding-service/internal/saga"
	"bidding-service/internal/usecase"
	pb "bidding-service/proto"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
)

// @title CURATORIAL NFT Bidding & Auction API
// @version 1.0
// @description High-performance distributed backend service for real-time NFT auctions.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:8080
// @BasePath /
func main() {
	// Khởi tạo Structured Logging (slog) chuẩn doanh nghiệp
	logger.InitLogger()

	slog.Info("🚀 Đang khởi động hệ thống Bidding Service...")

	// Tự động tải biến môi trường từ tệp .env cục bộ nếu tồn tại
	loadEnv()

	// ── KHỞI TẠO CÁC CƠ SỞ DỮ LIỆU & BROKER (SQL, REDIS, NOSQL, MQ) ──

	// 1. Khởi tạo kết nối PostgreSQL thông qua GORM
	db, err := repository.InitDB()
	if err != nil {
		slog.Error("❌ Lỗi nghiêm trọng khi khởi tạo Cơ sở dữ liệu Postgres", "error", err)
		os.Exit(1)
	}

	// 2. Khởi tạo kết nối Redis Client
	redisRepo, err := repository.InitRedis()
	if err != nil {
		slog.Error("❌ Lỗi nghiêm trọng khi khởi tạo Cơ sở dữ liệu Redis", "error", err)
		os.Exit(1)
	}

	// 3. Khởi tạo kết nối MongoDB Client
	ctxMongo, cancelMongo := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelMongo()

	mongoClient, mongoCollection, err := repository.InitMongoDB(ctxMongo)
	if err != nil {
		slog.Error("❌ Lỗi nghiêm trọng khi khởi tạo Cơ sở dữ liệu MongoDB", "error", err)
		os.Exit(1)
	}
	defer func() {
		ctxDisconnect, cancelDisconnect := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDisconnect()
		if err := mongoClient.Disconnect(ctxDisconnect); err != nil {
			slog.Warn("⚠️ Lỗi ngắt kết nối MongoDB", "error", err)
		}
	}()

	// 4. Khởi tạo kết nối RabbitMQ Broker
	rabbitRepo, err := repository.InitRabbitMQ()
	if err != nil {
		slog.Error("❌ Lỗi nghiêm trọng khi khởi tạo RabbitMQ Broker", "error", err)
		os.Exit(1)
	}
	defer rabbitRepo.Close()

	// ── KHỞI TẠO CÁC TẦNG REPOSITORY & USECASE ──────────────────────

	// Khởi tạo Postgres Repository thực tế
	nftRepo := repository.NewPostgresNFTRepository(db)

	// Khởi tạo MongoDB Metadata Repository thực tế
	mongoMetadataRepo := repository.NewMongoMetadataRepository(mongoCollection)

	// Khởi tạo Usecase nhận cả Postgres Repo và Redis Repo (Dependency Injection)
	nftUsecase := usecase.NewNFTUsecase(nftRepo, redisRepo)

	// Khởi tạo tầng REST API Handler nhận Usecase (SQL/Redis) và MetadataRepo (MongoDB)
	nftRESTHandler := handler.NewNFTHTTPHandler(nftUsecase, mongoMetadataRepo)

	// ── CẤU HÌNH WEB ROUTER (GIN) ──────────────────────────────────
	router := gin.New()
	
	// Sử dụng Structured Logging Middleware thay thế cho gin.Logger mặc định!
	router.Use(logger.GinLoggerMiddleware())
	router.Use(gin.Recovery())

	// CORS Middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Đăng ký REST API Routes
	nftRESTHandler.RegisterRoutes(router)

	// ── ĐĂNG KÝ S3 / CLOUDFLARE R2 MOCK ROUTES ────────────────────
	s3Handler := handler.NewS3MockHandler()
	router.Static("/uploads", "./uploads")
	s3Group := router.Group("/api/v1")
	{
		s3Group.GET("/s3/presign", s3Handler.PresignURL)
		s3Group.PUT("/s3/upload", s3Handler.Upload)
	}

	// ── PHƠI ENDPOINT SWAGGER DOCS TRỰC QUAN ─────────────────────
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Health Check REST API
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":      "healthy",
			"service":     "bidding-service",
			"environment": os.Getenv("APP_ENV"),
		})
	})

	// ── CẤU HÌNH CỔNG CHẠY HỆ THỐNG (MULTIPLEXED SINGLE PORT) ─────────
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("APP_PORT")
		if port == "" {
			port = "8080"
		}
	}

	outboxRepo := repository.NewPostgresOutboxRepository(db)
	sagaRepo := repository.NewPostgresSagaRepository(db)

	// Khởi chạy Outbox Publisher Worker chạy ngầm
	outboxPub := outbox.NewPublisher(outboxRepo, rabbitRepo, redisRepo)
	go outboxPub.Start(context.Background())

	// Khởi tạo HTTP Client kết nối tới blockchain-service độc lập
	blockchainServiceURL := os.Getenv("BLOCKCHAIN_SERVICE_URL")
	if blockchainServiceURL == "" {
		blockchainServiceURL = "http://blockchain-service:50052"
	}
	blockchainClient := saga.NewBlockchainClient(blockchainServiceURL)

	// Khởi chạy Saga Orchestrator Worker chạy ngầm
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@rabbitmq:5672/"
	}
	sagaOrch, err := saga.NewOrchestrator(db, sagaRepo, nftRepo, blockchainClient, rabbitURL)
	if err != nil {
		slog.Error("❌ Saga: Lỗi khởi chạy Saga Orchestrator", "error", err)
		os.Exit(1)
	}
	defer sagaOrch.Close()
	if err := sagaOrch.Start(context.Background()); err != nil {
		slog.Error("❌ Saga: Lỗi bắt đầu Saga Orchestrator", "error", err)
		os.Exit(1)
	}

	// Khởi tạo gRPC Handler nhận Postgres, Redis, RabbitMQ, Outbox & Saga để thực hiện Transaction
	nftGRPCHandler := handler.NewBiddingGRPCHandler(nftUsecase, redisRepo, rabbitRepo, db, outboxRepo, sagaRepo)
	grpcServer := grpc.NewServer()
	pb.RegisterBiddingServiceServer(grpcServer, nftGRPCHandler)

	// Multiplexing Handler: Tự động phân chia luồng gRPC/HTTP2 và HTTP/1.1 REST
	mixedHandler := h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			router.ServeHTTP(w, r)
		}
	}), &http2.Server{})

	slog.Info("🔥 Multiplexed Server (Gin REST + gRPC) đang lắng nghe kết nối", "port", port)
	slog.Info("👉 Xem tài liệu API Swagger tại", "url", "http://localhost:"+port+"/swagger/index.html")

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mixedHandler,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("❌ Server: Lỗi không thể phục vụ dịch vụ", "error", err)
		os.Exit(1)
	}
}

// loadEnv đọc tệp .env cục bộ nạp vào os.Setenv
func loadEnv() {
	file, err := os.Open(".env")
	if err != nil {
		fmt.Println("ℹ️ Không tìm thấy file .env, sử dụng biến môi trường hệ thống.")
		return
	}
	defer file.Close()

	fmt.Println("📂 Đang nạp biến cấu hình từ tệp .env...")
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			os.Setenv(key, val)
		}
	}
}
