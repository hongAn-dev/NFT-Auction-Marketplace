// Package owner tính bí danh chủ sở hữu (ownerRef) cho chuỗi (docs/23 §7.2).
//
// Sàn không bao giờ gửi userId lên Fabric hay Besu: ownerRef = HMAC-SHA256(K,
// "angrad-owner-v1:" + userId). Không có K thì không ai nối được bí danh với người
// dùng; mất K thì sàn cũng không tính lại được bí danh cho người dùng cũ (giới hạn
// đã ghi ở docs/19 §11 câu 3).
package owner

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const domain = "angrad-owner-v1:"

type Aliaser struct{ key []byte }

func New(key []byte) *Aliaser { return &Aliaser{key: append([]byte(nil), key...)} }

// Load đọc khoá 32 byte (hex) từ tệp — khoá không đi qua biến môi trường.
func Load(path string) (*Aliaser, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("đọc khoá bí danh: %w", err)
	}
	key, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(string(raw)), "0x"))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("khoá bí danh trong %s phải là 32 byte hex", path)
	}
	return New(key), nil
}

// Ref: bí danh 0x + 64 hex của một người dùng.
func (a *Aliaser) Ref(userID string) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(domain + userID))
	return "0x" + hex.EncodeToString(mac.Sum(nil))
}
