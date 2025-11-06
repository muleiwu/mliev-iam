package jwt

import (
	"fmt"
	"strings"
)

// KeyManagerFactory 密钥管理器工厂
type KeyManagerFactory struct{}

// NewKeyManagerFactory 创建密钥管理器工厂
func NewKeyManagerFactory() *KeyManagerFactory {
	return &KeyManagerFactory{}
}

// CreateKeyManager 根据算法创建密钥管理器
func (f *KeyManagerFactory) CreateKeyManager(algorithm, keyID string, keySize int) (KeyManager, error) {
	algorithm = strings.ToUpper(algorithm)

	switch {
	case strings.HasPrefix(algorithm, "RS"): // RS256, RS384, RS512
		return f.createRSAKeyManager(algorithm, keyID, keySize)
	case strings.HasPrefix(algorithm, "HS"): // HS256, HS384, HS512
		return f.createHMACKeyManager(algorithm, keyID, keySize)
	case strings.HasPrefix(algorithm, "ES"): // ES256, ES384, ES512 (预留)
		return nil, fmt.Errorf("%w: ECDSA算法尚未实现", ErrInvalidAlgorithm)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidAlgorithm, algorithm)
	}
}

// createRSAKeyManager 创建RSA密钥管理器
func (f *KeyManagerFactory) createRSAKeyManager(algorithm, keyID string, keySize int) (KeyManager, error) {
	// 验证RSA算法
	validAlgorithms := map[string]bool{
		"RS256": true,
		"RS384": true,
		"RS512": true,
	}

	if !validAlgorithms[algorithm] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidAlgorithm, algorithm)
	}

	// 设置默认密钥大小
	if keySize == 0 {
		switch algorithm {
		case "RS256", "RS384":
			keySize = 2048
		case "RS512":
			keySize = 4096
		}
	}

	// 验证密钥大小
	if keySize < 2048 {
		keySize = 2048
	}

	return NewRSAKeyManagerWithAlgorithm(keyID, algorithm, keySize), nil
}

// createHMACKeyManager 创建HMAC密钥管理器
func (f *KeyManagerFactory) createHMACKeyManager(algorithm, keyID string, keySize int) (KeyManager, error) {
	// 验证HMAC算法
	validAlgorithms := map[string]bool{
		"HS256": true,
		"HS384": true,
		"HS512": true,
	}

	if !validAlgorithms[algorithm] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidAlgorithm, algorithm)
	}

	// 设置默认密钥大小
	if keySize == 0 {
		switch algorithm {
		case "HS256":
			keySize = 256
		case "HS384":
			keySize = 384
		case "HS512":
			keySize = 512
		}
	}

	// 验证密钥大小
	if keySize < 256 {
		keySize = 256
	}

	return NewHMACKeyManagerWithAlgorithm(keyID, algorithm, keySize), nil
}

// LoadKeyManager 从元数据加载密钥管理器
func (f *KeyManagerFactory) LoadKeyManager(metadata *KeyManagerMetadata) (KeyManager, error) {
	// 创建密钥管理器
	km, err := f.CreateKeyManager(metadata.Algorithm, metadata.KeyID, metadata.KeySize)
	if err != nil {
		return nil, err
	}

	// 加载私钥
	if err := km.LoadPrivateKey(metadata.PrivateKey); err != nil {
		return nil, fmt.Errorf("加载私钥失败: %w", err)
	}

	// 如果有公钥，加载公钥
	if metadata.PublicKey != "" {
		if err := km.LoadPublicKey(metadata.PublicKey); err != nil {
			// 公钥加载失败不是致命错误（某些实现可以从私钥派生）
			// 忽略错误
		}
	}

	return km, nil
}

// SupportedAlgorithms 返回支持的算法列表
func (f *KeyManagerFactory) SupportedAlgorithms() []string {
	return []string{
		"RS256", "RS384", "RS512",
		"HS256", "HS384", "HS512",
		// "ES256", "ES384", "ES512", // 预留
	}
}

// IsAlgorithmSupported 检查算法是否支持
func (f *KeyManagerFactory) IsAlgorithmSupported(algorithm string) bool {
	algorithm = strings.ToUpper(algorithm)
	for _, supported := range f.SupportedAlgorithms() {
		if algorithm == supported {
			return true
		}
	}
	return false
}
