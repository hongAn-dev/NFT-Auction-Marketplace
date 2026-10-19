package domain

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID: <prefix>-<32 hex ngẫu nhiên>. Dùng cho bid (= khoá idempotency của ký
// quỹ) và settlement; chỉ gồm ký tự hợp lệ với auctionRef của chaincode.
func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand hỏng: " + err.Error()) // không bao giờ xảy ra trên Linux
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
