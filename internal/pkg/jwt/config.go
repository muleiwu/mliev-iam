package jwt

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const minJWTSecretLen = 32

// Config JWT 运行时配置（原 internal/pkg/config.JWTConfig）。
// 来自环境变量：JWT_SECRET_KEY / JWT_ISSUER / JWT_ACCESS_TOKEN_TTL / JWT_REFRESH_TOKEN_TTL。
type Config struct {
	SecretKey       string
	Issuer          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// GetConfig 读取环境变量构造一份 JWT Config。
// 启动期 fail-fast：JWT_SECRET_KEY 必须通过环境变量提供，且长度 >= 32 字节，
// 禁止使用历史占位字符串；否则 panic 阻止进程以弱密钥启动。
func GetConfig() *Config {
	secretKey := os.Getenv("JWT_SECRET_KEY")
	if secretKey == "" {
		panic("JWT_SECRET_KEY 未设置：请通过环境变量注入强随机密钥（建议：openssl rand -base64 48）")
	}
	if strings.Contains(secretKey, "your-secret-key-change-this") {
		panic("JWT_SECRET_KEY 仍为占位字符串：请替换为强随机密钥（建议：openssl rand -base64 48）")
	}
	if len(secretKey) < minJWTSecretLen {
		panic(fmt.Sprintf("JWT_SECRET_KEY 长度不足：当前 %d 字节，至少需要 %d 字节（建议：openssl rand -base64 48）", len(secretKey), minJWTSecretLen))
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
