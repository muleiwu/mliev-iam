package service

import (
	"fmt"
	"strconv"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	jwtLib "github.com/golang-jwt/jwt/v5"
)

// IDTokenClaims ID Token的JWT声明
type IDTokenClaims struct {
	// 标准OIDC Claims (必需)
	Sub string `json:"sub"` // Subject - 用户唯一标识符
	Iss string `json:"iss"` // Issuer - 颁发者
	Aud string `json:"aud"` // Audience - 接收者(客户端ID)
	Exp int64  `json:"exp"` // Expiration Time - 过期时间
	Iat int64  `json:"iat"` // Issued At - 颁发时间

	// 可选的标准Claims
	Nonce    string `json:"nonce,omitempty"`     // 用于防重放攻击
	AuthTime int64  `json:"auth_time,omitempty"` // 用户认证时间
	Azp      string `json:"azp,omitempty"`       // Authorized party - 授权方

	// Profile scope claims
	Name              string `json:"name,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`
	MiddleName        string `json:"middle_name,omitempty"`
	Nickname          string `json:"nickname,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Profile           string `json:"profile,omitempty"`
	Picture           string `json:"picture,omitempty"`
	Website           string `json:"website,omitempty"`
	Gender            string `json:"gender,omitempty"`
	Birthdate         string `json:"birthdate,omitempty"`
	Zoneinfo          string `json:"zoneinfo,omitempty"`
	Locale            string `json:"locale,omitempty"`
	UpdatedAt         int64  `json:"updated_at,omitempty"`

	// Email scope claims
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`

	// Phone scope claims
	PhoneNumber         string `json:"phone_number,omitempty"`
	PhoneNumberVerified bool   `json:"phone_number_verified,omitempty"`

	jwtLib.RegisteredClaims
}

// OIDCIDTokenService ID Token生成服务
type OIDCIDTokenService struct {
	algorithm     string             // 签名算法: "RS256" 或 "HS256"
	rsaKeyManager *jwt.RSAKeyManager // RS256 使用
	hmacSecret    []byte             // HS256 使用
	keyID         string             // 密钥ID
	issuer        string
	tokenLifetime int // 秒
}

// NewOIDCIDTokenServiceWithRSA 创建使用RSA签名的ID Token服务
func NewOIDCIDTokenServiceWithRSA(rsaKeyManager *jwt.RSAKeyManager, issuer string, tokenLifetime int) *OIDCIDTokenService {
	if tokenLifetime <= 0 {
		tokenLifetime = 3600 // 默认1小时
	}

	return &OIDCIDTokenService{
		algorithm:     "RS256",
		rsaKeyManager: rsaKeyManager,
		keyID:         rsaKeyManager.GetKeyID(),
		issuer:        issuer,
		tokenLifetime: tokenLifetime,
	}
}

// NewOIDCIDTokenServiceWithHMAC 创建使用HMAC签名的ID Token服务
func NewOIDCIDTokenServiceWithHMAC(hmacSecret []byte, keyID, issuer string, tokenLifetime int) *OIDCIDTokenService {
	if tokenLifetime <= 0 {
		tokenLifetime = 3600 // 默认1小时
	}

	return &OIDCIDTokenService{
		algorithm:     "HS256",
		hmacSecret:    hmacSecret,
		keyID:         keyID,
		issuer:        issuer,
		tokenLifetime: tokenLifetime,
	}
}

// NewOIDCIDTokenService 创建ID Token服务（向后兼容，默认使用RSA）
// 已弃用: 请使用 NewOIDCIDTokenServiceWithRSA 或 NewOIDCIDTokenServiceWithHMAC
func NewOIDCIDTokenService(rsaKeyManager *jwt.RSAKeyManager, issuer string, tokenLifetime int) *OIDCIDTokenService {
	return NewOIDCIDTokenServiceWithRSA(rsaKeyManager, issuer, tokenLifetime)
}

// GenerateIDToken 生成ID Token
func (s *OIDCIDTokenService) GenerateIDToken(user *model.User, clientID string, scopes []string, nonce string, authTime *time.Time) (string, error) {
	now := time.Now()
	exp := now.Add(time.Duration(s.tokenLifetime) * time.Second)

	// 构建基础claims
	claims := &IDTokenClaims{
		Sub: strconv.FormatUint(uint64(user.ID), 10),
		Iss: s.issuer,
		Aud: clientID,
		Exp: exp.Unix(),
		Iat: now.Unix(),
	}

	// 添加nonce（如果有）
	if nonce != "" {
		claims.Nonce = nonce
	}

	// 添加认证时间
	if authTime != nil {
		claims.AuthTime = authTime.Unix()
	} else {
		claims.AuthTime = now.Unix()
	}

	// 根据scopes添加用户信息
	if s.hasScope(scopes, "profile") {
		s.addProfileClaims(claims, user)
	}

	if s.hasScope(scopes, "email") {
		s.addEmailClaims(claims, user)
	}

	if s.hasScope(scopes, "phone") {
		s.addPhoneClaims(claims, user)
	}

	// 根据算法类型签名token
	var tokenString string
	var err error

	switch s.algorithm {
	case "RS256":
		if s.rsaKeyManager == nil || !s.rsaKeyManager.IsInitialized() {
			return "", fmt.Errorf("RS256算法需要初始化RSA密钥管理器")
		}
		tokenString, err = s.rsaKeyManager.SignToken(claims)
		if err != nil {
			return "", fmt.Errorf("使用RSA签名ID Token失败: %w", err)
		}

	case "HS256":
		if len(s.hmacSecret) == 0 {
			return "", fmt.Errorf("HS256算法需要HMAC密钥")
		}
		// 使用HMAC签名
		token := jwtLib.NewWithClaims(jwtLib.SigningMethodHS256, claims)
		token.Header["kid"] = s.keyID
		tokenString, err = token.SignedString(s.hmacSecret)
		if err != nil {
			return "", fmt.Errorf("使用HMAC签名ID Token失败: %w", err)
		}

	default:
		return "", fmt.Errorf("不支持的签名算法: %s", s.algorithm)
	}

	return tokenString, nil
}

// addProfileClaims 添加profile scope的claims
func (s *OIDCIDTokenService) addProfileClaims(claims *IDTokenClaims, user *model.User) {
	claims.Name = user.RealName
	claims.Nickname = user.Nickname
	claims.PreferredUsername = user.Username
	claims.Picture = user.Avatar
	claims.UpdatedAt = user.UpdatedAt.Unix()

	// 从扩展属性中获取其他profile信息
	if user.Attributes != nil {
		if givenName, ok := user.Attributes["given_name"].(string); ok {
			claims.GivenName = givenName
		}
		if familyName, ok := user.Attributes["family_name"].(string); ok {
			claims.FamilyName = familyName
		}
		if middleName, ok := user.Attributes["middle_name"].(string); ok {
			claims.MiddleName = middleName
		}
		if website, ok := user.Attributes["website"].(string); ok {
			claims.Website = website
		}
		if gender, ok := user.Attributes["gender"].(string); ok {
			claims.Gender = gender
		}
		if birthdate, ok := user.Attributes["birthdate"].(string); ok {
			claims.Birthdate = birthdate
		}
		if zoneinfo, ok := user.Attributes["zoneinfo"].(string); ok {
			claims.Zoneinfo = zoneinfo
		}
		if locale, ok := user.Attributes["locale"].(string); ok {
			claims.Locale = locale
		}
		if profile, ok := user.Attributes["profile"].(string); ok {
			claims.Profile = profile
		}
	}
}

// addEmailClaims 添加email scope的claims
func (s *OIDCIDTokenService) addEmailClaims(claims *IDTokenClaims, user *model.User) {
	claims.Email = user.Email
	claims.EmailVerified = user.IsVerified
}

// addPhoneClaims 添加phone scope的claims
func (s *OIDCIDTokenService) addPhoneClaims(claims *IDTokenClaims, user *model.User) {
	claims.PhoneNumber = user.Phone

	// 从扩展属性中获取手机号验证状态
	if user.Attributes != nil {
		if phoneVerified, ok := user.Attributes["phone_number_verified"].(bool); ok {
			claims.PhoneNumberVerified = phoneVerified
		}
	}
}

// hasScope 检查是否包含指定scope
func (s *OIDCIDTokenService) hasScope(scopes []string, targetScope string) bool {
	for _, scope := range scopes {
		if scope == targetScope {
			return true
		}
	}
	return false
}

// VerifyIDToken 验证ID Token
func (s *OIDCIDTokenService) VerifyIDToken(tokenString string) (*IDTokenClaims, error) {
	if !s.rsaKeyManager.IsInitialized() {
		return nil, fmt.Errorf("RSA密钥未初始化")
	}

	token, err := s.rsaKeyManager.VerifyToken(tokenString)
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("ID Token无效")
	}

	claims, ok := token.Claims.(jwtLib.MapClaims)
	if !ok {
		return nil, fmt.Errorf("无法解析ID Token claims")
	}

	// 这里可以进一步验证claims的内容
	// 例如验证issuer、audience等

	// 将MapClaims转换为IDTokenClaims
	idTokenClaims := &IDTokenClaims{}
	if sub, ok := claims["sub"].(string); ok {
		idTokenClaims.Sub = sub
	}
	if iss, ok := claims["iss"].(string); ok {
		idTokenClaims.Iss = iss
	}
	if aud, ok := claims["aud"].(string); ok {
		idTokenClaims.Aud = aud
	}
	// ... 可以继续转换其他字段

	return idTokenClaims, nil
}
