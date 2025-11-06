package jwt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	jwtLib "github.com/golang-jwt/jwt/v5"
)

// HMACKeyManager HMAC密钥管理器
type HMACKeyManager struct {
	secret    []byte
	keyID     string
	algorithm string // HS256, HS512等
	keySize   int    // 密钥长度（位）
	mu        sync.RWMutex
}

// NewHMACKeyManager 创建HMAC密钥管理器（默认HS256）
func NewHMACKeyManager(keyID string) *HMACKeyManager {
	return &HMACKeyManager{
		keyID:     keyID,
		algorithm: "HS256",
		keySize:   256,
	}
}

// NewHMACKeyManagerWithAlgorithm 创建指定算法的HMAC密钥管理器
func NewHMACKeyManagerWithAlgorithm(keyID, algorithm string, keySize int) *HMACKeyManager {
	if keySize < 256 {
		keySize = 256 // 最小256位
	}
	return &HMACKeyManager{
		keyID:     keyID,
		algorithm: algorithm,
		keySize:   keySize,
	}
}

// GenerateKey 生成新的HMAC密钥
func (m *HMACKeyManager) GenerateKey() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 生成随机密钥
	keyBytes := m.keySize / 8 // 转换为字节数
	secret := make([]byte, keyBytes)
	_, err := rand.Read(secret)
	if err != nil {
		return fmt.Errorf("生成HMAC密钥失败: %w", err)
	}

	m.secret = secret
	return nil
}

// GetKeyID 获取密钥ID
func (m *HMACKeyManager) GetKeyID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.keyID
}

// GetAlgorithm 获取签名算法
func (m *HMACKeyManager) GetAlgorithm() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.algorithm
}

// GetKeyType 获取密钥类型
func (m *HMACKeyManager) GetKeyType() KeyType {
	return KeyTypeHMAC
}

// GetKeySize 获取密钥长度
func (m *HMACKeyManager) GetKeySize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.keySize
}

// ExportPrivateKey 导出密钥（base64编码）
func (m *HMACKeyManager) ExportPrivateKey() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.secret == nil {
		return "", ErrKeyNotInitialized
	}

	return base64.StdEncoding.EncodeToString(m.secret), nil
}

// ExportPublicKey 导出公钥（HMAC无公钥，返回空）
func (m *HMACKeyManager) ExportPublicKey() (string, error) {
	return "", nil
}

// LoadPrivateKey 加载密钥（从base64字符串）
func (m *HMACKeyManager) LoadPrivateKey(data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	secret, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Errorf("解码HMAC密钥失败: %w", err)
	}

	if len(secret) < 32 {
		return errors.New("HMAC密钥长度不足（最小256位）")
	}

	m.secret = secret
	m.keySize = len(secret) * 8

	return nil
}

// LoadPublicKey 加载公钥（HMAC无公钥，空实现）
func (m *HMACKeyManager) LoadPublicKey(data string) error {
	// HMAC对称加密，无公钥概念
	return nil
}

// SignToken 使用HMAC签名JWT token
func (m *HMACKeyManager) SignToken(claims jwtLib.Claims) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.secret == nil {
		return "", ErrKeyNotInitialized
	}

	// 根据算法选择签名方法
	var method jwtLib.SigningMethod
	switch m.algorithm {
	case "HS256":
		method = jwtLib.SigningMethodHS256
	case "HS384":
		method = jwtLib.SigningMethodHS384
	case "HS512":
		method = jwtLib.SigningMethodHS512
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidAlgorithm, m.algorithm)
	}

	token := jwtLib.NewWithClaims(method, claims)
	token.Header["kid"] = m.keyID

	tokenString, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("签名token失败: %w", err)
	}

	return tokenString, nil
}

// VerifyToken 使用HMAC验证JWT token
func (m *HMACKeyManager) VerifyToken(tokenString string) (*jwtLib.Token, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.secret == nil {
		return nil, ErrKeyNotInitialized
	}

	token, err := jwtLib.Parse(tokenString, func(token *jwtLib.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwtLib.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("意外的签名方法: %v", token.Header["alg"])
		}
		return m.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("验证token失败: %w", err)
	}

	return token, nil
}

// GetJWK 获取JWK格式的公钥（HMAC对称加密不暴露密钥，返回nil）
func (m *HMACKeyManager) GetJWK() (map[string]interface{}, error) {
	// HMAC对称加密不应该暴露密钥
	return nil, nil
}

// IsInitialized 检查密钥是否已初始化
func (m *HMACKeyManager) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.secret) >= 32
}

// GetSecret 获取密钥（仅供内部使用）
func (m *HMACKeyManager) GetSecret() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.secret
}
