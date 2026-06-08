package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OIDCAuthorizationService OIDC授权服务
type OIDCAuthorizationService struct {
	authCodeDAO *dao.AuthorizationCodeDAO
	tokenDAO    *dao.OIDCTokenDAO
	appService  *ApplicationService
	userService *UserService
	db          *gorm.DB
}

// NewOIDCAuthorizationService 创建新的OIDC授权服务
func NewOIDCAuthorizationService(db *gorm.DB) *OIDCAuthorizationService {
	return &OIDCAuthorizationService{
		authCodeDAO: dao.NewAuthorizationCodeDAO(db),
		tokenDAO:    dao.NewOIDCTokenDAO(db),
		appService:  NewApplicationService(db),
		userService: NewUserService(db),
		db:          db,
	}
}

// ProcessAuthorize 处理授权请求
func (s *OIDCAuthorizationService) ProcessAuthorize(req *dto.AuthorizeRequest, userID uint, issuer string) (*dto.AuthorizeResponse, error) {
	// 1. 验证应用（OIDC客户端）
	app, err := s.appService.GetApplicationByClientID(req.ClientID)
	if err != nil {
		return s.errorResponse("invalid_client", "无效的客户端", req.State), nil
	}

	if app.ProtocolType != string(model.ProtocolTypeOIDC) {
		return s.errorResponse("invalid_client", "不是OIDC应用", req.State), nil
	}

	// 获取OIDC协议处理器
	handler := s.getOIDCHandler()

	// 2. 验证重定向URI
	if err := handler.ValidateRedirectURI(app, req.RedirectURI); err != nil {
		return s.errorResponse("invalid_request", "无效的重定向URI", req.State), nil
	}

	// 3. 验证响应类型
	if err := handler.ValidateResponseType(app, req.ResponseType); err != nil {
		return s.errorResponse("unsupported_response_type", "不支持的响应类型", req.State), nil
	}

	// 4. 验证作用域
	scopes := s.parseScopes(req.Scope)
	if err := handler.ValidateScopes(app, scopes); err != nil {
		return s.errorResponse("invalid_scope", "无效的作用域", req.State), nil
	}

	// 5. 验证PKCE
	// 公开客户端强制 PKCE；机密客户端若配置 requirePkce 则强制；不强制时若提供也需格式合规
	mustPKCE := handler.RequiresPKCE(app) || handler.IsPublicClient(app)
	if mustPKCE && req.CodeChallenge == "" {
		return s.errorResponse("invalid_request", "缺少 code_challenge（PKCE 必选）", req.State), nil
	}
	if req.CodeChallenge != "" {
		if err := validateCodeChallengeFormat(req.CodeChallenge, req.CodeChallengeMethod); err != nil {
			return s.errorResponse("invalid_request", err.Error(), req.State), nil
		}
	}

	// 6. 验证用户
	user, err := s.userService.GetUserModelByID(userID)
	if err != nil {
		return s.errorResponse("access_denied", "用户不存在", req.State), nil
	}

	if !user.CanLogin() {
		return s.errorResponse("access_denied", "用户无法登录", req.State), nil
	}

	// 6.5 检查应用授权范围
	if !s.appService.CheckUserAccess(app.ID, userID) {
		return s.errorResponse("access_denied", "您没有访问此应用的权限", req.State), nil
	}

	// 7. 处理不同的响应类型
	responseType := model.ResponseType(req.ResponseType)
	switch responseType {
	case model.ResponseTypeCode:
		return s.handleAuthorizationCodeFlow(req, app, user, scopes)
	case model.ResponseTypeToken:
		return s.handleImplicitFlow(req, app, user, scopes, issuer)
	case model.ResponseTypeIDToken:
		return s.handleImplicitIDTokenFlow(req, app, user, scopes, issuer)
	default:
		return s.errorResponse("unsupported_response_type", "不支持的响应类型", req.State), nil
	}
}

// ExchangeCodeForToken 交换授权码获取令牌
func (s *OIDCAuthorizationService) ExchangeCodeForToken(req *dto.TokenRequest, issuer string) (*dto.TokenResponse, error) {
	// 1. 验证授权类型
	if req.GrantType != string(model.GrantTypeAuthorizationCode) {
		return nil, constants.ErrUnsupportedGrantType
	}

	// 2. 验证应用（客户端）
	app, err := s.appService.ValidateApplication(req.ClientID, req.ClientSecret)
	if err != nil {
		return nil, constants.ErrInvalidClient
	}

	// 3. 原子消费授权码（RFC 6749 §4.1.2：code 必须一次性）
	//    若消费失败且 code 曾存在过，则将原先由该 code 签发的 token 全部撤销
	authCode, err := s.authCodeDAO.ConsumeAtomic(req.Code)
	if err != nil {
		if errors.Is(err, dao.ErrAuthorizationCodeConsumed) {
			if existing, _ := s.authCodeDAO.GetByCode(req.Code); existing != nil {
				// 检测到 code 复用，撤销所有基于该 code 的 token
				_ = s.tokenDAO.RevokeByAuthorizationCode(existing.ID)
				if logger := helper.GetLogger(); logger != nil {
					logger.Warn(fmt.Sprintf("authorization_code replay detected: fp=%s", codeFingerprint(req.Code)))
				}
			}
			return nil, constants.ErrInvalidGrant
		}
		return nil, constants.ErrInvalidGrant
	}

	// 4. 校验授权码的上下文（client_id、redirect_uri、PKCE）
	if err := s.validateAuthorizationCode(authCode, req, app); err != nil {
		// 上下文不匹配视为潜在滥用，撤销该 code 相关 token
		_ = s.tokenDAO.RevokeByAuthorizationCode(authCode.ID)
		return nil, err
	}

	// 5. 生成令牌（将 authCode.ID 透传，用于 token 与 code 的关联及后续回收）
	return s.generateTokensFromCode(app, authCode, issuer)
}

// generateTokensFromCode 基于已消费的授权码生成 token，并设置 authorization_code_id 关联
func (s *OIDCAuthorizationService) generateTokensFromCode(app *model.Application, authCode *model.AuthorizationCode, issuer string) (*dto.TokenResponse, error) {
	resp, err := s.GenerateTokens(app, authCode.UserID, authCode.Scopes, authCode.Nonce, issuer)
	if err != nil {
		return nil, err
	}
	// 将 token 记录与授权码绑定，便于重放检测时连锁撤销
	// 通过 access_token hash 反查最新插入的 token 行
	if token, lookupErr := s.tokenDAO.GetByAccessToken(resp.AccessToken); lookupErr == nil && token != nil {
		codeID := authCode.ID
		token.AuthorizationCodeID = &codeID
		_ = s.tokenDAO.Update(token)
	}
	return resp, nil
}

// ExchangeClientCredentials 客户端凭据流程
func (s *OIDCAuthorizationService) ExchangeClientCredentials(req *dto.TokenRequest, issuer string) (*dto.TokenResponse, error) {
	// 1. 验证授权类型
	if req.GrantType != string(model.GrantTypeClientCredentials) {
		return nil, constants.ErrUnsupportedGrantType
	}

	// 2. 验证应用（客户端）
	app, err := s.appService.ValidateApplication(req.ClientID, req.ClientSecret)
	if err != nil {
		return nil, constants.ErrInvalidClient
	}

	// 3. 验证客户端支持client_credentials授权类型
	handler := s.getOIDCHandler()
	if err := handler.ValidateGrantType(app, string(model.GrantTypeClientCredentials)); err != nil {
		return nil, constants.ErrUnauthorizedClient
	}

	// 4. 解析请求的作用域
	scopes := s.parseScopes(req.Scope)
	if len(scopes) == 0 {
		// 如果没有指定scope，使用应用配置的默认scopes
		if appScopes, ok := app.ProtocolConfig["scopes"].([]interface{}); ok {
			for _, scope := range appScopes {
				if scopeStr, ok := scope.(string); ok {
					scopes = append(scopes, scopeStr)
				}
			}
		}
	}

	// 5. 验证作用域
	if len(scopes) > 0 {
		if err := handler.ValidateScopes(app, scopes); err != nil {
			return nil, constants.ErrInvalidScope
		}
	}

	// 6. 生成 JWT 格式的访问令牌（不含refresh_token和id_token）
	accessToken, err := s.generateClientCredentialsJWTAccessToken(app, scopes, issuer)
	if err != nil {
		return nil, constants.ErrServerError
	}
	clientID := s.getClientIDFromApp(app)
	tokenLifetime := s.getTokenLifetimeFromApp(app)

	// 创建令牌记录（userID为0表示客户端凭据流程，不关联用户）
	token := &model.OIDCToken{
		AccessToken: accessToken,
		TokenType:   model.TokenTypeBearer,
		ClientID:    clientID,
		UserID:      0, // 客户端凭据流程不关联用户
		Scopes:      scopes,
		ExpiresAt:   time.Now().Add(time.Duration(tokenLifetime) * time.Second),
		Attributes:  make(map[string]interface{}),
	}

	// 保存令牌
	if err := s.tokenDAO.Create(token); err != nil {
		return nil, constants.ErrServerError
	}

	response := &dto.TokenResponse{
		AccessToken: accessToken,
		TokenType:   string(token.TokenType),
		ExpiresIn:   token.GetExpiresIn(),
		Scope:       strings.Join(scopes, " "),
	}

	return response, nil
}

// RefreshToken 刷新令牌
func (s *OIDCAuthorizationService) RefreshToken(req *dto.TokenRequest, issuer string) (*dto.TokenResponse, error) {
	// 1. 验证授权类型
	if req.GrantType != string(model.GrantTypeRefreshToken) {
		return nil, constants.ErrUnsupportedGrantType
	}

	// 2. 验证应用（客户端）
	app, err := s.appService.ValidateApplication(req.ClientID, req.ClientSecret)
	if err != nil {
		return nil, constants.ErrInvalidClient
	}

	// 获取ClientID
	clientID := s.getClientIDFromApp(app)

	// 3. 获取刷新令牌
	token, err := s.tokenDAO.GetValidByRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, constants.ErrInvalidGrant
	}

	// 4. 验证刷新令牌
	if token.ClientID != clientID {
		return nil, constants.ErrInvalidGrant
	}

	// 5. 撤销旧令牌
	token.Revoke()
	if err := s.tokenDAO.Update(token); err != nil {
		return nil, constants.ErrServerError
	}

	// 6. 解析请求的作用域
	var scopes []string
	if req.Scope != "" {
		scopes = s.parseScopes(req.Scope)
		// 验证请求的作用域不能超过原有的作用域
		if err := s.validateRefreshScopes(token.Scopes, scopes); err != nil {
			return nil, constants.ErrInvalidScope
		}
	} else {
		scopes = token.Scopes
	}

	// 7. 生成新令牌
	return s.GenerateTokens(app, token.UserID, scopes, "", issuer)
}

// IntrospectToken 令牌内省（RFC 7662）
//
// 强制要求：
//   - 客户端必须携带 client_id + client_secret（或 HTTP Basic），否则返回 invalid_client
//   - 令牌的 client_id 必须与请求客户端一致，否则仅返回 {"active":true} 的最小响应
//     避免跨客户端元数据泄漏（RFC 7662 §2.2）
func (s *OIDCAuthorizationService) IntrospectToken(req *dto.IntrospectionRequest) (*dto.IntrospectionResponse, error) {
	if req.ClientID == "" || req.ClientSecret == "" {
		return nil, constants.ErrInvalidClient
	}
	app, err := s.appService.ValidateApplication(req.ClientID, req.ClientSecret)
	if err != nil {
		return nil, constants.ErrInvalidClient
	}
	requesterClientID := s.getClientIDFromApp(app)

	token, err := s.tokenDAO.GetValidByAccessToken(req.Token)
	if err != nil {
		return &dto.IntrospectionResponse{Active: false}, nil
	}

	// 令牌不属于请求客户端：返回最小信息，避免跨客户端泄漏
	if token.ClientID != requesterClientID {
		return &dto.IntrospectionResponse{Active: true}, nil
	}

	response := &dto.IntrospectionResponse{
		Active:    true,
		ClientID:  token.ClientID,
		TokenType: string(token.TokenType),
		Exp:       token.ExpiresAt.Unix(),
		Iat:       token.CreatedAt.Unix(),
		Scope:     strings.Join(token.Scopes, " "),
	}

	if token.UserID > 0 {
		user, err := s.userService.GetUserByID(token.UserID)
		if err != nil {
			return &dto.IntrospectionResponse{Active: false}, nil
		}
		response.Sub = user.UUID
		response.Username = user.Username
	}

	return response, nil
}

// RevokeToken 撤销令牌
func (s *OIDCAuthorizationService) RevokeToken(req *dto.RevocationRequest) error {
	// 1. 验证应用（客户端）
	app, err := s.appService.ValidateApplication(req.ClientID, req.ClientSecret)
	if err != nil {
		return constants.ErrInvalidClient
	}

	// 获取ClientID
	clientID := s.getClientIDFromApp(app)

	// 2. 根据令牌类型处理
	if req.TokenTypeHint == "refresh_token" || req.TokenTypeHint == "" {
		// 尝试撤销刷新令牌
		if err := s.tokenDAO.RevokeByRefreshToken(req.Token); err == nil {
			return nil
		}
	}

	// 3. 尝试撤销访问令牌
	token, err := s.tokenDAO.GetByAccessToken(req.Token)
	if err != nil {
		// 令牌不存在或已过期，仍然返回成功（RFC 7009）
		return nil
	}

	// 4. 验证令牌所属客户端
	if token.ClientID != clientID {
		return constants.ErrInvalidGrant
	}

	// 5. 撤销令牌
	if err := s.tokenDAO.RevokeToken(req.Token); err != nil {
		return constants.ErrServerError
	}

	return nil
}

// 私有方法

// handleAuthorizationCodeFlow 处理授权码流程
func (s *OIDCAuthorizationService) handleAuthorizationCodeFlow(req *dto.AuthorizeRequest, app *model.Application, user *model.User, scopes []string) (*dto.AuthorizeResponse, error) {
	// 生成授权码
	code := s.generateAuthorizationCode()

	clientID := s.getClientIDFromApp(app)

	authCode := &model.AuthorizationCode{
		Code:                code,
		ClientID:            clientID,
		UserID:              user.ID,
		RedirectURI:         req.RedirectURI,
		Scopes:              scopes,
		State:               req.State,
		Nonce:               req.Nonce,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}

	if err := s.authCodeDAO.Create(authCode); err != nil {
		if logger := helper.GetLogger(); logger != nil {
			logger.Error(fmt.Sprintf("创建授权码失败 fp=%s err=%v", codeFingerprint(code), err))
		}
		return s.errorResponse("server_error", "创建授权码失败", req.State), nil
	}

	if logger := helper.GetLogger(); logger != nil {
		logger.Debug(fmt.Sprintf("authorization_code issued: fp=%s client=%s user=%d",
			codeFingerprint(code), clientID, user.ID))
	}

	return &dto.AuthorizeResponse{
		Code:  code,
		State: req.State,
	}, nil
}

// handleImplicitFlow 处理隐式流程
func (s *OIDCAuthorizationService) handleImplicitFlow(req *dto.AuthorizeRequest, app *model.Application, user *model.User, scopes []string, issuer string) (*dto.AuthorizeResponse, error) {
	// 隐式流程直接生成访问令牌
	tokenResponse, err := s.GenerateTokens(app, user.ID, scopes, req.Nonce, issuer)
	if err != nil {
		return s.errorResponse("server_error", "服务器错误", req.State), nil
	}

	// 注意：隐式流程不返回刷新令牌
	return &dto.AuthorizeResponse{
		Code:  tokenResponse.AccessToken, // 这里实际上是访问令牌
		State: req.State,
	}, nil
}

// handleImplicitIDTokenFlow 处理隐式ID令牌流程
func (s *OIDCAuthorizationService) handleImplicitIDTokenFlow(req *dto.AuthorizeRequest, app *model.Application, user *model.User, scopes []string, issuer string) (*dto.AuthorizeResponse, error) {
	// 生成ID令牌
	idToken, err := s.generateIDToken(app, user, scopes, req.Nonce, issuer)
	if err != nil {
		return s.errorResponse("server_error", "服务器错误", req.State), nil
	}

	return &dto.AuthorizeResponse{
		Code:  idToken,
		State: req.State,
	}, nil
}

// validateAuthorizationCode 验证授权码
func (s *OIDCAuthorizationService) validateAuthorizationCode(authCode *model.AuthorizationCode, req *dto.TokenRequest, app *model.Application) error {
	clientID := s.getClientIDFromApp(app)

	// 验证客户端ID
	if authCode.ClientID != clientID {
		return constants.ErrInvalidGrant
	}

	// 验证重定向URI
	if authCode.RedirectURI != req.RedirectURI {
		return constants.ErrInvalidGrant
	}

	// 验证PKCE（如果有）
	if authCode.CodeChallenge != "" {
		if req.CodeVerifier == "" {
			return constants.ErrInvalidRequest
		}

		if err := s.validatePKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, req.CodeVerifier); err != nil {
			return constants.ErrInvalidGrant
		}
	}

	return nil
}

// validatePKCE 验证 PKCE（RFC 7636）
//   - code_verifier 长度 43-128，字符集 [A-Z a-z 0-9 - . _ ~]
//   - plain 模式默认禁用（可通过配置 oidc.pkce.allow_plain=true 开启）
//   - 常量时间比较，防计时攻击
func (s *OIDCAuthorizationService) validatePKCE(codeChallenge, codeChallengeMethod, codeVerifier string) error {
	if err := validateCodeVerifierFormat(codeVerifier); err != nil {
		return err
	}

	var computed string
	switch codeChallengeMethod {
	case "plain":
		if !pkcePlainAllowed() {
			return errors.New("unsupported_code_challenge_method")
		}
		computed = codeVerifier
	case "S256", "":
		hash := sha256.Sum256([]byte(codeVerifier))
		computed = base64.RawURLEncoding.EncodeToString(hash[:])
	default:
		return errors.New("unsupported_code_challenge_method")
	}

	if subtle.ConstantTimeCompare([]byte(computed), []byte(codeChallenge)) != 1 {
		return errors.New("invalid_code_verifier")
	}
	return nil
}

var pkceAllowedCharset = regexp.MustCompile(`^[A-Za-z0-9\-._~]+$`)

// validateCodeChallengeFormat 校验授权端点收到的 code_challenge（RFC 7636 §4.2）
func validateCodeChallengeFormat(challenge, method string) error {
	if l := len(challenge); l < 43 || l > 128 {
		return errors.New("code_challenge 长度必须在 43-128 字符之间")
	}
	if !pkceAllowedCharset.MatchString(challenge) {
		return errors.New("code_challenge 字符非法，仅允许 [A-Za-z0-9-._~]")
	}
	if method != "" && method != "S256" && method != "plain" {
		return errors.New("code_challenge_method 仅支持 S256 或 plain")
	}
	if method == "plain" && !pkcePlainAllowed() {
		return errors.New("plain 挑战方法已禁用，请使用 S256")
	}
	return nil
}

// validateCodeVerifierFormat 校验 token 端点收到的 code_verifier（RFC 7636 §4.1）
func validateCodeVerifierFormat(verifier string) error {
	if l := len(verifier); l < 43 || l > 128 {
		return errors.New("code_verifier 长度必须在 43-128 字符之间")
	}
	if !pkceAllowedCharset.MatchString(verifier) {
		return errors.New("code_verifier 字符非法")
	}
	return nil
}

// pkcePlainAllowed 是否允许 plain 挑战方法（默认 false）
func pkcePlainAllowed() bool {
	env := helper.GetEnv()
	if env == nil {
		return false
	}
	return env.GetBool("oidc.pkce.allow_plain", false)
}

// validateRefreshScopes 验证刷新令牌的作用域
func (s *OIDCAuthorizationService) validateRefreshScopes(originalScopes, requestedScopes []string) error {
	originalScopeMap := make(map[string]bool)
	for _, scope := range originalScopes {
		originalScopeMap[scope] = true
	}

	for _, scope := range requestedScopes {
		if !originalScopeMap[scope] {
			return errors.New("requested scope exceeds original scope")
		}
	}

	return nil
}

// GenerateTokens 生成访问令牌和刷新令牌（导出以供登录接口使用）
// ID Token 中包含 at_hash，使客户端可验证 access_token 与 id_token 的绑定关系
func (s *OIDCAuthorizationService) GenerateTokens(app *model.Application, userID uint, scopes []string, nonce string, issuer string) (*dto.TokenResponse, error) {
	accessToken, err := s.generateJWTAccessToken(app, userID, scopes, issuer)
	if err != nil {
		return nil, fmt.Errorf("生成访问令牌失败: %w", err)
	}
	refreshToken := s.generateRefreshToken()

	clientID := s.getClientIDFromApp(app)
	tokenLifetime := s.getTokenLifetimeFromApp(app)

	token := &model.OIDCToken{
		AccessToken:  accessToken,
		TokenType:    model.TokenTypeBearer,
		ClientID:     clientID,
		UserID:       userID,
		Scopes:       scopes,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenLifetime) * time.Second),
		Attributes:   make(map[string]interface{}),
	}

	// 生成 ID 令牌（若 scope 包含 openid），并写入 at_hash
	if s.hasScope(scopes, "openid") {
		user, err := s.userService.GetUserModelByID(userID)
		if err != nil {
			return nil, err
		}

		idToken, err := s.generateIDTokenWithHashes(app, user, scopes, nonce, issuer, accessToken, "")
		if err != nil {
			return nil, err
		}
		token.IDToken = idToken
	}

	// 保存令牌
	if err := s.tokenDAO.Create(token); err != nil {
		return nil, err
	}

	response := &dto.TokenResponse{
		AccessToken:  accessToken,
		TokenType:    string(token.TokenType),
		ExpiresIn:    token.GetExpiresIn(),
		RefreshToken: refreshToken,
		Scope:        strings.Join(scopes, " "),
	}

	if token.IDToken != "" {
		response.IDToken = token.IDToken
	}

	return response, nil
}

// generateIDToken 生成 ID Token（不含 at_hash / c_hash）
func (s *OIDCAuthorizationService) generateIDToken(app *model.Application, user *model.User, scopes []string, nonce string, issuer string) (string, error) {
	return s.generateIDTokenWithHashes(app, user, scopes, nonce, issuer, "", "")
}

// generateIDTokenWithHashes 生成 ID Token，携带可选的 at_hash / c_hash
func (s *OIDCAuthorizationService) generateIDTokenWithHashes(
	app *model.Application, user *model.User, scopes []string, nonce string, issuer string,
	accessToken, authCode string,
) (string, error) {
	// 获取全局配置
	env := helper.GetEnv()
	idTokenLifetime := env.GetInt("oidc.token_lifetime.id_token", 3600)

	// 1. 获取客户端ID
	clientID := s.getClientIDFromApp(app)
	if clientID == "" {
		return "", fmt.Errorf("无法获取客户端ID")
	}

	// 2. 从数据库获取应用的主密钥管理器
	keyService := NewApplicationKeyService(s.db)
	keyManager, err := keyService.GetPrimaryKeyManagerForApplication(app.ID)
	if err != nil {
		return "", fmt.Errorf("获取应用签名密钥失败: %w", err)
	}

	// 3. 验证密钥是否已初始化
	if !keyManager.IsInitialized() {
		return "", fmt.Errorf("应用签名密钥未正确初始化")
	}

	// 4. 根据密钥类型创建ID Token服务
	var idTokenService *OIDCIDTokenService

	switch keyManager.GetKeyType() {
	case jwt.KeyTypeRSA:
		rsaKeyManager, ok := keyManager.(*jwt.RSAKeyManager)
		if !ok {
			return "", fmt.Errorf("密钥管理器类型转换失败")
		}
		idTokenService = NewOIDCIDTokenServiceWithRSA(rsaKeyManager, issuer, idTokenLifetime)

	case jwt.KeyTypeHMAC:
		hmacKeyManager, ok := keyManager.(*jwt.HMACKeyManager)
		if !ok {
			return "", fmt.Errorf("密钥管理器类型转换失败")
		}
		idTokenService = NewOIDCIDTokenServiceWithHMAC(
			hmacKeyManager.GetSecret(),
			keyManager.GetKeyID(),
			issuer,
			idTokenLifetime,
		)

	default:
		return "", fmt.Errorf("不支持的密钥类型: %s", keyManager.GetKeyType())
	}

	// 5. 生成ID Token（包含 at_hash / c_hash，若上层提供）
	idToken, err := idTokenService.GenerateIDTokenWithHashes(user, clientID, scopes, nonce, nil, accessToken, authCode)
	if err != nil {
		return "", fmt.Errorf("生成ID Token失败: %w", err)
	}

	return idToken, nil
}

// generateJWTAccessToken 生成 JWT 格式的 Access Token
func (s *OIDCAuthorizationService) generateJWTAccessToken(app *model.Application, userID uint, scopes []string, issuer string) (string, error) {
	accessTokenLifetime := s.getTokenLifetimeFromApp(app)
	if accessTokenLifetime <= 0 {
		// 只有应用未配置 tokenLifetime 时才回退到全局配置。
		env := helper.GetEnv()
		accessTokenLifetime = env.GetInt("oidc.token_lifetime.access_token", 3600)
	}

	// 1. 获取客户端ID
	clientID := s.getClientIDFromApp(app)
	if clientID == "" {
		return "", fmt.Errorf("无法获取客户端ID")
	}

	// 2. 从数据库获取应用的主密钥管理器
	keyService := NewApplicationKeyService(s.db)
	keyManager, err := keyService.GetPrimaryKeyManagerForApplication(app.ID)
	if err != nil {
		return "", fmt.Errorf("获取应用签名密钥失败: %w", err)
	}

	// 3. 验证密钥是否已初始化
	if !keyManager.IsInitialized() {
		return "", fmt.Errorf("应用签名密钥未正确初始化")
	}

	// 4. 获取用户信息
	user, err := s.userService.GetUserModelByID(userID)
	if err != nil {
		return "", fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 5. 根据密钥类型创建 Token 服务
	var tokenService *OIDCIDTokenService

	switch keyManager.GetKeyType() {
	case jwt.KeyTypeRSA:
		rsaKeyManager, ok := keyManager.(*jwt.RSAKeyManager)
		if !ok {
			return "", fmt.Errorf("密钥管理器类型转换失败")
		}
		tokenService = NewOIDCIDTokenServiceWithRSA(rsaKeyManager, issuer, accessTokenLifetime)

	case jwt.KeyTypeHMAC:
		hmacKeyManager, ok := keyManager.(*jwt.HMACKeyManager)
		if !ok {
			return "", fmt.Errorf("密钥管理器类型转换失败")
		}
		tokenService = NewOIDCIDTokenServiceWithHMAC(
			hmacKeyManager.GetSecret(),
			keyManager.GetKeyID(),
			issuer,
			accessTokenLifetime,
		)

	default:
		return "", fmt.Errorf("不支持的密钥类型: %s", keyManager.GetKeyType())
	}

	// 6. 生成 JWT Access Token
	accessToken, err := tokenService.GenerateAccessToken(user, clientID, scopes)
	if err != nil {
		return "", fmt.Errorf("生成 Access Token 失败: %w", err)
	}

	return accessToken, nil
}

// generateClientCredentialsJWTAccessToken 生成客户端凭据流程的 JWT Access Token
func (s *OIDCAuthorizationService) generateClientCredentialsJWTAccessToken(app *model.Application, scopes []string, issuer string) (string, error) {
	accessTokenLifetime := s.getTokenLifetimeFromApp(app)
	if accessTokenLifetime <= 0 {
		// 只有应用未配置 tokenLifetime 时才回退到全局配置。
		env := helper.GetEnv()
		accessTokenLifetime = env.GetInt("oidc.token_lifetime.access_token", 3600)
	}

	// 1. 获取客户端ID
	clientID := s.getClientIDFromApp(app)
	if clientID == "" {
		return "", fmt.Errorf("无法获取客户端ID")
	}

	// 2. 从数据库获取应用的主密钥管理器
	keyService := NewApplicationKeyService(s.db)
	keyManager, err := keyService.GetPrimaryKeyManagerForApplication(app.ID)
	if err != nil {
		return "", fmt.Errorf("获取应用签名密钥失败: %w", err)
	}

	// 3. 验证密钥是否已初始化
	if !keyManager.IsInitialized() {
		return "", fmt.Errorf("应用签名密钥未正确初始化")
	}

	// 4. 根据密钥类型创建 Token 服务
	var tokenService *OIDCIDTokenService

	switch keyManager.GetKeyType() {
	case jwt.KeyTypeRSA:
		rsaKeyManager, ok := keyManager.(*jwt.RSAKeyManager)
		if !ok {
			return "", fmt.Errorf("密钥管理器类型转换失败")
		}
		tokenService = NewOIDCIDTokenServiceWithRSA(rsaKeyManager, issuer, accessTokenLifetime)

	case jwt.KeyTypeHMAC:
		hmacKeyManager, ok := keyManager.(*jwt.HMACKeyManager)
		if !ok {
			return "", fmt.Errorf("密钥管理器类型转换失败")
		}
		tokenService = NewOIDCIDTokenServiceWithHMAC(
			hmacKeyManager.GetSecret(),
			keyManager.GetKeyID(),
			issuer,
			accessTokenLifetime,
		)

	default:
		return "", fmt.Errorf("不支持的密钥类型: %s", keyManager.GetKeyType())
	}

	// 5. 生成客户端凭据的 JWT Access Token
	accessToken, err := tokenService.GenerateClientCredentialsAccessToken(clientID, scopes)
	if err != nil {
		return "", fmt.Errorf("生成 Access Token 失败: %w", err)
	}

	return accessToken, nil
}

// 辅助方法

// codeFingerprint 生成授权码的不可逆指纹，用于日志中指代某一条 code
// 截取 SHA-256 前 16 个十六进制字符（64 bit）足以在日志里追踪一条 code 的生命周期
// 而不会泄漏明文
func codeFingerprint(code string) string {
	if code == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:8])
}

func (s *OIDCAuthorizationService) generateAuthorizationCode() string {
	v7, err := uuid.NewV7()
	if err != nil {
		v7 = uuid.New()
	}
	return v7.String()
}

func (s *OIDCAuthorizationService) generateAccessToken() string {
	v7, err := uuid.NewV7()
	if err != nil {
		v7 = uuid.New()
	}
	return v7.String()
}

func (s *OIDCAuthorizationService) generateRefreshToken() string {
	v7, err := uuid.NewV7()
	if err != nil {
		v7 = uuid.New()
	}
	return v7.String()
}

func (s *OIDCAuthorizationService) parseScopes(scopeStr string) []string {
	if scopeStr == "" {
		return []string{}
	}
	return strings.Split(scopeStr, " ")
}

// ParseScopes 公开的解析scopes方法
func (s *OIDCAuthorizationService) ParseScopes(scopeStr string) []string {
	return s.parseScopes(scopeStr)
}

func (s *OIDCAuthorizationService) hasScope(scopes []string, targetScope string) bool {
	for _, scope := range scopes {
		if scope == targetScope {
			return true
		}
	}
	return false
}

func (s *OIDCAuthorizationService) errorResponse(error, description, state string) *dto.AuthorizeResponse {
	return &dto.AuthorizeResponse{
		Error:            error,
		ErrorDescription: description,
		State:            state,
	}
}

// getOIDCHandler 获取OIDC协议处理器
func (s *OIDCAuthorizationService) getOIDCHandler() *protocol.OIDCProtocolHandler {
	return protocol.NewOIDCProtocolHandler(s.db)
}

// getClientIDFromApp 从应用中获取ClientID
func (s *OIDCAuthorizationService) getClientIDFromApp(app *model.Application) string {
	if clientID, ok := app.ProtocolConfig["clientId"].(string); ok {
		return clientID
	}
	return ""
}

// getTokenLifetimeFromApp 从应用中获取Token有效期
func (s *OIDCAuthorizationService) getTokenLifetimeFromApp(app *model.Application) int {
	if tokenLifetime, ok := app.ProtocolConfig["tokenLifetime"].(float64); ok {
		return int(tokenLifetime)
	}
	if tokenLifetime, ok := app.ProtocolConfig["tokenLifetime"].(int); ok {
		return tokenLifetime
	}
	return 3600 // 默认1小时
}
