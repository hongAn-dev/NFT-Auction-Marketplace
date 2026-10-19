// Package api: HTTP cho bidding-service, phía sau là passport-service vai platform
// (gRPC passport.v1). Đây là chỗ DUY NHẤT của sàn chạm chuỗi (docs/23 §3).
package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"blockchain-service/internal/owner"
	pb "blockchain-service/internal/passportv1"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// callTimeout: một giao dịch Fabric (endorse + order + commit) mất vài giây.
const callTimeout = 30 * time.Second

type server struct {
	pp    pb.PassportServiceClient
	alias *owner.Aliaser
}

func NewRouter(pp pb.PassportServiceClient, alias *owner.Aliaser) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	s := &server{pp: pp, alias: alias}
	g := r.Group("/api/v1/passport")
	g.POST("/listing-check", s.listingCheck)
	g.POST("/open-sale", s.openSale)
	g.GET("/sale/:id", s.getSale)
	g.POST("/complete-sale", s.completeSale)
	g.POST("/cancel-sale", s.cancelSale)
	g.GET("/passport/:id/history", s.history)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "blockchain-service"})
	})
	return r
}

// httpStatus: mã gRPC → HTTP (docs/23 §6.3). 503 nghĩa là "thử lại được"; mọi mã
// khác là lỗi vĩnh viễn với bidding-service.
func httpStatus(c codes.Code) int {
	switch c {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Aborted, codes.ResourceExhausted:
		return http.StatusServiceUnavailable
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.FailedPrecondition, codes.AlreadyExists:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func fail(c *gin.Context, err error) {
	st := status.Convert(err)
	c.JSON(httpStatus(st.Code()), gin.H{"code": st.Code().String(), "message": st.Message()})
}

func bad(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"code": codes.InvalidArgument.String(), "message": msg})
}

// reply: protobuf → JSON snake_case, giữ cả trường rỗng để phía Go đọc thẳng.
func reply(c *gin.Context, m proto.Message) {
	raw, err := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}.Marshal(m)
	if err != nil {
		fail(c, status.Error(codes.Internal, err.Error()))
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func ctx(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), callTimeout)
}

// listingCheck: docs/23 §7.1 — E9 phát hiện serial bị chép lúc đăng bán.
func (s *server) listingCheck(c *gin.Context) {
	var in struct {
		Brand    string `json:"brand"`
		Serial   string `json:"serial"`
		SellerID string `json:"seller_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Brand == "" || in.Serial == "" || in.SellerID == "" {
		bad(c, "cần brand, serial, seller_id")
		return
	}
	cx, cancel := ctx(c)
	defer cancel()
	ref, err := s.pp.DerivePassportID(cx, &pb.DeriveRequest{BrandCode: in.Brand, Serial: in.Serial})
	if err != nil {
		fail(c, err)
		return
	}
	pp, err := s.pp.GetPassport(cx, &pb.PassportRef{PassportId: ref.PassportId})
	if status.Code(err) == codes.NotFound {
		c.JSON(http.StatusOK, gin.H{"passport_id": ref.PassportId, "passport_existed": false})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if pp.OwnerRef != s.alias.Ref(in.SellerID) {
		c.JSON(http.StatusConflict, gin.H{"code": "SERIAL_OWNED_BY_OTHER",
			"message": "serial này đã có hộ chiếu số của người khác"})
		return
	}
	if pp.Status != "ACTIVE" {
		c.JSON(http.StatusConflict, gin.H{"code": "PASSPORT_LOCKED",
			"message": "hộ chiếu đang nằm trong một vụ bán khác"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"passport_id": ref.PassportId, "passport_existed": true})
}

func (s *server) openSale(c *gin.Context) {
	var in struct {
		AuctionRef        string `json:"auction_ref"`
		Brand             string `json:"brand"`
		Model             string `json:"model"`
		Serial            string `json:"serial"`
		SellerID          string `json:"seller_id"`
		WinnerID          string `json:"winner_id"`
		ListingPhotosHash string `json:"listing_photos_hash"`
		DeadlineDays      int32  `json:"deadline_days"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.SellerID == "" || in.WinnerID == "" {
		bad(c, "payload open-sale không hợp lệ")
		return
	}
	cx, cancel := ctx(c)
	defer cancel()
	sale, err := s.pp.OpenSale(cx, &pb.OpenSaleRequest{
		AuctionRef: in.AuctionRef, BrandCode: in.Brand, Model: in.Model, Serial: in.Serial,
		SellerRef: s.alias.Ref(in.SellerID), WinnerRef: s.alias.Ref(in.WinnerID),
		ListingPhotosHash: in.ListingPhotosHash, DeadlineDays: in.DeadlineDays,
	})
	if err != nil {
		fail(c, err)
		return
	}
	reply(c, sale)
}

func (s *server) getSale(c *gin.Context) {
	cx, cancel := ctx(c)
	defer cancel()
	sale, err := s.pp.GetSale(cx, &pb.SaleRef{SaleId: c.Param("id")})
	if err != nil {
		fail(c, err)
		return
	}
	reply(c, sale)
}

func (s *server) completeSale(c *gin.Context) {
	var in struct {
		SaleID string `json:"sale_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.SaleID == "" {
		bad(c, "cần sale_id")
		return
	}
	cx, cancel := ctx(c)
	defer cancel()
	pp, err := s.pp.CompleteSale(cx, &pb.SaleRef{SaleId: in.SaleID})
	if err != nil {
		fail(c, err)
		return
	}
	reply(c, pp)
}

func (s *server) cancelSale(c *gin.Context) {
	var in struct {
		SaleID string `json:"sale_id"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.SaleID == "" || in.Reason == "" {
		bad(c, "cần sale_id và reason")
		return
	}
	cx, cancel := ctx(c)
	defer cancel()
	sale, err := s.pp.CancelSale(cx, &pb.CancelSaleRequest{SaleId: in.SaleID, ReasonCode: strings.ToUpper(in.Reason)})
	if err != nil {
		fail(c, err)
		return
	}
	reply(c, sale)
}

func (s *server) history(c *gin.Context) {
	cx, cancel := ctx(c)
	defer cancel()
	h, err := s.pp.GetHistory(cx, &pb.PassportRef{PassportId: c.Param("id")})
	if err != nil {
		fail(c, err)
		return
	}
	reply(c, h)
}
