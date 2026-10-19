package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bidding-service/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Migrate: bảng của sàn (docs/23 §5) cộng chỉ mục một phần cho passport_id — một
// hộ chiếu chỉ nằm trong tối đa MỘT lot đang mở hoặc đang thanh toán.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&domain.NFT{}, &domain.OutboxEvent{}, &domain.Bid{}, &domain.Settlement{}); err != nil {
		return err
	}
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uniq_nfts_open_passport ON nfts (passport_id)
		WHERE passport_id <> '' AND status IN ('active', 'settling')`).Error
}

// AuctionStore: phần DB của đặt giá, đóng phiên, đối soát và saga thanh toán.
type AuctionStore struct {
	DB *gorm.DB
	// Invalidate: xoá cache Redis của lot sau mỗi lần ghi lot (nil = bỏ qua).
	Invalidate func(ctx context.Context, lotID string)
}

func (s *AuctionStore) invalidate(ctx context.Context, lotID string) {
	if s.Invalidate != nil {
		s.Invalidate(ctx, lotID)
	}
}

func (s *AuctionStore) Lot(ctx context.Context, id string) (*domain.NFT, error) {
	var lot domain.NFT
	err := s.DB.WithContext(ctx).First(&lot, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNFTNotFound
	}
	return &lot, err
}

func (s *AuctionStore) Highest(ctx context.Context, lotID string) (*domain.Bid, error) {
	var b domain.Bid
	err := s.DB.WithContext(ctx).Where("nft_id = ? AND status = ?", lotID, domain.BidHeld).
		Order("amount DESC").First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &b, err
}

func (s *AuctionStore) CommitBid(ctx context.Context, lot *domain.NFT, bid *domain.Bid, prevBidID string, events []domain.OutboxEvent) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Điều kiện status = active: đóng phiên chen vào giữa (dù đã có khoá Redis)
		// thì lượt đặt giá hỏng, Bidder nhả khoản giữ.
		res := tx.Model(&domain.NFT{}).Where("id = ? AND status = ?", lot.ID, domain.LotActive).
			Updates(map[string]any{"current_price": lot.CurrentPrice, "highest_bid_id": lot.HighestBidID, "updated_at": lot.UpdatedAt})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("lot %s không còn mở", lot.ID)
		}
		if err := tx.Create(bid).Error; err != nil {
			return err
		}
		if prevBidID != "" {
			if err := tx.Model(&domain.Bid{}).Where("id = ?", prevBidID).
				Updates(map[string]any{"status": domain.BidReleased, "updated_at": bid.CreatedAt}).Error; err != nil {
				return err
			}
		}
		for i := range events {
			if err := tx.Create(&events[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.invalidate(ctx, lot.ID)
	}
	return err
}

func (s *AuctionStore) DueLots(ctx context.Context, now time.Time, limit int) ([]string, error) {
	var ids []string
	err := s.DB.WithContext(ctx).Model(&domain.NFT{}).
		Where("status = ? AND ends_at IS NOT NULL AND ends_at <= ?", domain.LotActive, now).
		Order("ends_at").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func (s *AuctionStore) CloseLot(ctx context.Context, lotID string, now time.Time) (*domain.Settlement, error) {
	var out *domain.Settlement
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lot domain.NFT
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			First(&lot, "id = ?", lotID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // đang bị tiến trình khác khoá — lần sau
		}
		if err != nil {
			return err
		}
		if lot.Status != domain.LotActive || lot.EndsAt == nil || lot.EndsAt.After(now) {
			return nil
		}
		var win domain.Bid
		err = tx.Where("nft_id = ? AND status = ?", lotID, domain.BidHeld).Order("amount DESC").First(&win).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Model(&lot).Updates(map[string]any{"status": domain.LotUnsold, "updated_at": now}).Error
		}
		if err != nil {
			return err
		}
		id := fmt.Sprintf("stl-%d", now.UnixNano())
		st := &domain.Settlement{ID: id, LotID: lotID, SellerID: lot.OwnerID, WinnerID: win.UserID, Amount: win.Amount,
			HoldID: win.HoldID, AuctionRef: lotID + "-" + id, PassportID: lot.PassportID, State: domain.StlPendingOpen,
			ClosedAt: now, NextAttemptAt: now}
		if err := tx.Create(st).Error; err != nil {
			return err
		}
		if err := tx.Model(&win).Updates(map[string]any{"status": domain.BidWon, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&lot).Updates(map[string]any{"status": domain.LotSettling, "updated_at": now}).Error; err != nil {
			return err
		}
		out = st
		return nil
	})
	if err == nil {
		s.invalidate(ctx, lotID)
	}
	return out, err
}

func (s *AuctionStore) BidExistsForHold(ctx context.Context, holdID string) (bool, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&domain.Bid{}).Where("hold_id = ?", holdID).Count(&n).Error
	return n > 0, err
}

// ---- settlement.Store -------------------------------------------------------

var terminal = []string{domain.StlSold, domain.StlRefunded, domain.StlNeedsAttention}

func (s *AuctionStore) Due(ctx context.Context, now time.Time, limit int) ([]*domain.Settlement, error) {
	var out []*domain.Settlement
	err := s.DB.WithContext(ctx).Where("state NOT IN ? AND next_attempt_at <= ?", terminal, now).
		Order("next_attempt_at").Limit(limit).Find(&out).Error
	return out, err
}

func (s *AuctionStore) Get(ctx context.Context, id string) (*domain.Settlement, error) {
	var st domain.Settlement
	if err := s.DB.WithContext(ctx).First(&st, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

// SettlementOfLot: saga của một lot (nil nếu lot chưa đóng phiên).
func (s *AuctionStore) SettlementOfLot(ctx context.Context, lotID string) (*domain.Settlement, error) {
	var st domain.Settlement
	err := s.DB.WithContext(ctx).First(&st, "lot_id = ?", lotID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &st, err
}

func save(tx *gorm.DB, st *domain.Settlement, from string) (bool, error) {
	res := tx.Model(&domain.Settlement{}).Where("id = ? AND state = ?", st.ID, from).Select("*").Updates(st)
	return res.RowsAffected == 1, res.Error
}

func (s *AuctionStore) Save(ctx context.Context, st *domain.Settlement, from string) (bool, error) {
	return save(s.DB.WithContext(ctx), st, from)
}

func (s *AuctionStore) Finish(ctx context.Context, st *domain.Settlement, from, lotStatus, newOwner string, events []domain.OutboxEvent) (bool, error) {
	ok := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if ok, err = save(tx, st, from); err != nil || !ok {
			return err
		}
		upd := map[string]any{"status": lotStatus, "updated_at": time.Now()}
		if newOwner != "" {
			upd["owner_id"] = newOwner
		}
		if err := tx.Model(&domain.NFT{}).Where("id = ?", st.LotID).Updates(upd).Error; err != nil {
			return err
		}
		for i := range events {
			if err := tx.Create(&events[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil && ok {
		s.invalidate(ctx, st.LotID)
	}
	return ok, err
}
