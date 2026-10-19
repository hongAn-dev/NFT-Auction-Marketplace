package clients

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bidding-service/internal/auction"
	"bidding-service/internal/settlement"
)

type seen struct {
	method, path, token string
	body                map[string]any
}

func server(t *testing.T, status int, reply string, got *seen) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*got = seen{r.Method, r.URL.RequestURI(), r.Header.Get("x-service-token"), map[string]any{}}
		_ = json.Unmarshal(raw, &got.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestEscrowClient(t *testing.T) {
	var got seen
	ctx := context.Background()
	ok := server(t, 201, `{"success":true}`, &got)
	e := &Escrow{Base: ok.URL, Token: "bi-mat"}
	if err := e.Hold(ctx, "bid-1", "alice", 2_000_000); err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/internal/escrow/holds" || got.token != "bi-mat" ||
		got.body["id"] != "bid-1" || got.body["userId"] != "alice" || got.body["amount"] != float64(2_000_000) {
		t.Fatalf("yêu cầu hold sai: %+v", got)
	}
	_ = e.Capture(ctx, "bid-1", "seller")
	if got.path != "/internal/escrow/holds/bid-1/capture" || got.body["sellerId"] != "seller" {
		t.Fatalf("yêu cầu capture sai: %+v", got)
	}
	_ = e.Refund(ctx, "bid-1")
	if got.path != "/internal/escrow/holds/bid-1/refund" {
		t.Fatalf("yêu cầu refund sai: %+v", got)
	}

	poor := &Escrow{Base: server(t, 402, `{"code":"INSUFFICIENT_FUNDS"}`, &got).URL}
	if err := poor.Hold(ctx, "bid-2", "alice", 1); !errors.Is(err, auction.ErrInsufficientFunds) {
		t.Fatalf("402 phải là ErrInsufficientFunds, nhận %v", err)
	}
	conflict := &Escrow{Base: server(t, 409, `{"code":"CONFLICT","message":"đã CAPTURED"}`, &got).URL}
	err := conflict.Refund(ctx, "bid-1")
	if err == nil || settlement.IsTransient(err) {
		t.Fatalf("409 là lỗi vĩnh viễn, nhận %v", err)
	}
	if !errors.Is(conflict.Release(ctx, "bid-1"), ErrAlreadySettled) {
		t.Fatal("release một khoản đã chốt (409) phải là ErrAlreadySettled")
	}
	down := &Escrow{Base: server(t, 503, `{}`, &got).URL}
	if err := down.Refund(ctx, "bid-1"); !settlement.IsTransient(err) {
		t.Fatalf("503 phải thử lại được, nhận %v", err)
	}
	dead := &Escrow{Base: "http://127.0.0.1:1", HTTP: &http.Client{Timeout: time.Second}}
	if err := dead.Refund(ctx, "bid-1"); !settlement.IsTransient(err) {
		t.Fatalf("lỗi mạng phải thử lại được, nhận %v", err)
	}

	stale := &Escrow{Base: server(t, 200, `{"success":true,"data":[{"id":"bid-7"},{"id":"bid-8"}]}`, &got).URL}
	ids, err := stale.Stale(ctx, 5*time.Minute)
	if err != nil || len(ids) != 2 || ids[0] != "bid-7" || got.path != "/internal/escrow/holds/stale?olderThanSec=300" {
		t.Fatalf("stale sai: %v %v %+v", ids, err, got)
	}
}

func TestChainClient(t *testing.T) {
	var got seen
	ctx := context.Background()
	open := server(t, 200, `{"sale_id":"0xs","passport_id":"0xp","status":"OPEN","deadline_at":"2026-10-27T00:00:00Z","tx_id":"t"}`, &got)
	c := &Chain{Base: open.URL}
	sale, err := c.OpenSale(ctx, settlement.OpenRequest{AuctionRef: "lot-1-stl-1", Brand: "ROLEX", Model: "M",
		Serial: "S1", SellerID: "seller", WinnerID: "winner", PhotosHash: "0xab", DeadlineDays: 7})
	if err != nil || sale.SaleID != "0xs" || sale.DeadlineAt != "2026-10-27T00:00:00Z" {
		t.Fatalf("open-sale sai: %+v %v", sale, err)
	}
	if got.path != "/api/v1/passport/open-sale" || got.body["auction_ref"] != "lot-1-stl-1" ||
		got.body["seller_id"] != "seller" || got.body["deadline_days"] != float64(7) || got.body["listing_photos_hash"] != "0xab" {
		t.Fatalf("yêu cầu open-sale sai: %+v", got)
	}

	chk := &Chain{Base: server(t, 200, `{"passport_id":"0xp","passport_existed":true}`, &got).URL}
	pid, existed, err := chk.ListingCheck(ctx, "ROLEX", "S1", "seller")
	if err != nil || pid != "0xp" || !existed || got.body["seller_id"] != "seller" {
		t.Fatalf("listing-check sai: %s %v %v %+v", pid, existed, err, got)
	}
	copied := &Chain{Base: server(t, 409, `{"code":"SERIAL_OWNED_BY_OTHER","message":"của người khác"}`, &got).URL}
	_, _, err = copied.ListingCheck(ctx, "ROLEX", "S1", "seller")
	var api *APIError
	if !errors.As(err, &api) || api.Code != "SERIAL_OWNED_BY_OTHER" || api.Status != 409 {
		t.Fatalf("E9 phải trả APIError SERIAL_OWNED_BY_OTHER, nhận %v", err)
	}

	busy := &Chain{Base: server(t, 503, `{"code":"Unavailable"}`, &got).URL}
	if _, err := busy.GetSale(ctx, "0xs"); !settlement.IsTransient(err) {
		t.Fatalf("503 phải thử lại được, nhận %v", err)
	}
	if got.path != "/api/v1/passport/sale/0xs" {
		t.Fatalf("đường dẫn get-sale sai: %s", got.path)
	}
	rejected := &Chain{Base: server(t, 409, `{"code":"FailedPrecondition","message":"[PP-005]"}`, &got).URL}
	if err := rejected.CompleteSale(ctx, "0xs"); err == nil || settlement.IsTransient(err) {
		t.Fatalf("409 là lỗi vĩnh viễn, nhận %v", err)
	}
	if err := (&Chain{Base: server(t, 200, `{}`, &got).URL}).CancelSale(ctx, "0xs", "AUTH_TIMEOUT"); err != nil ||
		got.body["reason"] != "AUTH_TIMEOUT" || got.path != "/api/v1/passport/cancel-sale" {
		t.Fatalf("cancel-sale sai: %v %+v", err, got)
	}
}
