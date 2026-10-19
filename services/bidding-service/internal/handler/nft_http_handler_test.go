package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"bidding-service/internal/auction"
	"bidding-service/internal/domain"
	"bidding-service/internal/repository"
	"bidding-service/internal/usecase"

	"github.com/gin-gonic/gin"
)

type stubChecker struct{ err error }

func (s stubChecker) ListingCheck(context.Context, string, string, string) (string, bool, error) {
	return "0xpp", false, s.err
}

type e9 struct{}

func (e9) Error() string         { return "serial này đã có hộ chiếu số của người khác" }
func (e9) RejectionCode() string { return "SERIAL_OWNED_BY_OTHER" }

func httpRouter(chk auction.ListingChecker, stl SettlementReader) (*gin.Engine, domain.NFTRepository) {
	gin.SetMode(gin.TestMode)
	repo := repository.NewMockNFTRepository()
	uc := usecase.NewNFTUsecase(repo, nil)
	h := NewNFTHTTPHandler(uc, nil, &auction.Lister{Repo: repo, Check: chk, Now: func() time.Time { return t0 }}, stl)
	r := gin.New()
	h.RegisterRoutes(r)
	return r, repo
}

func send(r *gin.Engine, method, path string, body any) (int, map[string]any) {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

var listing = map[string]any{"id": "lot-9", "title": "Submariner", "image_url": "https://anh/1.jpg",
	"creator_id": "seller", "start_price": 100000000, "brand_code": "ROLEX", "model": "Submariner Date",
	"serial": "116610LN7Z3X9Q1", "duration_hours": 24}

func TestCreateLot(t *testing.T) {
	r, _ := httpRouter(stubChecker{}, nil)
	code, out := send(r, "POST", "/api/v1/nfts", listing)
	data, _ := out["data"].(map[string]any)
	if code != 201 || data["passport_id"] != "0xpp" || data["ends_at"] == nil {
		t.Fatalf("đăng bán hợp lệ phải 201 kèm passport_id và ends_at: %d %v", code, out)
	}
	r, _ = httpRouter(stubChecker{err: e9{}}, nil)
	code, out = send(r, "POST", "/api/v1/nfts", listing)
	errBody, _ := out["error"].(map[string]any)
	if code != 409 || errBody["code"] != "SERIAL_OWNED_BY_OTHER" {
		t.Fatalf("E9 phải 409 SERIAL_OWNED_BY_OTHER, nhận %d %v", code, out)
	}
	noSerial := map[string]any{}
	for k, v := range listing {
		noSerial[k] = v
	}
	delete(noSerial, "serial")
	if code, _ = send(r, "POST", "/api/v1/nfts", noSerial); code != 400 {
		t.Fatalf("thiếu serial phải 400, nhận %d", code)
	}
}

// docs/23 §5.1: lot có hộ chiếu không sửa / xoá được qua API không xác thực này.
func TestPassportLotIsLocked(t *testing.T) {
	r, repo := httpRouter(stubChecker{}, nil)
	_ = repo.Create(context.Background(), &domain.NFT{ID: "lot-pp", Title: "x", StartPrice: 1, PassportID: "0xpp", Status: "active"})
	if code, out := send(r, "PUT", "/api/v1/nfts/lot-pp", map[string]any{"status": "sold"}); code != 409 {
		t.Fatalf("sửa lot có hộ chiếu phải 409, nhận %d %v", code, out)
	}
	if code, _ := send(r, "DELETE", "/api/v1/nfts/lot-pp", nil); code != 409 {
		t.Fatalf("xoá lot có hộ chiếu phải 409, nhận %d", code)
	}
	if code, _ := send(r, "PUT", "/api/v1/nfts/nft-1", map[string]any{"title": "moi"}); code != 200 {
		t.Fatalf("lot cũ (không hộ chiếu) vẫn sửa được, nhận %d", code)
	}
}

type oneSettlement struct{ s *domain.Settlement }

func (o oneSettlement) SettlementOfLot(context.Context, string) (*domain.Settlement, error) {
	return o.s, nil
}

func TestGetSettlement(t *testing.T) {
	r, _ := httpRouter(stubChecker{}, oneSettlement{&domain.Settlement{ID: "stl-1", LotID: "nft-1", State: domain.StlAwaitAuth}})
	code, out := send(r, "GET", "/api/v1/nfts/nft-1/settlement", nil)
	data, _ := out["data"].(map[string]any)
	if code != 200 || data["state"] != domain.StlAwaitAuth {
		t.Fatalf("muốn 200 kèm trạng thái saga, nhận %d %v", code, out)
	}
	r, _ = httpRouter(stubChecker{}, oneSettlement{})
	if code, out = send(r, "GET", "/api/v1/nfts/nft-1/settlement", nil); code != 200 || out["data"] != nil {
		t.Fatalf("chưa đóng phiên: data null, nhận %d %v", code, out)
	}
}
