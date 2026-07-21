package credentialcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	EnvironmentKey              = "IDENTITY_CREDENTIALS_KEY"
	EnvironmentKeys             = "IDENTITY_CREDENTIALS_KEYS"
	EnvironmentActiveKeyVersion = "IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION"
	Algorithm                   = "AES-256-GCM"
	KeyVersion                  = 1
)

var (
	ErrMissingKey = errors.New("IDENTITY_CREDENTIALS_KEYS 或 IDENTITY_CREDENTIALS_KEY 未配置")
	ErrInvalidKey = errors.New("身份凭据密钥必须是 Base64 编码的 32 字节密钥")
)

type Envelope struct {
	Ciphertext string
	Nonce      string
	Algorithm  string
	KeyVersion int
}

type Cipher struct {
	aeads         map[int]cipher.AEAD
	activeVersion int
}

func FromEnvironment() (*Cipher, error) {
	encodedKeys := strings.TrimSpace(os.Getenv(EnvironmentKeys))
	if encodedKeys == "" {
		encoded := strings.TrimSpace(os.Getenv(EnvironmentKey))
		if encoded == "" {
			return nil, ErrMissingKey
		}
		return newKeyring(map[int]string{KeyVersion: encoded}, KeyVersion)
	}

	keys := make(map[int]string)
	if err := json.Unmarshal([]byte(encodedKeys), &keys); err != nil {
		// 兼容便于容器 secret 注入的 `1:base64,2:base64` 形式。
		for _, item := range strings.Split(encodedKeys, ",") {
			parts := strings.SplitN(strings.TrimSpace(item), ":", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("%s 必须是 JSON version->key 映射或 version:key 列表", EnvironmentKeys)
			}
			version, parseErr := strconv.Atoi(strings.TrimSpace(parts[0]))
			if parseErr != nil || version <= 0 {
				return nil, fmt.Errorf("非法凭据密钥版本 %q", parts[0])
			}
			keys[version] = strings.TrimSpace(parts[1])
		}
	}

	activeText := strings.TrimSpace(os.Getenv(EnvironmentActiveKeyVersion))
	if activeText == "" {
		return nil, fmt.Errorf("配置 %s 时必须同时配置 %s", EnvironmentKeys, EnvironmentActiveKeyVersion)
	}
	activeVersion, err := strconv.Atoi(activeText)
	if err != nil || activeVersion <= 0 {
		return nil, fmt.Errorf("%s 必须是正整数", EnvironmentActiveKeyVersion)
	}
	return newKeyring(keys, activeVersion)
}

func New(key []byte) (*Cipher, error) {
	return NewKeyring(map[int][]byte{KeyVersion: key}, KeyVersion)
}

func NewKeyring(keys map[int][]byte, activeVersion int) (*Cipher, error) {
	if activeVersion <= 0 {
		return nil, errors.New("active key version 必须是正整数")
	}
	aeads := make(map[int]cipher.AEAD, len(keys))
	for version, key := range keys {
		if version <= 0 || len(key) != 32 {
			return nil, ErrInvalidKey
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("创建 AES cipher v%d 失败: %w", version, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("创建 GCM cipher v%d 失败: %w", version, err)
		}
		aeads[version] = aead
	}
	if _, ok := aeads[activeVersion]; !ok {
		return nil, fmt.Errorf("active key version %d 不在 keyring 中", activeVersion)
	}
	return &Cipher{aeads: aeads, activeVersion: activeVersion}, nil
}

func newKeyring(encoded map[int]string, activeVersion int) (*Cipher, error) {
	keys := make(map[int][]byte, len(encoded))
	for version, value := range encoded {
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("凭据密钥 v%d 无效: %w", version, ErrInvalidKey)
		}
		keys[version] = key
	}
	return NewKeyring(keys, activeVersion)
}

func (c *Cipher) ActiveKeyVersion() int {
	if c == nil {
		return 0
	}
	return c.activeVersion
}

func AAD(connectionID uint, purpose string, keyVersion int) []byte {
	return []byte(fmt.Sprintf("%d:%s:%d", connectionID, purpose, keyVersion))
}

func (c *Cipher) Encrypt(plaintext, aad []byte) (*Envelope, error) {
	if c == nil {
		return nil, errors.New("凭据 cipher 未初始化")
	}
	aead := c.aeads[c.activeVersion]
	if aead == nil {
		return nil, errors.New("active 凭据 cipher 未初始化")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成凭据 nonce 失败: %w", err)
	}
	sealed := aead.Seal(nil, nonce, plaintext, aad)
	return &Envelope{
		Ciphertext: base64.StdEncoding.EncodeToString(sealed),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Algorithm:  Algorithm,
		KeyVersion: c.activeVersion,
	}, nil
}

func (c *Cipher) Decrypt(envelope Envelope, aad []byte) ([]byte, error) {
	if c == nil {
		return nil, errors.New("凭据 cipher 未初始化")
	}
	if envelope.Algorithm != Algorithm || envelope.KeyVersion <= 0 {
		return nil, fmt.Errorf("不支持的凭据加密格式: %s/v%d", envelope.Algorithm, envelope.KeyVersion)
	}
	aead := c.aeads[envelope.KeyVersion]
	if aead == nil {
		return nil, fmt.Errorf("凭据密钥版本 v%d 未配置", envelope.KeyVersion)
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != aead.NonceSize() {
		return nil, errors.New("凭据 nonce 无效")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, errors.New("凭据密文无效")
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("凭据解密失败：密钥、用途或数据不匹配")
	}
	return plaintext, nil
}
