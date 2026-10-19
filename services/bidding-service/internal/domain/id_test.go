package domain

import (
	"regexp"
	"testing"
)

// Review cuối 24b #2: ID là khoá idempotency của ký quỹ — không được trùng dù
// sinh song song (UnixNano có thể trùng trên đồng hồ thô).
func TestNewIDIsRandomAndChaincodeSafe(t *testing.T) {
	pat := regexp.MustCompile(`^bid-[0-9a-f]{32}$`)
	seen := map[string]bool{}
	for i := 0; i < 10000; i++ {
		id := NewID("bid")
		if !pat.MatchString(id) {
			t.Fatalf("ID sai dạng: %q", id)
		}
		if seen[id] {
			t.Fatalf("ID trùng: %q", id)
		}
		seen[id] = true
	}
}
