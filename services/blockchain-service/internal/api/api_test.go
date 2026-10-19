package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"blockchain-service/internal/owner"
	pb "blockchain-service/internal/passportv1"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const pid = "0x1290dfbf6a971b147440f0e5f7fd9d7dc1d0a1129c06ce9cf6c453777589c857"

// fakePassport: passport-service giả, chạy thật qua gRPC trên bufconn.
type fakePassport struct {
	pb.UnimplementedPassportServiceServer
	deriveErr error
	passport  *pb.Passport // nil = chưa có hộ chiếu (NotFound)
	sale      *pb.Sale
	saleErr   error
	opened    *pb.OpenSaleRequest
	cancelled *pb.CancelSaleRequest
}

func (f *fakePassport) DerivePassportID(_ context.Context, r *pb.DeriveRequest) (*pb.PassportRef, error) {
	if f.deriveErr != nil {
		return nil, f.deriveErr
	}
	return &pb.PassportRef{PassportId: pid}, nil
}

func (f *fakePassport) GetPassport(context.Context, *pb.PassportRef) (*pb.Passport, error) {
	if f.passport == nil {
		return nil, status.Error(codes.NotFound, "[PP-003] không tìm thấy hộ chiếu")
	}
	return f.passport, nil
}

func (f *fakePassport) OpenSale(_ context.Context, r *pb.OpenSaleRequest) (*pb.Sale, error) {
	f.opened = r
	return &pb.Sale{SaleId: "0xsale", PassportId: pid, Status: "OPEN", TxId: "tx-open"}, nil
}

func (f *fakePassport) GetSale(context.Context, *pb.SaleRef) (*pb.Sale, error) {
	return f.sale, f.saleErr
}

func (f *fakePassport) CancelSale(_ context.Context, r *pb.CancelSaleRequest) (*pb.Sale, error) {
	f.cancelled = r
	return &pb.Sale{SaleId: r.SaleId, Status: "CANCELLED", ReasonCode: r.ReasonCode, TxId: "tx-cancel"}, nil
}

var alias = owner.New(bytes.Repeat([]byte{0x01}, 32))

func router(t *testing.T, f *fakePassport) *gin.Engine {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterPassportServiceServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewRouter(pb.NewPassportServiceClient(conn), alias)
}

func call(t *testing.T, r *gin.Engine, method, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

var check = map[string]string{"brand": "ROLEX", "serial": "116610LN7Z3X9Q1", "seller_id": "seller-1"}

// docs/23 §7.1: bốn nhánh của listing-check.
func TestListingCheck(t *testing.T) {
	code, out := call(t, router(t, &fakePassport{}), "POST", "/api/v1/passport/listing-check", check)
	if code != 200 || out["passport_id"] != pid || out["passport_existed"] != false {
		t.Fatalf("chưa có hộ chiếu: muốn 200 lần bán đầu, nhận %d %v", code, out)
	}

	mine := &pb.Passport{PassportId: pid, OwnerRef: alias.Ref("seller-1"), Status: "ACTIVE"}
	code, out = call(t, router(t, &fakePassport{passport: mine}), "POST", "/api/v1/passport/listing-check", check)
	if code != 200 || out["passport_existed"] != true {
		t.Fatalf("bán lại đồ của mình: muốn 200, nhận %d %v", code, out)
	}

	other := &pb.Passport{PassportId: pid, OwnerRef: alias.Ref("nguoi-khac"), Status: "ACTIVE"}
	code, out = call(t, router(t, &fakePassport{passport: other}), "POST", "/api/v1/passport/listing-check", check)
	if code != 409 || out["code"] != "SERIAL_OWNED_BY_OTHER" {
		t.Fatalf("E9 — serial của người khác: muốn 409 SERIAL_OWNED_BY_OTHER, nhận %d %v", code, out)
	}

	locked := &pb.Passport{PassportId: pid, OwnerRef: alias.Ref("seller-1"), Status: "IN_SALE"}
	code, out = call(t, router(t, &fakePassport{passport: locked}), "POST", "/api/v1/passport/listing-check", check)
	if code != 409 || out["code"] != "PASSPORT_LOCKED" {
		t.Fatalf("hộ chiếu đang khoá: muốn 409 PASSPORT_LOCKED, nhận %d %v", code, out)
	}

	bad := &fakePassport{deriveErr: status.Error(codes.InvalidArgument, "[PP-002] mã thương hiệu")}
	if code, _ = call(t, router(t, bad), "POST", "/api/v1/passport/listing-check", check); code != 400 {
		t.Fatalf("thương hiệu sai: muốn 400, nhận %d", code)
	}
	if code, _ = call(t, router(t, &fakePassport{}), "POST", "/api/v1/passport/listing-check", map[string]string{}); code != 400 {
		t.Fatalf("thiếu trường: muốn 400, nhận %d", code)
	}
}

// open-sale đổi userId sang ownerRef — sàn không bao giờ gửi userId lên chuỗi.
func TestOpenSaleSendsOwnerRefs(t *testing.T) {
	f := &fakePassport{}
	code, out := call(t, router(t, f), "POST", "/api/v1/passport/open-sale", map[string]any{
		"auction_ref": "lot-1-7", "brand": "ROLEX", "model": "Submariner", "serial": "116610LN7Z3X9Q1",
		"seller_id": "seller-1", "winner_id": "winner-1", "listing_photos_hash": "0x" + string(bytes.Repeat([]byte("ab"), 32)),
		"deadline_days": 7,
	})
	if code != 200 || out["sale_id"] != "0xsale" || out["tx_id"] != "tx-open" {
		t.Fatalf("muốn 200 với sale_id/tx_id, nhận %d %v", code, out)
	}
	if f.opened.SellerRef != alias.Ref("seller-1") || f.opened.WinnerRef != alias.Ref("winner-1") ||
		f.opened.AuctionRef != "lot-1-7" || f.opened.DeadlineDays != 7 {
		t.Fatalf("yêu cầu gửi passport-service sai: %+v", f.opened)
	}
}

// GET sale trả snake_case, có delivery_deadline_at; cancel-sale chuyển lý do.
func TestSaleAndCancel(t *testing.T) {
	f := &fakePassport{sale: &pb.Sale{SaleId: "0xsale", Status: "PASSED", DeliveryDeadlineAt: "2026-11-02T00:00:00Z"}}
	r := router(t, f)
	code, out := call(t, r, "GET", "/api/v1/passport/sale/0xsale", nil)
	if code != 200 || out["status"] != "PASSED" || out["delivery_deadline_at"] != "2026-11-02T00:00:00Z" || out["reason_code"] != "" {
		t.Fatalf("GET sale sai: %d %v", code, out)
	}
	code, out = call(t, r, "POST", "/api/v1/passport/cancel-sale", map[string]string{"sale_id": "0xsale", "reason": "AUTH_TIMEOUT"})
	if code != 200 || out["status"] != "CANCELLED" || f.cancelled.ReasonCode != "AUTH_TIMEOUT" {
		t.Fatalf("cancel-sale sai: %d %v", code, out)
	}
}

// docs/23 §6.3: bidding-service phân loại lỗi theo mã HTTP — 503 thử lại, còn lại
// là lỗi vĩnh viễn. Body luôn mang tên mã gRPC.
func TestErrorMapping(t *testing.T) {
	for c, want := range map[codes.Code]int{
		codes.Unavailable:        http.StatusServiceUnavailable,
		codes.DeadlineExceeded:   http.StatusServiceUnavailable,
		codes.Aborted:            http.StatusServiceUnavailable,
		codes.FailedPrecondition: http.StatusConflict,
		codes.PermissionDenied:   http.StatusForbidden,
		codes.NotFound:           http.StatusNotFound,
		codes.InvalidArgument:    http.StatusBadRequest,
		codes.Internal:           http.StatusInternalServerError,
	} {
		f := &fakePassport{saleErr: status.Error(c, "loi")}
		code, out := call(t, router(t, f), "GET", "/api/v1/passport/sale/0xsale", nil)
		if code != want || out["code"] != c.String() {
			t.Errorf("%s: muốn %d, nhận %d %v", c, want, code, out)
		}
	}
}
