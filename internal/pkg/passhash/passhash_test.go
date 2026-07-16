package passhash

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("expected argon2id hash, got %q", h)
	}
	if !Verify("correct horse battery staple", h) {
		t.Fatal("expected verify to succeed")
	}
	if Verify("wrong password", h) {
		t.Fatal("expected verify to fail for wrong password")
	}
}

func TestVerifyAndUpgradeFromBcrypt(t *testing.T) {
	// 模拟一个历史 bcrypt 哈希（gopasshash 能识别但非 target，应触发升级）。
	b, err := bcrypt.GenerateFromPassword([]byte("s3cr3t"), 12)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	bcryptHash := string(b)

	match, newHash := VerifyAndUpgrade("s3cr3t", bcryptHash)
	if !match {
		t.Fatal("expected match=true for correct password against bcrypt hash")
	}
	if newHash == "" || !strings.HasPrefix(newHash, "$argon2id$") {
		t.Fatalf("expected upgraded argon2id hash, got %q", newHash)
	}
	// 升级后的哈希仍可校验同一密码。
	if !Verify("s3cr3t", newHash) {
		t.Fatal("expected upgraded hash to verify")
	}

	// 错误密码不应匹配，也不返回新哈希。
	if m, nh := VerifyAndUpgrade("nope", bcryptHash); m || nh != "" {
		t.Fatalf("expected (false, \"\") for wrong password, got (%v, %q)", m, nh)
	}
}

func TestIsRecognized(t *testing.T) {
	argon, _ := Hash("x")
	if !IsRecognized(argon) {
		t.Fatal("argon2id hash should be recognized")
	}
	// 历史 sha256 十六进制串不是标准编码格式，应不被识别。
	if IsRecognized("5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8") {
		t.Fatal("raw sha256 hex should NOT be recognized")
	}
}
