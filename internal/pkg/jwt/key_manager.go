package jwt

import (
	"errors"

	jwtLib "github.com/golang-jwt/jwt/v5"
)

// KeyType 密钥类型枚举
type KeyType string

const (
	KeyTypeRSA   KeyType = "RSA"
	KeyTypeHMAC  KeyType = "HMAC"
	KeyTypeECDSA KeyType = "ECDSA"
)

// Common errors
var (
	ErrKeyNotInitialized   = errors.New("密钥未初始化")
	ErrInvalidKeyType      = errors.New("无效的密钥类型")
	ErrInvalidAlgorithm    = errors.New("无效的签名算法")
	ErrKeyGenerationFailed = errors.New("密钥生成失败")
	ErrKeyExportFailed     = errors.New("密钥导出失败")
	ErrKeyLoadFailed       = errors.New("密钥加载失败")
)

// KeyManager 密钥管理器接口 - 支持多种密钥类型的统一抽象
type KeyManager interface {
	// GenerateKey 生成新的密钥对/密钥
	GenerateKey() error

	// GetKeyID 获取密钥ID（kid，用于JWT header）
	GetKeyID() string

	// GetAlgorithm 获取签名算法（如RS256, HS256等）
	GetAlgorithm() string

	// GetKeyType 获取密钥类型（RSA, HMAC, ECDSA等）
	GetKeyType() KeyType

	// GetKeySize 获取密钥长度
	GetKeySize() int

	// ExportPrivateKey 导出私钥（PEM格式或base64编码）
	ExportPrivateKey() (string, error)

	// ExportPublicKey 导出公钥（PEM格式，对称加密返回空字符串）
	ExportPublicKey() (string, error)

	// LoadPrivateKey 从字符串加载私钥
	LoadPrivateKey(data string) error

	// LoadPublicKey 从字符串加载公钥（可选，某些实现可能从私钥派生）
	LoadPublicKey(data string) error

	// SignToken 使用密钥签名JWT token
	SignToken(claims jwtLib.Claims) (string, error)

	// VerifyToken 使用密钥验证JWT token
	VerifyToken(tokenString string) (*jwtLib.Token, error)

	// GetJWK 获取JWK格式的公钥（用于JWKS端点，对称加密返回nil）
	GetJWK() (map[string]interface{}, error)

	// IsInitialized 检查密钥是否已初始化
	IsInitialized() bool
}

// KeyManagerMetadata 密钥管理器元数据（用于数据库存储）
type KeyManagerMetadata struct {
	KeyID      string
	Algorithm  string
	KeyType    KeyType
	KeySize    int
	PrivateKey string
	PublicKey  string
}

// ExportMetadata 导出密钥管理器的元数据（用于数据库存储）
func ExportMetadata(km KeyManager) (*KeyManagerMetadata, error) {
	privateKey, err := km.ExportPrivateKey()
	if err != nil {
		return nil, err
	}

	publicKey, _ := km.ExportPublicKey() // 对称加密可能返回空

	return &KeyManagerMetadata{
		KeyID:      km.GetKeyID(),
		Algorithm:  km.GetAlgorithm(),
		KeyType:    km.GetKeyType(),
		KeySize:    km.GetKeySize(),
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}, nil
}
