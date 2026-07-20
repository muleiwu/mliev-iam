package credentialcrypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
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

func TestKeyringDecryptsOldVersionAndWritesActiveVersion(t *testing.T) {
	oldKey := bytes.Repeat([]byte{1}, 32)
	newKey := bytes.Repeat([]byte{2}, 32)
	oldCipher, err := NewKeyring(map[int][]byte{1: oldKey}, 1)
	if err != nil {
		t.Fatal(err)
	}
	oldAAD := AAD(7, "directory", 1)
	oldEnvelope, err := oldCipher.Encrypt([]byte("old-secret"), oldAAD)
	if err != nil {
		t.Fatal(err)
	}

	keyring, err := NewKeyring(map[int][]byte{1: oldKey, 2: newKey}, 2)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := keyring.Decrypt(*oldEnvelope, oldAAD)
	if err != nil || string(plain) != "old-secret" {
		t.Fatalf("旧版本密文应可解密: plain=%q err=%v", plain, err)
	}
	newEnvelope, err := keyring.Encrypt([]byte("new-secret"), AAD(7, "directory", 2))
	if err != nil {
		t.Fatal(err)
	}
	if newEnvelope.KeyVersion != 2 || keyring.ActiveKeyVersion() != 2 {
		t.Fatalf("写入必须使用 active version 2: %#v", newEnvelope)
	}
}

func TestFromEnvironmentKeyring(t *testing.T) {
	key1 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	key2 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	t.Setenv(EnvironmentKey, "")
	t.Setenv(EnvironmentKeys, `{"1":"`+key1+`","2":"`+key2+`"}`)
	t.Setenv(EnvironmentActiveKeyVersion, "2")
	keyring, err := FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if keyring.ActiveKeyVersion() != 2 {
		t.Fatalf("unexpected active version: %d", keyring.ActiveKeyVersion())
	}
}
