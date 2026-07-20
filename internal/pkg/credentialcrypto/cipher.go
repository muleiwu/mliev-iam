package credentialcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	EnvironmentKey = "IDENTITY_CREDENTIALS_KEY"
	Algorithm      = "AES-256-GCM"
	KeyVersion     = 1
)

var (
	ErrMissingKey = errors.New("IDENTITY_CREDENTIALS_KEY 未配置")
	ErrInvalidKey = errors.New("IDENTITY_CREDENTIALS_KEY 必须是 Base64 编码的 32 字节密钥")
)

type Envelope struct {
	Ciphertext string
	Nonce      string
	Algorithm  string
	KeyVersion int
}

type Cipher struct {
	aead cipher.AEAD
}

func FromEnvironment() (*Cipher, error) {
	encoded := os.Getenv(EnvironmentKey)
	if encoded == "" {
		return nil, ErrMissingKey
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return New(key)
}

func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES cipher 失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM cipher 失败: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func AAD(connectionID uint, purpose string, keyVersion int) []byte {
	return []byte(fmt.Sprintf("%d:%s:%d", connectionID, purpose, keyVersion))
}

func (c *Cipher) Encrypt(plaintext, aad []byte) (*Envelope, error) {
	if c == nil || c.aead == nil {
		return nil, errors.New("凭据 cipher 未初始化")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成凭据 nonce 失败: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, plaintext, aad)
	return &Envelope{
		Ciphertext: base64.StdEncoding.EncodeToString(sealed),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Algorithm:  Algorithm,
		KeyVersion: KeyVersion,
	}, nil
}

func (c *Cipher) Decrypt(envelope Envelope, aad []byte) ([]byte, error) {
	if c == nil || c.aead == nil {
		return nil, errors.New("凭据 cipher 未初始化")
	}
	if envelope.Algorithm != Algorithm || envelope.KeyVersion <= 0 {
		return nil, fmt.Errorf("不支持的凭据加密格式: %s/v%d", envelope.Algorithm, envelope.KeyVersion)
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != c.aead.NonceSize() {
		return nil, errors.New("凭据 nonce 无效")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, errors.New("凭据密文无效")
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("凭据解密失败：密钥、用途或数据不匹配")
	}
	return plaintext, nil
}
