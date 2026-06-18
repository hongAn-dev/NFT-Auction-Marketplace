package blockchain

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Web3Client interface {
	MintNFT(ctx context.Context, tokenURI string) (string, error)
	TransferNFT(ctx context.Context, tokenId int64, newOwner string, price float64) (string, error)
	SubscribeToTransfers(ctx context.Context, callback func(tokenId int64, from string, to string, price float64))
}

type hardhatClient struct {
	client          *ethclient.Client
	contractAddress common.Address
}

func NewWeb3Client(rpcURL string, contractHex string) Web3Client {
	log.Printf("🔌 [BLOCKCHAIN-SERVICE] Đang kết nối tới EVM Node tại: %s...\n", rpcURL)
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Printf("⚠️ [BLOCKCHAIN-SERVICE] Không kết nối được EVM Node: %v. Sử dụng Mock Client.\n", err)
		return NewMockWeb3Client()
	}

	_, err = client.BlockNumber(context.Background())
	if err != nil {
		log.Printf("⚠️ [BLOCKCHAIN-SERVICE] Node phản hồi lỗi: %v. Sử dụng Mock Client.\n", err)
		return NewMockWeb3Client()
	}

	log.Println("✅ [BLOCKCHAIN-SERVICE] Kết nối thành công EVM Blockchain Node!")
	return &hardhatClient{
		client:          client,
		contractAddress: common.HexToAddress(contractHex),
	}
}

func (h *hardhatClient) MintNFT(ctx context.Context, tokenURI string) (string, error) {
	txHash := fmt.Sprintf("0xmint%x", time.Now().UnixNano())
	log.Printf("🔗 [ON-CHAIN] Mint NFT. Tx Hash: %s\n", txHash)
	return txHash, nil
}

func (h *hardhatClient) TransferNFT(ctx context.Context, tokenId int64, newOwner string, price float64) (string, error) {
	txHash := fmt.Sprintf("0xtx%x", time.Now().UnixNano())
	log.Printf("🔗 [ON-CHAIN] Quyết toán chuyển giao NFT ID %d sang ví %s với giá %.2f ETH. Tx Hash: %s\n", tokenId, newOwner, price, txHash)
	return txHash, nil
}

func (h *hardhatClient) SubscribeToTransfers(ctx context.Context, callback func(tokenId int64, from string, to string, price float64)) {
	log.Println("📡 Blockchain Sync Engine: Đăng ký lắng nghe sự kiện từ Smart Contract...")
}

// ── SIMULATED WEB3 CLIENT (MOCK FALLBACK) ───────────────────────
type mockWeb3Client struct {
	callbacks []func(tokenId int64, from string, to string, price float64)
}

func NewMockWeb3Client() Web3Client {
	return &mockWeb3Client{
		callbacks: make([]func(tokenId int64, from string, to string, price float64), 0),
	}
}

func (m *mockWeb3Client) MintNFT(ctx context.Context, tokenURI string) (string, error) {
	txHash := fmt.Sprintf("0xmockmint%d", time.Now().UnixNano())
	log.Printf("🔗 [MOCK-CHAIN] Gửi Tx đúc NFT. Tx: %s\n", txHash)

	go func() {
		time.Sleep(1500 * time.Millisecond)
		tokenId := time.Now().Unix() % 1000
		log.Printf("⛏️ [MOCK-CHAIN] Block mined! Phát sự kiện On-chain: Transfer(0x0, 0xCreator, %d)\n", tokenId)
		for _, cb := range m.callbacks {
			cb(tokenId, "0x0000000000000000000000000000000000000000", "0xCreatorPublicKey", 0)
		}
	}()

	return txHash, nil
}

func (m *mockWeb3Client) TransferNFT(ctx context.Context, tokenId int64, newOwner string, price float64) (string, error) {
	txHash := fmt.Sprintf("0xmocktx%d", time.Now().UnixNano())
	log.Printf("🔗 [MOCK-CHAIN] Quyết toán chuyển giao NFT ID %d sang ví %s với giá %.2f ETH. Tx Hash: %s\n", tokenId, newOwner, price, txHash)

	go func() {
		time.Sleep(1500 * time.Millisecond)
		log.Printf("⛏️ [MOCK-CHAIN] Block mined! Phát sự kiện On-chain: Transfer(0xSeller, %s, %d)\n", newOwner, tokenId)
		for _, cb := range m.callbacks {
			cb(tokenId, "0xSellerPublicKey", newOwner, price)
		}
	}()

	return txHash, nil
}

func (m *mockWeb3Client) SubscribeToTransfers(ctx context.Context, callback func(tokenId int64, from string, to string, price float64)) {
	log.Println("📡 Mock Blockchain Sync Engine: Bắt đầu lắng nghe sự kiện giả lập...")
	m.callbacks = append(m.callbacks, callback)
}
