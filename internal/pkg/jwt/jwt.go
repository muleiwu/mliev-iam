package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenSubject 是 JWTService.GenerateToken 所需的最小用户凭据。
// 原先直接依赖 app/dto.UserResponse，解耦后改为本包独立的轻量类型，
// 便于未来将 jwt 包抽取为独立 SDK。
type TokenSubject struct {
	UserID   uint
	Username string
	Role     string // 必填，不得为空（GenerateToken 会拒绝空 role，防止静默提权）
}

// Claims JWT声明结构
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// JWTService JWT服务
type JWTService struct {
	secretKey []byte
	issuer    string
}

// NewJWTService 创建JWT服务实例
func NewJWTService(secretKey, issuer string) *JWTService {
	return &JWTService{
		secretKey: []byte(secretKey),
		issuer:    issuer,
	}
}

// GenerateToken 生成JWT token
func (j *JWTService) GenerateToken(sub TokenSubject, expirationHours int) (string, error) {
	// 设置过期时间
	expirationTime := time.Now().Add(time.Duration(expirationHours) * time.Hour)

	// role 必须显式指定：早期实现空 role 会回退为 "admin"，造成任何漏填 role 的
	// 调用方静默拿到管理员身份的提权漏洞，故改为拒绝空 role。
	if sub.Role == "" {
		return "", errors.New("token role 不能为空")
	}

	// 创建声明
	claims := &Claims{
		UserID:   sub.UserID,
		Username: sub.Username,
		Role:     sub.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    j.issuer,
			Subject:   sub.Username,
		},
	}

	// 创建token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 签名并返回token字符串
	tokenString, err := token.SignedString(j.secretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken 验证JWT token
func (j *JWTService) ValidateToken(tokenString string) (*Claims, error) {
	// 解析token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("无效的签名方法")
		}
		return j.secretKey, nil
	})

	if err != nil {
		return nil, err
	}

	// 检查token是否有效
	if !token.Valid {
		return nil, errors.New("无效的token")
	}

	// 获取claims
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("无法解析token声明")
	}

	return claims, nil
}

// RefreshToken 刷新JWT token
func (j *JWTService) RefreshToken(tokenString string, expirationHours int) (string, error) {
	// 验证旧token（即使过期也要能解析出用户信息）
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("无效的签名方法")
		}
		return j.secretKey, nil
	})

	// 检查错误类型
	var claims *Claims
	if err != nil {
		// 检查是否为过期错误
		if errors.Is(err, jwt.ErrTokenExpired) {
			// Token过期，但我们仍然可以解析出用户信息进行刷新
			if token != nil {
				if c, ok := token.Claims.(*Claims); ok {
					claims = c
				} else {
					return "", errors.New("无法解析token声明")
				}
			} else {
				return "", errors.New("无法解析过期的token")
			}
		} else {
			return "", err
		}
	} else {
		// Token未过期
		if c, ok := token.Claims.(*Claims); ok {
			claims = c
		} else {
			return "", errors.New("无法解析token声明")
		}
	}

	// 检查token是否在刷新时间窗口内（例如：过期后30天内仍可刷新）
	if time.Since(claims.ExpiresAt.Time) > 30*24*time.Hour {
		return "", errors.New("token过期时间过长，无法刷新")
	}

	// 创建新的声明
	newClaims := &Claims{
		UserID:   claims.UserID,
		Username: claims.Username,
		Role:     claims.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expirationHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    j.issuer,
			Subject:   claims.Subject,
		},
	}

	// 生成新token
	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	return newToken.SignedString(j.secretKey)
}
