package jwt

import (
	"os"
	"strconv"
	"time"
)

// Config JWT 运行时配置（原 internal/pkg/config.JWTConfig）。
// 来自环境变量：JWT_SECRET_KEY / JWT_ISSUER / JWT_ACCESS_TOKEN_TTL / JWT_REFRESH_TOKEN_TTL。
type Config struct {
	SecretKey       string
	Issuer          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// GetConfig 读取环境变量构造一份 JWT Config。
func GetConfig() *Config {
	secretKey := os.Getenv("JWT_SECRET_KEY")
	if secretKey == "" {
		secretKey = "your-secret-key-change-this-in-production-please-use-a-strong-random-key"
	}

	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = "mliev-iam"
	}

	accessTokenTTL := time.Hour
	if ttlStr := os.Getenv("JWT_ACCESS_TOKEN_TTL"); ttlStr != "" {
		if hours, err := strconv.Atoi(ttlStr); err == nil {
			accessTokenTTL = time.Duration(hours) * time.Hour
		}
	}

	refreshTokenTTL := 30 * 24 * time.Hour
	if ttlStr := os.Getenv("JWT_REFRESH_TOKEN_TTL"); ttlStr != "" {
		if days, err := strconv.Atoi(ttlStr); err == nil {
			refreshTokenTTL = time.Duration(days) * 24 * time.Hour
		}
	}

	return &Config{
		SecretKey:       secretKey,
		Issuer:          issuer,
		AccessTokenTTL:  accessTokenTTL,
		RefreshTokenTTL: refreshTokenTTL,
	}
}

// GetAccessTokenHours 访问令牌小时数
func (c *Config) GetAccessTokenHours() int { return int(c.AccessTokenTTL.Hours()) }

// GetRefreshTokenDays 刷新令牌天数
func (c *Config) GetRefreshTokenDays() int { return int(c.RefreshTokenTTL.Hours() / 24) }
