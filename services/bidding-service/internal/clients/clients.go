// Package clients: HTTP client tới auth-service (ký quỹ) và blockchain-service
// (chuỗi). Lỗi mạng và 5xx là "tạm thời" (Transient) — saga thử lại; 4xx là lỗi
// vĩnh viễn (docs/23 §6.3).
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"bidding-service/internal/auction"
	"bidding-service/internal/settlement"
)

// APIError: phản hồi lỗi của dịch vụ bên kia. Status 0 = không tới được.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return "không kết nối được: " + e.Message
	}
	return fmt.Sprintf("HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

func (e *APIError) Transient() bool { return e.Status == 0 || e.Status >= 500 }

// RejectionCode: 400/409 là từ chối nghiệp vụ (ví dụ E9 SERIAL_OWNED_BY_OTHER) —
// phía đăng bán báo lại cho người dùng thay vì coi là lỗi hệ thống.
func (e *APIError) RejectionCode() string {
	if e.Status == 400 || e.Status == 409 {
		return e.Code
	}
	return ""
}

// ErrAlreadySettled: release một khoản đã chốt (409) — người bị vượt giá không còn
// gì để nhả; outbox coi như xong.
var ErrAlreadySettled = errors.New("khoản giữ đã chốt")

func defaultHTTP(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 45 * time.Second} // một giao dịch Fabric mất vài giây
}

// do gửi JSON, đọc JSON vào out (nil = bỏ qua body).
func do(ctx context.Context, hc *http.Client, method, u, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("x-service-token", token)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return &APIError{Message: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		var e struct{ Code, Message string }
		_ = json.Unmarshal(raw, &e)
		return &APIError{Status: resp.StatusCode, Code: e.Code, Message: e.Message}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("giải mã phản hồi %s: %w", u, err)
	}
	return nil
}

// Escrow: auth-service /internal/escrow (docs/23 §4).
type Escrow struct {
	Base, Token string
	HTTP        *http.Client
}

func (e *Escrow) post(ctx context.Context, path string, in any) error {
	return do(ctx, defaultHTTP(e.HTTP), http.MethodPost, e.Base+"/internal/escrow"+path, e.Token, in, nil)
}

func (e *Escrow) Hold(ctx context.Context, holdID, userID string, amount int64) error {
	err := e.post(ctx, "/holds", map[string]any{"id": holdID, "userId": userID, "amount": amount})
	var api *APIError
	if errors.As(err, &api) && api.Status == http.StatusPaymentRequired {
		return auction.ErrInsufficientFunds
	}
	return err
}

func (e *Escrow) Release(ctx context.Context, holdID string) error {
	err := e.post(ctx, "/holds/"+url.PathEscape(holdID)+"/release", map[string]any{})
	var api *APIError
	if errors.As(err, &api) && api.Status == http.StatusConflict {
		return ErrAlreadySettled
	}
	return err
}

func (e *Escrow) Refund(ctx context.Context, holdID string) error {
	return e.post(ctx, "/holds/"+url.PathEscape(holdID)+"/refund", map[string]any{})
}

func (e *Escrow) Capture(ctx context.Context, holdID, sellerID string) error {
	return e.post(ctx, "/holds/"+url.PathEscape(holdID)+"/capture", map[string]any{"sellerId": sellerID})
}

func (e *Escrow) Stale(ctx context.Context, minAge time.Duration) ([]string, error) {
	var out struct {
		Data []struct{ ID string } `json:"data"`
	}
	u := fmt.Sprintf("%s/internal/escrow/holds/stale?olderThanSec=%d", e.Base, int(minAge.Seconds()))
	if err := do(ctx, defaultHTTP(e.HTTP), http.MethodGet, u, e.Token, nil, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, h := range out.Data {
		ids = append(ids, h.ID)
	}
	return ids, nil
}

// Chain: blockchain-service /api/v1/passport (docs/23 §3).
type Chain struct {
	Base string
	HTTP *http.Client
}

func (c *Chain) url(path string) string { return c.Base + "/api/v1/passport" + path }

// ListingCheck: docs/23 §7.1. 409 SERIAL_OWNED_BY_OTHER / PASSPORT_LOCKED trả về
// dạng *APIError để phía đăng bán báo lại cho người dùng.
func (c *Chain) ListingCheck(ctx context.Context, brand, serial, sellerID string) (string, bool, error) {
	var out struct {
		PassportID      string `json:"passport_id"`
		PassportExisted bool   `json:"passport_existed"`
	}
	err := do(ctx, defaultHTTP(c.HTTP), http.MethodPost, c.url("/listing-check"), "",
		map[string]string{"brand": brand, "serial": serial, "seller_id": sellerID}, &out)
	return out.PassportID, out.PassportExisted, err
}

func (c *Chain) OpenSale(ctx context.Context, r settlement.OpenRequest) (*settlement.Sale, error) {
	var out settlement.Sale
	err := do(ctx, defaultHTTP(c.HTTP), http.MethodPost, c.url("/open-sale"), "", map[string]any{
		"auction_ref": r.AuctionRef, "brand": r.Brand, "model": r.Model, "serial": r.Serial,
		"seller_id": r.SellerID, "winner_id": r.WinnerID, "listing_photos_hash": r.PhotosHash,
		"deadline_days": r.DeadlineDays,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Chain) GetSale(ctx context.Context, saleID string) (*settlement.Sale, error) {
	var out settlement.Sale
	if err := do(ctx, defaultHTTP(c.HTTP), http.MethodGet, c.url("/sale/"+url.PathEscape(saleID)), "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Chain) CompleteSale(ctx context.Context, saleID string) error {
	return do(ctx, defaultHTTP(c.HTTP), http.MethodPost, c.url("/complete-sale"), "", map[string]string{"sale_id": saleID}, nil)
}

func (c *Chain) CancelSale(ctx context.Context, saleID, reason string) error {
	return do(ctx, defaultHTTP(c.HTTP), http.MethodPost, c.url("/cancel-sale"), "",
		map[string]string{"sale_id": saleID, "reason": reason}, nil)
}
