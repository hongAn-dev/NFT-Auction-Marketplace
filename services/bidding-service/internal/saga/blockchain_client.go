package saga

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// BlockchainClient định nghĩa giao diện gọi API sang blockchain-service
type BlockchainClient interface {
	TransferNFT(ctx context.Context, tokenId int64, newOwner string, price float64) (string, error)
}

type httpBlockchainClient struct {
	serviceURL string
}

// NewBlockchainClient tạo đối tượng HTTP Client kết nối tới blockchain-service
func NewBlockchainClient(serviceURL string) BlockchainClient {
	return &httpBlockchainClient{
		serviceURL: serviceURL,
	}
}

func (c *httpBlockchainClient) TransferNFT(ctx context.Context, tokenId int64, newOwner string, price float64) (string, error) {
	url := fmt.Sprintf("%s/api/v1/blockchain/transfer", c.serviceURL)
	payload := map[string]interface{}{
		"token_id":  tokenId,
		"new_owner": newOwner,
		"price":     price,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("lỗi serialize payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("lỗi khởi tạo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("lỗi kết nối tới blockchain-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("blockchain-service trả về status code: %d", resp.StatusCode)
	}

	var result struct {
		Success bool   `json:"success"`
		TxHash  string `json:"tx_hash"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("lỗi decode response: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("blockchain transaction thất bại: %s", result.Message)
	}

	return result.TxHash, nil
}
