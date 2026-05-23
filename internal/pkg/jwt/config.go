package jwt

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
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

// LoadConfig 读取环境变量或 config.yaml 构造一份 JWT Config。
// 环境变量优先：JWT_SECRET_KEY / JWT_ISSUER / JWT_ACCESS_TOKEN_TTL / JWT_REFRESH_TOKEN_TTL。
// 兼容配置文件：jwt.secret / jwt.issuer / jwt.expire_hours / jwt.refresh_expire_days。
func LoadConfig() (*Config, error) {
	secretKey := os.Getenv("JWT_SECRET_KEY")
	if secretKey == "" {
		secretKey = viper.GetString("jwt.secret")
	}
	if secretKey == "" {
		return nil, fmt.Errorf("JWT_SECRET_KEY 或 jwt.secret 未设置：请注入强随机密钥（建议：openssl rand -base64 48）")
	}
	if strings.Contains(secretKey, "your-secret-key-change-this") {
		return nil, fmt.Errorf("JWT_SECRET_KEY 或 jwt.secret 仍为占位字符串：请替换为强随机密钥（建议：openssl rand -base64 48）")
	}
	if strings.Contains(secretKey, "your-secret-key") {
		return nil, fmt.Errorf("JWT_SECRET_KEY 或 jwt.secret 仍为示例弱密钥：请替换为强随机密钥（建议：openssl rand -base64 48）")
	}
	if len(secretKey) < minJWTSecretLen {
		return nil, fmt.Errorf("JWT_SECRET_KEY 或 jwt.secret 长度不足：当前 %d 字节，至少需要 %d 字节（建议：openssl rand -base64 48）", len(secretKey), minJWTSecretLen)
	}

	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = viper.GetString("jwt.issuer")
		if issuer == "" {
			issuer = "mliev-iam"
		}
	}

	accessTokenTTL := time.Hour
	if ttlStr := os.Getenv("JWT_ACCESS_TOKEN_TTL"); ttlStr != "" {
		if hours, err := strconv.Atoi(ttlStr); err == nil {
			accessTokenTTL = time.Duration(hours) * time.Hour
		}
	} else if hours := viper.GetInt("jwt.expire_hours"); hours > 0 {
		accessTokenTTL = time.Duration(hours) * time.Hour
	}

	refreshTokenTTL := 30 * 24 * time.Hour
	if ttlStr := os.Getenv("JWT_REFRESH_TOKEN_TTL"); ttlStr != "" {
		if days, err := strconv.Atoi(ttlStr); err == nil {
			refreshTokenTTL = time.Duration(days) * 24 * time.Hour
		}
	} else if days := viper.GetInt("jwt.refresh_expire_days"); days > 0 {
		refreshTokenTTL = time.Duration(days) * 24 * time.Hour
	}

	return &Config{
		SecretKey:       secretKey,
		Issuer:          issuer,
		AccessTokenTTL:  accessTokenTTL,
		RefreshTokenTTL: refreshTokenTTL,
	}, nil
}

// GetConfig 读取 JWT Config；保留历史 Must 语义给启动期 fail-fast 场景使用。
func GetConfig() *Config {
	cfg, err := LoadConfig()
	if err != nil {
		panic(err)
	}
	return cfg
}

// GetAccessTokenHours 访问令牌小时数
func (c *Config) GetAccessTokenHours() int { return int(c.AccessTokenTTL.Hours()) }

// GetRefreshTokenDays 刷新令牌天数
func (c *Config) GetRefreshTokenDays() int { return int(c.RefreshTokenTTL.Hours() / 24) }
