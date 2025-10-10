package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sync"

	"github.com/golang-jwt/jwt/v5"
)

// RSAKeyManager RSA密钥管理器
type RSAKeyManager struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
	mu         sync.RWMutex
}

// NewRSAKeyManager 创建RSA密钥管理器
func NewRSAKeyManager(keyID string) *RSAKeyManager {
	return &RSAKeyManager{
		keyID: keyID,
	}
}

// GenerateKey 生成新的RSA密钥对
func (m *RSAKeyManager) GenerateKey(bits int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if bits < 2048 {
		bits = 2048 // 最小2048位
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return fmt.Errorf("生成RSA密钥失败: %w", err)
	}

	m.privateKey = privateKey
	m.publicKey = &privateKey.PublicKey

	return nil
}

// LoadPrivateKeyFromFile 从文件加载私钥
func (m *RSAKeyManager) LoadPrivateKeyFromFile(filepath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pemData, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("读取私钥文件失败: %w", err)
	}

	return m.loadPrivateKeyFromPEM(pemData)
}

// LoadPrivateKeyFromPEM 从PEM格式加载私钥
func (m *RSAKeyManager) LoadPrivateKeyFromPEM(pemData string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.loadPrivateKeyFromPEM([]byte(pemData))
}

// loadPrivateKeyFromPEM 内部方法，从PEM格式加载私钥（不加锁）
func (m *RSAKeyManager) loadPrivateKeyFromPEM(pemData []byte) error {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return errors.New("无效的PEM格式")
	}

	var privateKey *rsa.PrivateKey
	var err error

	// 尝试解析PKCS1格式
	if block.Type == "RSA PRIVATE KEY" {
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("解析PKCS1私钥失败: %w", err)
		}
	} else if block.Type == "PRIVATE KEY" {
		// 尝试解析PKCS8格式
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("解析PKCS8私钥失败: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return errors.New("私钥不是RSA类型")
		}
	} else {
		return fmt.Errorf("不支持的私钥类型: %s", block.Type)
	}

	m.privateKey = privateKey
	m.publicKey = &privateKey.PublicKey

	return nil
}

// LoadPublicKeyFromFile 从文件加载公钥
func (m *RSAKeyManager) LoadPublicKeyFromFile(filepath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pemData, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("读取公钥文件失败: %w", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		return errors.New("无效的PEM格式")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("解析公钥失败: %w", err)
	}

	rsaPublicKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("公钥不是RSA类型")
	}

	m.publicKey = rsaPublicKey

	return nil
}

// ExportPrivateKeyToPEM 导出私钥为PEM格式
func (m *RSAKeyManager) ExportPrivateKeyToPEM() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.privateKey == nil {
		return "", errors.New("私钥未初始化")
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(m.privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return string(privateKeyPEM), nil
}

// ExportPublicKeyToPEM 导出公钥为PEM格式
func (m *RSAKeyManager) ExportPublicKeyToPEM() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.publicKey == nil {
		return "", errors.New("公钥未初始化")
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(m.publicKey)
	if err != nil {
		return "", fmt.Errorf("序列化公钥失败: %w", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM), nil
}

// SavePrivateKeyToFile 保存私钥到文件
func (m *RSAKeyManager) SavePrivateKeyToFile(filepath string) error {
	pemData, err := m.ExportPrivateKeyToPEM()
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, []byte(pemData), 0600)
}

// SavePublicKeyToFile 保存公钥到文件
func (m *RSAKeyManager) SavePublicKeyToFile(filepath string) error {
	pemData, err := m.ExportPublicKeyToPEM()
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, []byte(pemData), 0644)
}

// GetPrivateKey 获取私钥
func (m *RSAKeyManager) GetPrivateKey() *rsa.PrivateKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.privateKey
}

// GetPublicKey 获取公钥
func (m *RSAKeyManager) GetPublicKey() *rsa.PublicKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.publicKey
}

// GetKeyID 获取密钥ID
func (m *RSAKeyManager) GetKeyID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.keyID
}

// GetJWK 获取JWK格式的公钥
func (m *RSAKeyManager) GetJWK() (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.publicKey == nil {
		return nil, errors.New("公钥未初始化")
	}

	// 将N和E转换为base64url编码
	nBytes := m.publicKey.N.Bytes()
	eBytes := big.NewInt(int64(m.publicKey.E)).Bytes()

	n := base64.RawURLEncoding.EncodeToString(nBytes)
	e := base64.RawURLEncoding.EncodeToString(eBytes)

	jwk := map[string]interface{}{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": m.keyID,
		"n":   n,
		"e":   e,
	}

	return jwk, nil
}

// SignToken 使用私钥签名JWT token
func (m *RSAKeyManager) SignToken(claims jwt.Claims) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.privateKey == nil {
		return "", errors.New("私钥未初始化")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = m.keyID

	tokenString, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("签名token失败: %w", err)
	}

	return tokenString, nil
}

// VerifyToken 使用公钥验证JWT token
func (m *RSAKeyManager) VerifyToken(tokenString string) (*jwt.Token, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.publicKey == nil {
		return nil, errors.New("公钥未初始化")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("意外的签名方法: %v", token.Header["alg"])
		}
		return m.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("验证token失败: %w", err)
	}

	return token, nil
}

// IsInitialized 检查密钥是否已初始化
func (m *RSAKeyManager) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.privateKey != nil && m.publicKey != nil
}
