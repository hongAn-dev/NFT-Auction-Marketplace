package auction

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"bidding-service/internal/domain"
)

// ListingChecker: blockchain-service /listing-check (docs/23 §7.1).
type ListingChecker interface {
	ListingCheck(ctx context.Context, brand, serial, sellerID string) (passportID string, existed bool, err error)
}

type ListingInput struct {
	ID, Title, Description, ImageURL, SellerID string
	StartPrice                                 float64
	Brand, Model, Serial                       string
	DurationHours                              int
}

// Lister: đăng bán món hàng hiệu (docs/23 §5.1, §7.1).
type Lister struct {
	Repo  domain.NFTRepository
	Check ListingChecker
	Now   func() time.Time
}

const maxDurationHours = 7 * 24

func (l *Lister) List(ctx context.Context, in ListingInput) (*domain.NFT, error) {
	switch {
	case in.ID == "" || in.Title == "" || in.SellerID == "" || in.ImageURL == "":
		return nil, reject("cần id, title, image_url, creator_id")
	case strings.TrimSpace(in.Brand) == "" || strings.TrimSpace(in.Serial) == "" || strings.TrimSpace(in.Model) == "":
		return nil, reject("cần thương hiệu, mẫu và serial")
	case in.DurationHours < 1 || in.DurationHours > maxDurationHours:
		return nil, reject("thời lượng phiên phải từ 1 giờ tới 7 ngày")
	case in.StartPrice <= 0 || in.StartPrice != math.Trunc(in.StartPrice) || in.StartPrice > maxAmount:
		return nil, reject("giá khởi điểm phải là số nguyên VND dương")
	}
	pid, _, err := l.Check.ListingCheck(ctx, strings.ToUpper(strings.TrimSpace(in.Brand)), in.Serial, in.SellerID)
	if err != nil {
		var coded interface{ RejectionCode() string }
		if errors.As(err, &coded) && coded.RejectionCode() != "" {
			return nil, &Rejected{Code: coded.RejectionCode(), Reason: err.Error()}
		}
		return nil, fmt.Errorf("kiểm hộ chiếu khi đăng bán: %w", err)
	}
	now := l.Now()
	ends := now.Add(time.Duration(in.DurationHours) * time.Hour)
	lot := &domain.NFT{ID: in.ID, Title: in.Title, Description: in.Description, ImageUrl: in.ImageURL,
		CreatorID: in.SellerID, OwnerID: in.SellerID, StartPrice: in.StartPrice, CurrentPrice: in.StartPrice,
		Status: domain.LotActive, BrandCode: strings.ToUpper(strings.TrimSpace(in.Brand)), Model: in.Model,
		Serial: in.Serial, PassportID: pid, EndsAt: &ends, CreatedAt: now, UpdatedAt: now}
	if err := l.Repo.Create(ctx, lot); err != nil {
		// Chỉ mục uniq_nfts_open_passport: hộ chiếu đang nằm trong lot khác của sàn.
		if strings.Contains(err.Error(), "uniq_nfts_open_passport") {
			return nil, &Rejected{Code: "PASSPORT_LOCKED", Reason: "món đồ này đang được đấu giá ở một lot khác"}
		}
		return nil, err
	}
	return lot, nil
}
