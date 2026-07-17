package service

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash"
	"net/url"
	"strings"
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

	// Hybrid/Implicit 流程校验字段（OpenID Connect Core §3.3.2.11, §3.3.2.10）
	AtHash string `json:"at_hash,omitempty"` // left-most half of hash(access_token)
	CHash  string `json:"c_hash,omitempty"`  // left-most half of hash(authorization_code)

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

// AccessTokenClaims Access Token的JWT声明 (符合 RFC 9068)
type AccessTokenClaims struct {
	// 标准JWT Claims (必需)
	Iss string `json:"iss"`           // Issuer - 颁发者
	Sub string `json:"sub"`           // Subject - 用户唯一标识符
	Aud string `json:"aud"`           // Audience - 接收者(客户端ID)
	Exp int64  `json:"exp"`           // Expiration Time - 过期时间
	Iat int64  `json:"iat"`           // Issued At - 颁发时间
	Jti string `json:"jti,omitempty"` // JWT ID - 唯一标识符

	// OAuth 2.0 Access Token 专用 Claims
	ClientID string `json:"client_id,omitempty"` // 客户端ID
	Scope    string `json:"scope,omitempty"`     // 授权范围（空格分隔）

	// 用户信息 Claims（可选，便于第三方系统解析）
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Email             string `json:"email,omitempty"`

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

// GenerateIDToken 生成 ID Token（不含 at_hash / c_hash，用于纯 id_token 场景）
func (s *OIDCIDTokenService) GenerateIDToken(user *model.User, clientID string, scopes []string, nonce string, authTime *time.Time) (string, error) {
	return s.GenerateIDTokenWithHashes(user, clientID, scopes, nonce, authTime, "", "")
}

// GenerateIDTokenWithHashes 生成 ID Token，并按需嵌入 at_hash / c_hash
// accessToken 非空时写入 at_hash；authorizationCode 非空时写入 c_hash。
// Hash 算法跟随签名算法：RS256/HS256 -> SHA-256；RS512/HS512 -> SHA-512。
func (s *OIDCIDTokenService) GenerateIDTokenWithHashes(
	user *model.User, clientID string, scopes []string, nonce string, authTime *time.Time,
	accessToken, authorizationCode string,
) (string, error) {
	now := time.Now()
	exp := now.Add(time.Duration(s.tokenLifetime) * time.Second)

	claims := &IDTokenClaims{
		Sub: user.UUID,
		Iss: s.issuer,
		Aud: clientID,
		Exp: exp.Unix(),
		Iat: now.Unix(),
	}
	if nonce != "" {
		claims.Nonce = nonce
	}
	if authTime != nil {
		claims.AuthTime = authTime.Unix()
	} else {
		claims.AuthTime = now.Unix()
	}

	if accessToken != "" {
		claims.AtHash = leftMostHash(accessToken, s.algorithm)
	}
	if authorizationCode != "" {
		claims.CHash = leftMostHash(authorizationCode, s.algorithm)
	}

	if s.hasScope(scopes, "profile") {
		s.addProfileClaims(claims, user)
	}
	if s.hasScope(scopes, "email") {
		s.addEmailClaims(claims, user)
	}
	if s.hasScope(scopes, "phone") {
		s.addPhoneClaims(claims, user)
	}

	return s.signToken(claims)
}

// leftMostHash 按 OIDC Core §3.1.3.6 计算 at_hash / c_hash
// 取 hash 的前一半字节，然后 base64url（不含 padding）编码
func leftMostHash(value, alg string) string {
	if value == "" {
		return ""
	}
	var h hash.Hash
	switch alg {
	case "RS512", "HS512":
		h = sha512.New()
	default:
		h = sha256.New()
	}
	h.Write([]byte(value))
	sum := h.Sum(nil)
	half := sum[:len(sum)/2]
	return base64.RawURLEncoding.EncodeToString(half)
}

// GenerateAccessToken 生成 JWT 格式的 Access Token (符合 RFC 9068)
func (s *OIDCIDTokenService) GenerateAccessToken(user *model.User, clientID string, scopes []string) (string, error) {
	now := time.Now()
	exp := now.Add(time.Duration(s.tokenLifetime) * time.Second)

	// 生成唯一标识符
	jti := generateUUID()

	// 构建 Access Token Claims - 使用 UUID 作为 sub claim
	claims := &AccessTokenClaims{
		Iss:      s.issuer,
		Sub:      user.UUID,
		Aud:      clientID,
		Exp:      exp.Unix(),
		Iat:      now.Unix(),
		Jti:      jti,
		ClientID: clientID,
		Scope:    joinScopes(scopes),
	}

	// 添加用户基本信息（便于第三方系统解析）
	claims.Name = user.RealName
	claims.PreferredUsername = user.Username
	if user.Email != nil {
		claims.Email = *user.Email
	}

	// 根据算法类型签名token
	return s.signToken(claims)
}

// GenerateClientCredentialsAccessToken 生成客户端凭据流程的 JWT Access Token
func (s *OIDCIDTokenService) GenerateClientCredentialsAccessToken(clientID string, scopes []string) (string, error) {
	now := time.Now()
	exp := now.Add(time.Duration(s.tokenLifetime) * time.Second)

	// 生成唯一标识符
	jti := generateUUID()

	// 客户端凭据流程：sub 为客户端ID
	claims := &AccessTokenClaims{
		Iss:      s.issuer,
		Sub:      clientID, // 客户端凭据流程，sub 为客户端ID
		Aud:      clientID,
		Exp:      exp.Unix(),
		Iat:      now.Unix(),
		Jti:      jti,
		ClientID: clientID,
		Scope:    joinScopes(scopes),
	}

	// 根据算法类型签名token
	return s.signToken(claims)
}

// signToken 使用配置的算法签名 token
func (s *OIDCIDTokenService) signToken(claims jwtLib.Claims) (string, error) {
	var tokenString string
	var err error

	switch s.algorithm {
	case "RS256":
		if s.rsaKeyManager == nil || !s.rsaKeyManager.IsInitialized() {
			return "", fmt.Errorf("RS256算法需要初始化RSA密钥管理器")
		}
		tokenString, err = s.rsaKeyManager.SignToken(claims)
		if err != nil {
			return "", fmt.Errorf("使用RSA签名Token失败: %w", err)
		}

	case "HS256":
		if len(s.hmacSecret) == 0 {
			return "", fmt.Errorf("HS256算法需要HMAC密钥")
		}
		token := jwtLib.NewWithClaims(jwtLib.SigningMethodHS256, claims)
		token.Header["kid"] = s.keyID
		tokenString, err = token.SignedString(s.hmacSecret)
		if err != nil {
			return "", fmt.Errorf("使用HMAC签名Token失败: %w", err)
		}

	default:
		return "", fmt.Errorf("不支持的签名算法: %s", s.algorithm)
	}

	return tokenString, nil
}

// generateUUID 生成UUID
func generateUUID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// joinScopes 将 scopes 数组连接为空格分隔的字符串
func joinScopes(scopes []string) string {
	result := ""
	for i, scope := range scopes {
		if i > 0 {
			result += " "
		}
		result += scope
	}
	return result
}

// resolveAvatarURL 把站内相对头像路径（/attachments/{uuid}）解析为绝对地址；外链 URL 原样返回。
func (s *OIDCIDTokenService) resolveAvatarURL(avatar string) string {
	if avatar == "" || !strings.HasPrefix(avatar, "/") {
		return avatar
	}
	issuerURL, err := url.Parse(s.issuer)
	if err != nil || issuerURL.Host == "" {
		return avatar
	}
	return issuerURL.Scheme + "://" + issuerURL.Host + avatar
}

// addProfileClaims 添加profile scope的claims
func (s *OIDCIDTokenService) addProfileClaims(claims *IDTokenClaims, user *model.User) {
	claims.Name = user.RealName
	claims.Nickname = user.Nickname
	claims.PreferredUsername = user.Username
	claims.Picture = s.resolveAvatarURL(user.Avatar)
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
	if user.Email != nil {
		claims.Email = *user.Email
	}
	// 默认由 EmailVerifiedAt 派生；attributes 可覆盖（与 /userinfo 行为一致）
	claims.EmailVerified = user.EmailVerified()
	if user.Attributes != nil {
		if emailVerified, ok := user.Attributes["email_verified"].(bool); ok {
			claims.EmailVerified = emailVerified
		}
	}
}

// addPhoneClaims 添加phone scope的claims
func (s *OIDCIDTokenService) addPhoneClaims(claims *IDTokenClaims, user *model.User) {
	if user.Phone != nil {
		claims.PhoneNumber = *user.Phone
	}
	// 默认由 PhoneVerifiedAt 派生；attributes 可覆盖
	claims.PhoneNumberVerified = user.PhoneVerified()
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

// VerifyIDToken 验证 ID Token。
//
// 强制校验（OIDC Core §3.1.3.7）：
//   - 签名（由 rsaKeyManager 完成）
//   - iss == s.issuer
//   - aud 包含 expectedClientID（aud 可为 string 或 []string）
//   - exp > now，nbf <= now（若存在）
//
// expectedClientID 必填；调用方应传入当前 RP 的 client_id。
func (s *OIDCIDTokenService) VerifyIDToken(tokenString, expectedClientID string) (*IDTokenClaims, error) {
	if !s.rsaKeyManager.IsInitialized() {
		return nil, fmt.Errorf("RSA密钥未初始化")
	}
	if expectedClientID == "" {
		return nil, fmt.Errorf("expectedClientID 不能为空")
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

	iss, _ := claims["iss"].(string)
	if iss != s.issuer {
		return nil, fmt.Errorf("invalid issuer: got %q, expected %q", iss, s.issuer)
	}

	if !audienceContains(claims["aud"], expectedClientID) {
		return nil, fmt.Errorf("invalid audience: token aud does not contain %q", expectedClientID)
	}

	now := time.Now().Unix()
	if expF, ok := claims["exp"].(float64); ok {
		if int64(expF) <= now {
			return nil, fmt.Errorf("ID Token 已过期")
		}
	} else {
		return nil, fmt.Errorf("ID Token 缺少 exp")
	}
	if nbfF, ok := claims["nbf"].(float64); ok {
		if int64(nbfF) > now {
			return nil, fmt.Errorf("ID Token 尚未生效 (nbf)")
		}
	}

	idTokenClaims := &IDTokenClaims{Iss: iss}
	if sub, ok := claims["sub"].(string); ok {
		idTokenClaims.Sub = sub
	}
	if audStr, ok := claims["aud"].(string); ok {
		idTokenClaims.Aud = audStr
	} else {
		idTokenClaims.Aud = expectedClientID
	}
	if nonce, ok := claims["nonce"].(string); ok {
		idTokenClaims.Nonce = nonce
	}
	if expF, ok := claims["exp"].(float64); ok {
		idTokenClaims.Exp = int64(expF)
	}
	if iatF, ok := claims["iat"].(float64); ok {
		idTokenClaims.Iat = int64(iatF)
	}
	if atF, ok := claims["auth_time"].(float64); ok {
		idTokenClaims.AuthTime = int64(atF)
	}
	return idTokenClaims, nil
}

// audienceContains 判断 aud claim（string 或 []interface{}）是否包含指定 clientID。
func audienceContains(aud any, clientID string) bool {
	switch v := aud.(type) {
	case string:
		return v == clientID
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == clientID {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if s == clientID {
				return true
			}
		}
	}
	return false
}
