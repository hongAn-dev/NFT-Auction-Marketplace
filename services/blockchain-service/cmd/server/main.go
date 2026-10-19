// blockchain-service: chỗ duy nhất của sàn mô phỏng chạm chuỗi (docs/23 §3). Gọi
// passport-service vai platform qua gRPC; không giữ khoá chuỗi nào ngoài khoá bí danh.
package main

import (
	"log"
	"os"

	"blockchain-service/internal/api"
	"blockchain-service/internal/owner"
	pb "blockchain-service/internal/passportv1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	keyFile := os.Getenv("OWNER_ALIAS_KEY_FILE")
	if keyFile == "" {
		log.Fatal("thiếu OWNER_ALIAS_KEY_FILE (tệp 32 byte hex, quyền 600, ngoài repo)")
	}
	alias, err := owner.Load(keyFile)
	if err != nil {
		log.Fatal(err)
	}
	addr := env("PASSPORT_SERVICE_ADDR", "172.17.0.1:7601")
	// ponytail: gRPC tới passport-service chưa có TLS — chỉ đi trên cầu docker của M-UBU.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("kết nối passport-service %s: %v", addr, err)
	}
	defer conn.Close()

	port := env("PORT", "50052")
	log.Printf("blockchain-service nghe :%s, passport-service %s", port, addr)
	if err := api.NewRouter(pb.NewPassportServiceClient(conn), alias).Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
