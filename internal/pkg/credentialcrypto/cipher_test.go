package credentialcrypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestCipherRoundTripAndRandomNonce(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	c, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	aad := AAD(42, "authentication", 1)
	one, err := c.Encrypt([]byte(`{"clientSecret":"secret"}`), aad)
	if err != nil {
		t.Fatal(err)
	}
	two, err := c.Encrypt([]byte(`{"clientSecret":"secret"}`), aad)
	if err != nil {
		t.Fatal(err)
	}
	if one.Ciphertext == two.Ciphertext || one.Nonce == two.Nonce {
		t.Fatal("相同明文必须使用不同随机 nonce")
	}
	plain, err := c.Decrypt(*one, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, []byte(`{"clientSecret":"secret"}`)) {
		t.Fatalf("unexpected plaintext: %s", plain)
	}
	if _, err := c.Decrypt(*one, AAD(43, "authentication", 1)); err == nil {
		t.Fatal("错误 AAD 不应解密成功")
	}
}
