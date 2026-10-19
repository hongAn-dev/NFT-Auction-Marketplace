package auction

import (
	"context"
	"errors"
	"testing"
	"time"

	"bidding-service/internal/domain"
	"bidding-service/internal/repository"
)

type fakeChecker struct {
	pid  string
	err  error
	args []string
}

func (f *fakeChecker) ListingCheck(_ context.Context, brand, serial, seller string) (string, bool, error) {
	f.args = []string{brand, serial, seller}
	return f.pid, false, f.err
}

// codeErr: lỗi có mã như *clients.APIError (409 SERIAL_OWNED_BY_OTHER …).
type codeErr struct{ code string }

func (e codeErr) Error() string         { return e.code }
func (e codeErr) RejectionCode() string { return e.code }

func input() ListingInput {
	return ListingInput{ID: "lot-9", Title: "Submariner", ImageURL: "https://anh/1.jpg", SellerID: "seller",
		StartPrice: 100_000_000, Brand: "ROLEX", Model: "Submariner Date", Serial: "116610ln-7z3x 9q1", DurationHours: 24}
}

func TestListLotRunsListingCheck(t *testing.T) {
	repo, chk := repository.NewMockNFTRepository(), &fakeChecker{pid: "0xpp"}
	l := &Lister{Repo: repo, Check: chk, Now: func() time.Time { return t0 }}
	lot, err := l.List(context.Background(), input())
	if err != nil {
		t.Fatal(err)
	}
	if lot.PassportID != "0xpp" || lot.EndsAt == nil || !lot.EndsAt.Equal(t0.Add(24*time.Hour)) ||
		lot.Status != domain.LotActive || lot.OwnerID != "seller" || lot.CurrentPrice != 100_000_000 {
		t.Fatalf("lot sai: %+v", lot)
	}
	if chk.args[0] != "ROLEX" || chk.args[2] != "seller" {
		t.Fatalf("listing-check phải nhận đúng thương hiệu và người bán: %v", chk.args)
	}
}

// E9: serial đã có hộ chiếu của người khác — từ chối, không tạo lot.
func TestListLotRejectsCopiedSerial(t *testing.T) {
	repo := repository.NewMockNFTRepository()
	l := &Lister{Repo: repo, Check: &fakeChecker{err: codeErr{"SERIAL_OWNED_BY_OTHER"}}, Now: func() time.Time { return t0 }}
	_, err := l.List(context.Background(), input())
	var r *Rejected
	if !errors.As(err, &r) || r.Code != "SERIAL_OWNED_BY_OTHER" {
		t.Fatalf("muốn Rejected SERIAL_OWNED_BY_OTHER, nhận %v", err)
	}
	if _, err := repo.GetByID(context.Background(), "lot-9"); err == nil {
		t.Fatal("bị từ chối thì không được tạo lot")
	}
}

func TestListLotValidation(t *testing.T) {
	for name, mutate := range map[string]func(*ListingInput){
		"thiếu serial":      func(i *ListingInput) { i.Serial = "" },
		"thiếu thương hiệu": func(i *ListingInput) { i.Brand = "" },
		"thời lượng 0 giờ":  func(i *ListingInput) { i.DurationHours = 0 },
		"thời lượng 8 ngày": func(i *ListingInput) { i.DurationHours = 8 * 24 },
		"giá lẻ đồng":       func(i *ListingInput) { i.StartPrice = 1.5 },
		"thiếu người bán":   func(i *ListingInput) { i.SellerID = "" },
	} {
		in := input()
		mutate(&in)
		chk := &fakeChecker{pid: "0xpp"}
		_, err := (&Lister{Repo: repository.NewMockNFTRepository(), Check: chk, Now: func() time.Time { return t0 }}).List(context.Background(), in)
		var r *Rejected
		if !errors.As(err, &r) || chk.args != nil {
			t.Errorf("%s: muốn Rejected trước khi hỏi chuỗi, nhận %v (args %v)", name, err, chk.args)
		}
	}
}
