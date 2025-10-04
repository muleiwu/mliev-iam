package config

import (
	"os"
	"strconv"
	"time"
)

// JWTConfig JWT配置
type JWTConfig struct {
	SecretKey       string        // JWT签名密钥
	Issuer          string        // 发行者
	AccessTokenTTL  time.Duration // 访问令牌有效期
	RefreshTokenTTL time.Duration // 刷新令牌有效期
}

// GetJWTConfig 获取JWT配置
func GetJWTConfig() *JWTConfig {
	secretKey := os.Getenv("JWT_SECRET_KEY")
	if secretKey == "" {
		// 生产环境中应该抛出错误或使用更安全的默认值
		secretKey = "your-secret-key-change-this-in-production-please-use-a-strong-random-key"
	}

	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = "mliev-iam"
	}

	// 访问令牌有效期，默认1小时
	accessTokenTTL := 1 * time.Hour
	if ttlStr := os.Getenv("JWT_ACCESS_TOKEN_TTL"); ttlStr != "" {
		if hours, err := strconv.Atoi(ttlStr); err == nil {
			accessTokenTTL = time.Duration(hours) * time.Hour
		}
	}

	// 刷新令牌有效期，默认30天
	refreshTokenTTL := 30 * 24 * time.Hour
	if ttlStr := os.Getenv("JWT_REFRESH_TOKEN_TTL"); ttlStr != "" {
		if days, err := strconv.Atoi(ttlStr); err == nil {
			refreshTokenTTL = time.Duration(days) * 24 * time.Hour
		}
	}

	return &JWTConfig{
		SecretKey:       secretKey,
		Issuer:          issuer,
		AccessTokenTTL:  accessTokenTTL,
		RefreshTokenTTL: refreshTokenTTL,
	}
}

// GetAccessTokenHours 获取访问令牌小时数
func (c *JWTConfig) GetAccessTokenHours() int {
	return int(c.AccessTokenTTL.Hours())
}

// GetRefreshTokenDays 获取刷新令牌天数
func (c *JWTConfig) GetRefreshTokenDays() int {
	return int(c.RefreshTokenTTL.Hours() / 24)
}
