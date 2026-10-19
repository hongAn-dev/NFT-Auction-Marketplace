package owner

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestRefIsHMACOfUserID(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	a := New(key)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("angrad-owner-v1:user-1"))
	want := "0x" + hex.EncodeToString(mac.Sum(nil))
	if got := a.Ref("user-1"); got != want {
		t.Fatalf("Ref = %s, muốn %s", got, want)
	}
	if a.Ref("user-1") == a.Ref("user-2") {
		t.Fatal("hai người dùng khác nhau phải có bí danh khác nhau")
	}
	if New(bytes.Repeat([]byte{0x02}, 32)).Ref("user-1") == want {
		t.Fatal("khoá khác phải ra bí danh khác")
	}
}

func TestLoadKeyFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.key")
	_ = os.WriteFile(good, []byte(hex.EncodeToString(bytes.Repeat([]byte{0x01}, 32))+"\n"), 0o600)
	a, err := Load(good)
	if err != nil {
		t.Fatal(err)
	}
	if a.Ref("user-1") != New(bytes.Repeat([]byte{0x01}, 32)).Ref("user-1") {
		t.Fatal("khoá đọc từ tệp hex phải giống khoá thô")
	}
	short := filepath.Join(dir, "short.key")
	_ = os.WriteFile(short, []byte("0101"), 0o600)
	if _, err := Load(short); err == nil {
		t.Fatal("khoá ngắn hơn 32 byte phải bị từ chối")
	}
	if _, err := Load(filepath.Join(dir, "khong-co")); err == nil {
		t.Fatal("thiếu tệp khoá phải báo lỗi")
	}
}
