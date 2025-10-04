package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"cnb.cool/mliev/examples/go-web/app/dao"
	"cnb.cool/mliev/examples/go-web/app/dto"
	"cnb.cool/mliev/examples/go-web/app/model"
	"gorm.io/gorm"
)

// OIDCAuthorizationService OIDC授权服务
type OIDCAuthorizationService struct {
	authCodeDAO   *dao.AuthorizationCodeDAO
	tokenDAO      *dao.OIDCTokenDAO
	clientService *OIDCClientService
	userService   *UserService
	db            *gorm.DB
}

// NewOIDCAuthorizationService 创建新的OIDC授权服务
func NewOIDCAuthorizationService(db *gorm.DB) *OIDCAuthorizationService {
	return &OIDCAuthorizationService{
		authCodeDAO:   dao.NewAuthorizationCodeDAO(db),
		tokenDAO:      dao.NewOIDCTokenDAO(db),
		clientService: NewOIDCClientService(db),
		userService:   NewUserService(db),
		db:            db,
	}
}

// ProcessAuthorize 处理授权请求
func (s *OIDCAuthorizationService) ProcessAuthorize(req *dto.AuthorizeRequest, userID uint) (*dto.AuthorizeResponse, error) {
	// 1. 验证客户端
	client, err := s.clientService.GetClientByClientID(req.ClientID)
	if err != nil {
		return s.errorResponse("invalid_client", "无效的客户端", req.State), nil
	}

	// 2. 验证重定向URI
	if err := s.clientService.ValidateRedirectURI(client, req.RedirectURI); err != nil {
		return s.errorResponse("invalid_request", "无效的重定向URI", req.State), nil
	}

	// 3. 验证响应类型
	responseType := model.ResponseType(req.ResponseType)
	if err := s.clientService.ValidateResponseType(client, responseType); err != nil {
		return s.errorResponse("unsupported_response_type", "不支持的响应类型", req.State), nil
	}

	// 4. 验证作用域
	scopes := s.parseScopes(req.Scope)
	if err := s.clientService.ValidateScopes(client, scopes); err != nil {
		return s.errorResponse("invalid_scope", "无效的作用域", req.State), nil
	}

	// 5. 验证PKCE（如果需要）
	if client.RequirePKCE && req.CodeChallenge == "" {
		return s.errorResponse("invalid_request", "需要PKCE验证", req.State), nil
	}

	// 6. 验证用户
	user, err := s.userService.GetUserModelByID(userID)
	if err != nil {
		return s.errorResponse("access_denied", "用户不存在", req.State), nil
	}

	if !user.CanLogin() {
		return s.errorResponse("access_denied", "用户无法登录", req.State), nil
	}

	// 7. 处理不同的响应类型
	switch responseType {
	case model.ResponseTypeCode:
		return s.handleAuthorizationCodeFlow(req, client, user, scopes)
	case model.ResponseTypeToken:
		return s.handleImplicitFlow(req, client, user, scopes)
	case model.ResponseTypeIDToken:
		return s.handleImplicitIDTokenFlow(req, client, user, scopes)
	default:
		return s.errorResponse("unsupported_response_type", "不支持的响应类型", req.State), nil
	}
}

// ExchangeCodeForToken 交换授权码获取令牌
func (s *OIDCAuthorizationService) ExchangeCodeForToken(req *dto.TokenRequest) (*dto.TokenResponse, error) {
	// 1. 验证授权类型
	if req.GrantType != string(model.GrantTypeAuthorizationCode) {
		return nil, errors.New("unsupported_grant_type")
	}

	// 2. 验证客户端
	client, err := s.clientService.ValidateClient(req.ClientID, req.ClientSecret)
	if err != nil {
		return nil, errors.New("invalid_client")
	}

	// 3. 获取授权码
	authCode, err := s.authCodeDAO.GetValidByCode(req.Code)
	if err != nil {
		return nil, errors.New("invalid_grant")
	}

	// 4. 验证授权码
	if err := s.validateAuthorizationCode(authCode, req, client); err != nil {
		return nil, err
	}

	// 5. 标记授权码为已使用
	authCode.MarkAsUsed()
	if err := s.authCodeDAO.Update(authCode); err != nil {
		return nil, errors.New("server_error")
	}

	// 6. 生成令牌
	return s.generateTokens(client, authCode.UserID, authCode.Scopes, authCode.Nonce)
}

// RefreshToken 刷新令牌
func (s *OIDCAuthorizationService) RefreshToken(req *dto.TokenRequest) (*dto.TokenResponse, error) {
	// 1. 验证授权类型
	if req.GrantType != string(model.GrantTypeRefreshToken) {
		return nil, errors.New("unsupported_grant_type")
	}

	// 2. 验证客户端
	client, err := s.clientService.ValidateClient(req.ClientID, req.ClientSecret)
	if err != nil {
		return nil, errors.New("invalid_client")
	}

	// 3. 获取刷新令牌
	token, err := s.tokenDAO.GetValidByRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, errors.New("invalid_grant")
	}

	// 4. 验证刷新令牌
	if token.ClientID != client.ClientID {
		return nil, errors.New("invalid_grant")
	}

	// 5. 撤销旧令牌
	token.Revoke()
	if err := s.tokenDAO.Update(token); err != nil {
		return nil, errors.New("server_error")
	}

	// 6. 解析请求的作用域
	var scopes []string
	if req.Scope != "" {
		scopes = s.parseScopes(req.Scope)
		// 验证请求的作用域不能超过原有的作用域
		if err := s.validateRefreshScopes(token.Scopes, scopes); err != nil {
			return nil, errors.New("invalid_scope")
		}
	} else {
		scopes = token.Scopes
	}

	// 7. 生成新令牌
	return s.generateTokens(client, token.UserID, scopes, "")
}

// IntrospectToken 令牌内省
func (s *OIDCAuthorizationService) IntrospectToken(req *dto.IntrospectionRequest) (*dto.IntrospectionResponse, error) {
	// 1. 验证客户端（可选）
	if req.ClientID != "" {
		_, err := s.clientService.ValidateClient(req.ClientID, req.ClientSecret)
		if err != nil {
			return &dto.IntrospectionResponse{Active: false}, nil
		}
	}

	// 2. 获取令牌
	token, err := s.tokenDAO.GetValidByAccessToken(req.Token)
	if err != nil {
		return &dto.IntrospectionResponse{Active: false}, nil
	}

	// 3. 获取用户信息
	user, err := s.userService.GetUserByID(token.UserID)
	if err != nil {
		return &dto.IntrospectionResponse{Active: false}, nil
	}

	// 4. 构建响应
	return &dto.IntrospectionResponse{
		Active:    true,
		Sub:       strconv.FormatUint(uint64(user.ID), 10),
		ClientID:  token.ClientID,
		Username:  user.Username,
		TokenType: string(token.TokenType),
		Exp:       token.ExpiresAt.Unix(),
		Iat:       token.CreatedAt.Unix(),
		Scope:     strings.Join(token.Scopes, " "),
	}, nil
}

// RevokeToken 撤销令牌
func (s *OIDCAuthorizationService) RevokeToken(req *dto.RevocationRequest) error {
	// 1. 验证客户端
	client, err := s.clientService.ValidateClient(req.ClientID, req.ClientSecret)
	if err != nil {
		return errors.New("invalid_client")
	}

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
	if token.ClientID != client.ClientID {
		return errors.New("invalid_grant")
	}

	// 5. 撤销令牌
	if err := s.tokenDAO.RevokeToken(req.Token); err != nil {
		return errors.New("server_error")
	}

	return nil
}

// 私有方法

// handleAuthorizationCodeFlow 处理授权码流程
func (s *OIDCAuthorizationService) handleAuthorizationCodeFlow(req *dto.AuthorizeRequest, client *model.OIDCClient, user *model.User, scopes []string) (*dto.AuthorizeResponse, error) {
	// 生成授权码
	code := s.generateAuthorizationCode()

	authCode := &model.AuthorizationCode{
		Code:                code,
		ClientID:            client.ClientID,
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
		return s.errorResponse("server_error", "服务器错误", req.State), nil
	}

	return &dto.AuthorizeResponse{
		Code:  code,
		State: req.State,
	}, nil
}

// handleImplicitFlow 处理隐式流程
func (s *OIDCAuthorizationService) handleImplicitFlow(req *dto.AuthorizeRequest, client *model.OIDCClient, user *model.User, scopes []string) (*dto.AuthorizeResponse, error) {
	// 隐式流程直接生成访问令牌
	tokenResponse, err := s.generateTokens(client, user.ID, scopes, req.Nonce)
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
func (s *OIDCAuthorizationService) handleImplicitIDTokenFlow(req *dto.AuthorizeRequest, client *model.OIDCClient, user *model.User, scopes []string) (*dto.AuthorizeResponse, error) {
	// 生成ID令牌
	idToken, err := s.generateIDToken(client, user, scopes, req.Nonce)
	if err != nil {
		return s.errorResponse("server_error", "服务器错误", req.State), nil
	}

	return &dto.AuthorizeResponse{
		Code:  idToken,
		State: req.State,
	}, nil
}

// validateAuthorizationCode 验证授权码
func (s *OIDCAuthorizationService) validateAuthorizationCode(authCode *model.AuthorizationCode, req *dto.TokenRequest, client *model.OIDCClient) error {
	// 验证客户端ID
	if authCode.ClientID != client.ClientID {
		return errors.New("invalid_grant")
	}

	// 验证重定向URI
	if authCode.RedirectURI != req.RedirectURI {
		return errors.New("invalid_grant")
	}

	// 验证PKCE（如果有）
	if authCode.CodeChallenge != "" {
		if req.CodeVerifier == "" {
			return errors.New("invalid_request")
		}

		if err := s.validatePKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, req.CodeVerifier); err != nil {
			return errors.New("invalid_grant")
		}
	}

	return nil
}

// validatePKCE 验证PKCE
func (s *OIDCAuthorizationService) validatePKCE(codeChallenge, codeChallengeMethod, codeVerifier string) error {
	var challengeVerifier string

	switch codeChallengeMethod {
	case "plain":
		challengeVerifier = codeVerifier
	case "S256", "":
		hash := sha256.Sum256([]byte(codeVerifier))
		challengeVerifier = base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
	default:
		return errors.New("unsupported_code_challenge_method")
	}

	if challengeVerifier != codeChallenge {
		return errors.New("invalid_code_verifier")
	}

	return nil
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

// generateTokens 生成令牌
func (s *OIDCAuthorizationService) generateTokens(client *model.OIDCClient, userID uint, scopes []string, nonce string) (*dto.TokenResponse, error) {
	// 生成访问令牌
	accessToken := s.generateAccessToken()
	refreshToken := s.generateRefreshToken()

	// 创建令牌记录
	token := &model.OIDCToken{
		AccessToken:  accessToken,
		TokenType:    model.TokenTypeBearer,
		ClientID:     client.ClientID,
		UserID:       userID,
		Scopes:       scopes,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(client.TokenLifetime) * time.Second),
	}

	// 生成ID令牌（如果作用域包含openid）
	if s.hasScope(scopes, "openid") {
		user, err := s.userService.GetUserModelByID(userID)
		if err != nil {
			return nil, err
		}

		idToken, err := s.generateIDToken(client, user, scopes, nonce)
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

// generateIDToken 生成ID令牌
func (s *OIDCAuthorizationService) generateIDToken(client *model.OIDCClient, user *model.User, scopes []string, nonce string) (string, error) {
	// 这里需要实现JWT ID令牌的生成
	// 暂时返回一个占位符
	return "id_token_placeholder", nil
}

// 辅助方法

func (s *OIDCAuthorizationService) generateAuthorizationCode() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (s *OIDCAuthorizationService) generateAccessToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

func (s *OIDCAuthorizationService) generateRefreshToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

func (s *OIDCAuthorizationService) parseScopes(scopeStr string) []string {
	if scopeStr == "" {
		return []string{}
	}
	return strings.Split(scopeStr, " ")
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
