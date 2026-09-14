package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"gorm.io/gorm"
)

// OIDCController OIDC控制器
type OIDCController struct {
	BaseResponse
}

// Authorize OIDC授权端点
// GET /oidc/authorize
func (oc OIDCController) Authorize(c httpInterfaces.RouterContextInterface) {
	var req dto.AuthorizeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		// redirect_uri 尚未完成客户端精确匹配校验时绝不能用于重定向。
		oc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 1. 验证客户端应用
	appService := service.NewApplicationService(helper.GetDatabase())
	app, err := appService.GetApplicationByClientID(req.ClientID)
	if err != nil {
		oc.Error(c, constants.ErrCodeBadRequest, "无效的客户端")
		return
	}
	issuer := utils.GetBaseURL(c, fmt.Sprintf("/oidc/%s", req.ClientID))
	if err := validateOIDCRedirectURI(app, req.RedirectURI, issuer); err != nil {
		oc.Error(c, constants.ErrCodeBadRequest, "无效的重定向URI")
		return
	}
	prompt, err := normalizeAndValidateOIDCPrompt(req.Prompt)
	if err != nil {
		oc.redirectWithError(c, req.RedirectURI, "invalid_request", err.Error(), req.State)
		return
	}
	req.Prompt = prompt

	// 2. 创建 OIDC 会话（携带 prompt 参数）
	rctx := c.Request().Context()
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.CreateSession(
		rctx,
		req.ClientID,
		req.RedirectURI,
		req.Scope,
		req.State,
		req.Nonce,
		req.ResponseType,
		req.CodeChallenge,
		req.CodeChallengeMethod,
		req.Prompt,
	)
	if err != nil {
		helper.GetLogger().Error(fmt.Sprintf("创建会话失败 %s -> %v", err.Error(), err))
		oc.redirectWithError(c, req.RedirectURI, "server_error", "创建会话失败", req.State)
		return
	}

	if err := markOIDCRegistrationUnavailable(rctx, sessionService, app, session); err != nil {
		_ = sessionService.DeleteSession(rctx, session.SessionID)
		oc.redirectWithError(c, req.RedirectURI, "server_error", "记录注册状态失败", req.State)
		return
	}
	if err := captureOIDCRegistrationCandidate(c, sessionService, app, session); err != nil {
		_ = sessionService.DeleteSession(rctx, session.SessionID)
		oc.redirectWithError(c, req.RedirectURI, "server_error", "读取当前登录状态失败", req.State)
		return
	}

	// 3. prompt=none：不允许显示任何 UI。组合冲突已在创建会话前校验。
	if promptHas(req.Prompt, "none") {
		if ok := oc.trySSO(c, sessionService, session, req, true); ok {
			return
		}
		sessionService.DeleteSession(rctx, session.SessionID)
		oc.redirectWithError(c, req.RedirectURI, "login_required", "用户未登录", req.State)
		return
	}

	// 4. prompt=login 强制重新认证、prompt=create 直达注册，其余尝试 SSO 自动认证（不中断交互式流程，仅标记会话）
	if !promptBypassesSSO(req.Prompt) {
		if oc.trySSO(c, sessionService, session, req, false) {
			return
		}
	}

	loginTicket, err := sessionService.CreateLoginTicket(rctx, session)
	if err != nil {
		_ = sessionService.DeleteSession(rctx, session.SessionID)
		oc.redirectWithError(c, req.RedirectURI, "server_error", "创建浏览器登录票据失败", req.State)
		return
	}
	c.Redirect(http.StatusFound, oidcLoginURL(loginTicket, session.SessionID))
}

// promptHas 判断空格分隔的 prompt 参数中是否包含指定值（OIDC Core §3.1.2.1）
func promptHas(prompt, value string) bool {
	for _, p := range strings.Fields(prompt) {
		if p == value {
			return true
		}
	}
	return false
}

func promptBypassesSSO(prompt string) bool {
	return promptHas(prompt, "login") || promptHas(prompt, "create")
}

// normalizeAndValidateOIDCPrompt validates the space-delimited, case-sensitive
// prompt parameter and returns a stable representation for the session.
func normalizeAndValidateOIDCPrompt(prompt string) (string, error) {
	values := strings.Fields(prompt)
	if len(values) == 0 {
		return "", nil
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !service.SupportsOIDCPromptValue(value) {
			return "", fmt.Errorf("不支持的 prompt 值: %s", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	if _, ok := seen["none"]; ok && len(normalized) > 1 {
		return "", fmt.Errorf("prompt=none 不能与其他值组合")
	}
	if _, ok := seen["create"]; ok && len(normalized) > 1 {
		return "", fmt.Errorf("prompt=create 不能与其他值组合")
	}

	return strings.Join(normalized, " "), nil
}

// Token OIDC令牌端点
// POST /oidc/token
func (oc OIDCController) Token(c httpInterfaces.RouterContextInterface) {
	appUuid := c.Param("client_id")
	var req dto.TokenRequest
	if err := c.ShouldBind(&req); err != nil {
		oc.tokenError(c, "invalid_request", "请求参数错误: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 支持HTTP Basic认证
	// 根据 RFC 6749 Section 2.3.1，客户端可以通过HTTP Basic认证或请求体传递凭据
	if req.ClientID == "" || req.ClientSecret == "" {
		clientID, clientSecret, hasAuth := c.Request().BasicAuth()
		if hasAuth {
			req.ClientID = clientID
			req.ClientSecret = clientSecret
		}
	}

	// 验证必需的客户端凭据（除了public客户端的某些流程）
	if req.ClientID == "" {
		oc.tokenErrorWithAuth(c, "invalid_client", "缺少客户端凭据")
		return
	}

	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())

	// 获取issuer，确保与Discovery端点一致
	issuer := utils.GetBaseURL(c, fmt.Sprintf("/oidc/%s", appUuid))

	var response *dto.TokenResponse
	var err error

	switch req.GrantType {
	case "authorization_code":
		response, err = authService.ExchangeCodeForToken(&req, issuer)
	case "refresh_token":
		response, err = authService.RefreshToken(&req, issuer)
	case "client_credentials":
		response, err = authService.ExchangeClientCredentials(&req, issuer)
	default:
		oc.tokenError(c, "unsupported_grant_type", "不支持的授权类型", http.StatusBadRequest)
		return
	}

	if err != nil {
		// 解析错误类型并返回适当的状态码
		errorCode := err.Error()
		statusCode := oc.getStatusCodeForError(errorCode)
		errorDescription := oc.getTokenErrorDescription(errorCode)

		if statusCode == http.StatusUnauthorized {
			oc.tokenErrorWithAuth(c, errorCode, errorDescription)
		} else {
			oc.tokenError(c, errorCode, errorDescription, statusCode)
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// UserInfo OIDC用户信息端点
// GET/POST /oidc/userinfo
// 支持 GET (token 在 Authorization 头) 和 POST (token 在请求体或头)
func (oc OIDCController) UserInfo(c httpInterfaces.RouterContextInterface) {
	// 1. 获取访问令牌
	accessToken := oc.extractAccessToken(c)
	if accessToken == "" {
		oc.userInfoError(c, "invalid_token", "缺少或无效的访问令牌")
		return
	}

	// 2. 获取用户信息
	userInfoService := service.NewOIDCUserInfoService(helper.GetDatabase())
	response, err := userInfoService.GetUserInfo(accessToken)
	if err != nil {
		errorCode := err.Error()
		errorDescription := oc.getUserInfoErrorDescription(errorCode)

		// 根据错误类型返回不同状态码
		if errorCode == "insufficient_scope" {
			// RFC 6750 §3: scope 不足返回 403 + WWW-Authenticate
			c.SetHeader("WWW-Authenticate", fmt.Sprintf(
				`Bearer realm="oidc", error="insufficient_scope", error_description=%q, scope="openid"`,
				errorDescription,
			))
			c.JSON(http.StatusForbidden, map[string]any{
				"error":             errorCode,
				"error_description": errorDescription,
			})
		} else {
			oc.userInfoError(c, errorCode, errorDescription)
		}
		return
	}

	// 站内相对头像路径（/attachments/{uuid}）转为绝对地址
	if strings.HasPrefix(response.Picture, "/") {
		response.Picture = utils.GetBaseURL(c, response.Picture)
	}

	c.JSON(http.StatusOK, response)
}

// extractAccessToken 从请求中提取访问令牌
// 支持 Authorization 头 (Bearer token) 和请求体 (POST)
func (oc OIDCController) extractAccessToken(c httpInterfaces.RouterContextInterface) string {
	// 1. 优先从 Authorization 头获取
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return strings.TrimSpace(parts[1])
		}
	}

	// 2. 如果是 POST 请求，尝试从请求体获取
	if c.Method() == "POST" {
		token := c.PostForm("access_token")
		if token != "" {
			return strings.TrimSpace(token)
		}
	}

	return ""
}

// userInfoError 返回 UserInfo 端点的错误响应（RFC 6750 §3）
// 统一 realm 为 "oidc"；error_description 用 %q 转义双引号
func (oc OIDCController) userInfoError(c httpInterfaces.RouterContextInterface, errCode, errorDescription string) {
	wwwAuth := fmt.Sprintf(`Bearer realm="oidc", error=%q`, errCode)
	if errorDescription != "" {
		wwwAuth += fmt.Sprintf(`, error_description=%q`, errorDescription)
	}
	c.SetHeader("WWW-Authenticate", wwwAuth)

	c.JSON(http.StatusUnauthorized, map[string]any{
		"error":             errCode,
		"error_description": errorDescription,
	})
}

// getUserInfoErrorDescription 获取 UserInfo 错误描述
func (oc OIDCController) getUserInfoErrorDescription(errorCode string) string {
	switch errorCode {
	case "invalid_token":
		return "访问令牌无效或已过期"
	case "insufficient_scope":
		return "权限不足，需要 openid scope"
	case "user_not_found":
		return "用户不存在"
	default:
		return "未知错误"
	}
}

// Discovery OIDC发现文档端点
// GET /.well-known/openid-configuration
func (oc OIDCController) Discovery(c httpInterfaces.RouterContextInterface) {
	clientId := c.Param("client_id")
	// 构建基础URL，支持反向代理场景
	baseURL := utils.GetBaseURL(c, fmt.Sprintf("/oidc/%s", clientId))

	discoveryService := service.NewOIDCDiscoveryService(baseURL)
	response := discoveryService.GetDiscoveryDocument()

	c.JSON(http.StatusOK, response)
}

// Introspect 令牌内省端点
// POST /oidc/introspect
func (oc OIDCController) Introspect(c httpInterfaces.RouterContextInterface) {
	var req dto.IntrospectionRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]any{
			"error":             "invalid_request",
			"error_description": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 支持 HTTP Basic 认证（RFC 7662 §2.1 要求 token endpoint 的认证方式）
	if req.ClientID == "" || req.ClientSecret == "" {
		if id, secret, ok := c.Request().BasicAuth(); ok {
			req.ClientID = id
			req.ClientSecret = secret
		}
	}

	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
	response, err := authService.IntrospectToken(&req)
	if err != nil {
		c.SetHeader("WWW-Authenticate", `Basic realm="oidc"`)
		c.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_client",
			"error_description": "内省端点必须携带有效的客户端凭据",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// Revoke 令牌撤销端点
// POST /oidc/revoke
func (oc OIDCController) Revoke(c httpInterfaces.RouterContextInterface) {
	var req dto.RevocationRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]any{
			"error":             "invalid_request",
			"error_description": "请求参数错误: " + err.Error(),
		})
		return
	}

	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
	err := authService.RevokeToken(&req)
	if err != nil {
		errorCode := err.Error()
		statusCode := http.StatusBadRequest
		if errorCode == "invalid_client" {
			statusCode = http.StatusUnauthorized
		}

		c.JSON(statusCode, map[string]any{
			"error":             errorCode,
			"error_description": oc.getRevocationErrorDescription(errorCode),
		})
		return
	}

	// RFC 7009: 成功的撤销请求返回HTTP 200
	c.Status(http.StatusOK)
}

// JWKS JSON Web Key Set端点
// GET /oidc/jwks
func (oc OIDCController) JWKS(c httpInterfaces.RouterContextInterface) {
	db := helper.GetDatabase()

	// 要求必须提供 client_id 参数
	clientID := c.Param("client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, map[string]any{
			"error":             "invalid_request",
			"error_description": "缺少必需的 client_id 参数",
		})
		return
	}

	// 获取应用的所有有效密钥的 JWKS
	jwks, err := oc.getJWKSArrayForApplication(db, clientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{
			"error":             "server_error",
			"error_description": "无法获取密钥",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]any{
		"keys": jwks,
	})
}

// getJWKSArrayForApplication 获取指定应用的所有有效密钥的 JWK 数组
func (oc OIDCController) getJWKSArrayForApplication(db *gorm.DB, clientID string) ([]map[string]any, error) {
	// 1. 获取应用
	appService := service.NewApplicationService(db)
	app, err := appService.GetApplicationByClientID(clientID)
	if err != nil {
		return nil, err
	}

	// 2. 获取应用的所有有效密钥
	keyService := service.NewApplicationKeyService(db)
	keys, err := keyService.ListValidKeysByApplication(app.ID)
	if err != nil {
		return nil, err
	}

	// 3. 为每个密钥生成 JWK
	var jwks []map[string]any
	for _, key := range keys {
		// 加载密钥管理器
		keyManager, err := keyService.LoadKeyManager(key)
		if err != nil {
			continue // 跳过加载失败的密钥
		}

		// 获取 JWK（对称加密算法会返回 nil）
		jwk, err := keyManager.GetJWK()
		if err != nil {
			continue // 跳过生成失败的密钥
		}

		// 只添加非空的 JWK（跳过对称加密算法）
		if jwk != nil {
			jwks = append(jwks, jwk)
		}
	}

	return jwks, nil
}

// 私有方法

// trySSO reuses a valid browser login for another authorization transaction.
// It accepts the IAM cookie or its associated SSO refresh credential, rotates
// the browser refresh cookie atomically, and preserves the login session ID.
// silent completes authorization directly; otherwise the UI handles consent.
func (oc OIDCController) trySSO(
	c httpInterfaces.RouterContextInterface,
	sessionService *service.OIDCSessionService,
	session *service.OIDCSession,
	req dto.AuthorizeRequest,
	silent bool,
) bool {
	rctx := c.Request().Context()
	db := helper.GetDatabase()
	var loginSID string
	var userID uint
	for _, name := range []string{middleware.UserCookieName, middleware.AdminCookieName} {
		if raw, err := c.Cookie(name); err == nil {
			if claims, err := middleware.ValidateUserSessionToken(raw); err == nil {
				loginSID, userID = claims.SessionID, claims.UserID
				break
			}
		}
	}
	refreshToken, _ := c.Cookie("refresh_token")
	oldToken, _ := dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(refreshToken)
	if loginSID == "" && oldToken != nil {
		loginSID, userID = oldToken.LoginSessionID, oldToken.UserID
	}
	if loginSID == "" {
		return false
	}
	if oldToken != nil && oldToken.LoginSessionID != loginSID {
		oldToken = nil
	}
	userService := service.NewUserService(db)
	userModel, userErr := userService.GetUserModelByID(userID)
	if userErr != nil || userModel == nil || !userModel.CanLogin() {
		return false
	}
	user, userErr := userService.GetUserByID(userID)
	if userErr != nil || user == nil {
		return false
	}
	session.LoginSessionID = loginSID

	// 在旋转全局 SSO Cookie、认证事务或签发新 Token 前检查目标应用范围。
	_, failure, accessErr := checkOIDCUserAccess(session, userID)
	if accessErr != nil {
		if silent {
			sessionService.DeleteSession(rctx, session.SessionID)
			oc.redirectWithError(c, req.RedirectURI, "server_error", "检查应用访问权限失败", req.State)
			return true
		}
		failure, failureErr := newOIDCFailure(session, "server_error", "检查应用访问权限失败")
		if failureErr == nil {
			_ = markOIDCSessionFailed(rctx, sessionService, session.SessionID, failure)
		}
		return false
	}
	if failure != nil {
		if silent {
			sessionService.DeleteSession(rctx, session.SessionID)
			oc.redirectWithError(c, req.RedirectURI, failure.Code, failure.Description, req.State)
			return true
		}
		_ = markOIDCSessionFailed(rctx, sessionService, session.SessionID, failure)
		return false
	}

	// 检查补充信息
	authFlowCtrl := OIDCAuthFlowController{}
	missingFields := authFlowCtrl.checkMissingUserInfo(req.Scope, user)
	if len(missingFields) > 0 {
		if silent {
			sessionService.DeleteSession(rctx, session.SessionID)
			oc.redirectWithError(c, req.RedirectURI, "interaction_required", "需要用户补充信息", req.State)
			return true
		}
		return false
	}

	if err := sessionService.MarkAsAuthenticated(rctx, session.SessionID, userID, loginSID); err != nil {
		return false
	}
	previousRefresh := ""
	if oldToken != nil {
		previousRefresh = oldToken.RefreshToken
	}
	if err := oc.reissueSSOCookie(c, session, userID, previousRefresh); err != nil {
		_ = sessionService.ResetAuthentication(rctx, session.SessionID)
		return false
	}

	if !silent {
		helper.GetLogger().Info(fmt.Sprintf("SSO: 用户 %d 通过 refresh_token 自动认证并旋转", userID))
		// 旧 token 已在上方旋转撤销，必须在此立即续发新 cookie：
		// 后续的 Consent 接口不会写 cookie，若不在这里续发，
		// 浏览器将持有已撤销的凭证，下次访问被迫重新输入密码
		return false
	}

	// silent 模式：直接完成授权码签发并重定向
	if err := sessionService.MarkAsConsentGranted(rctx, session.SessionID); err != nil {
		sessionService.DeleteSession(rctx, session.SessionID)
		oc.redirectWithError(c, req.RedirectURI, "server_error", "更新会话失败", req.State)
		return true
	}

	authService := service.NewOIDCAuthorizationService(db)
	issuer := utils.GetBaseURL(c, fmt.Sprintf("/oidc/%s", session.ClientID))

	authReq := &dto.AuthorizeRequest{
		LoginSessionID:      loginSID,
		ResponseType:        session.ResponseType,
		ClientID:            session.ClientID,
		RedirectURI:         session.RedirectURI,
		Scope:               session.Scope,
		State:               session.State,
		Nonce:               session.Nonce,
		CodeChallenge:       session.CodeChallenge,
		CodeChallengeMethod: session.CodeChallengeMethod,
	}
	authResponse, authErr := authService.ProcessAuthorize(authReq, userID, issuer)
	sessionService.DeleteSession(rctx, session.SessionID)
	if authErr != nil {
		oc.redirectWithError(c, req.RedirectURI, "server_error", "处理授权失败", req.State)
		return true
	}
	if authResponse.Error != "" {
		oc.redirectWithError(c, req.RedirectURI, authResponse.Error, authResponse.ErrorDescription, req.State)
		return true
	}

	// 续发新的 IAM 内部会话 refresh_token cookie（独立于 RP 的 token）

	redirectURL := session.RedirectURI
	sep := "?"
	if strings.Contains(redirectURL, "?") {
		sep = "&"
	}
	q := url.Values{}
	q.Set("code", authResponse.Code)
	if session.State != "" {
		q.Set("state", session.State)
	}
	c.Redirect(http.StatusFound, redirectURL+sep+q.Encode())
	return true
}

// reissueSSOCookie issues the browser SSO refresh credential within the same
// persistent login. Rotation failures abort SSO rather than creating an
// authenticated transaction with unusable credentials.
func (oc OIDCController) reissueSSOCookie(
	c httpInterfaces.RouterContextInterface,
	session *service.OIDCSession,
	userID uint,
	previousRefresh string,
) error {
	db := helper.GetDatabase()
	appService := service.NewApplicationService(db)
	app, err := appService.GetApplicationByClientID(session.ClientID)
	if err != nil || app == nil {
		helper.GetLogger().Error(fmt.Sprintf("SSO: 续发 cookie 时读取应用失败 client=%s err=%v", session.ClientID, err))
		return fmt.Errorf("无法续发 SSO 登录凭据")
	}

	authService := service.NewOIDCAuthorizationService(db)
	issuer := utils.GetBaseURL(c, fmt.Sprintf("/oidc/%s", session.ClientID))
	scopes := authService.ParseScopes(session.Scope)
	newResp, err := authService.GenerateSSOTokens(app, userID, scopes, session.Nonce, issuer, session.LoginSessionID, previousRefresh)
	if err != nil || newResp == nil {
		helper.GetLogger().Error(fmt.Sprintf("SSO: 续发 refresh_token 失败 user=%d err=%v", userID, err))
		return fmt.Errorf("无法续发 SSO 登录凭据")
	}

	isSecure := c.Request().TLS != nil
	// SameSite 必须在 SetCookie 之前调用才能生效
	if isSecure {
		c.SetSameSite(http.SameSiteNoneMode)
	} else {
		c.SetSameSite(http.SameSiteLaxMode)
	}
	c.SetCookie("refresh_token", newResp.RefreshToken, 7*24*60*60, "/", "", isSecure, true)
	return nil
}

// redirectWithError 重定向并携带错误信息（RFC 6749 §4.1.2.1）
// 所有参数均做 URL 编码，避免特殊字符破坏 query string 或触发重定向污染
func (oc OIDCController) redirectWithError(c httpInterfaces.RouterContextInterface, redirectURI, errorCode, errorDescription, state string) {
	if redirectURI == "" {
		c.JSON(http.StatusBadRequest, map[string]any{
			"error":             errorCode,
			"error_description": errorDescription,
		})
		return
	}

	sep := "?"
	if strings.Contains(redirectURI, "?") {
		sep = "&"
	}
	q := url.Values{}
	q.Set("error", errorCode)
	if errorDescription != "" {
		q.Set("error_description", errorDescription)
	}
	if state != "" {
		q.Set("state", state)
	}
	c.Redirect(http.StatusFound, redirectURI+sep+q.Encode())
}

// tokenError 返回令牌错误响应
func (oc OIDCController) tokenError(c httpInterfaces.RouterContextInterface, error, errorDescription string, statusCode int) {
	c.JSON(statusCode, dto.TokenErrorResponse{
		Error:            error,
		ErrorDescription: errorDescription,
	})
}

// tokenErrorWithAuth 返回带 WWW-Authenticate 头的 401 错误（RFC 6750 §3）
// realm 统一为 "oidc"，避免多个端点各自一个 realm 造成规范混乱
func (oc OIDCController) tokenErrorWithAuth(c httpInterfaces.RouterContextInterface, errCode, errorDescription string) {
	wwwAuth := fmt.Sprintf(`Basic realm="oidc", error=%q`, errCode)
	if errorDescription != "" {
		wwwAuth += fmt.Sprintf(`, error_description=%q`, errorDescription)
	}
	c.SetHeader("WWW-Authenticate", wwwAuth)
	c.JSON(http.StatusUnauthorized, dto.TokenErrorResponse{
		Error:            errCode,
		ErrorDescription: errorDescription,
	})
}

// getStatusCodeForError 根据错误代码返回HTTP状态码
func (oc OIDCController) getStatusCodeForError(errorCode string) int {
	switch errorCode {
	case "invalid_client":
		return http.StatusUnauthorized
	case "server_error":
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

// getTokenErrorDescription 获取令牌错误描述
func (oc OIDCController) getTokenErrorDescription(errorCode string) string {
	switch errorCode {
	case "invalid_request":
		return "请求格式错误或缺少必需参数"
	case "invalid_client":
		return "客户端认证失败"
	case "invalid_grant":
		return "授权码无效、过期或已使用"
	case "unauthorized_client":
		return "客户端未被授权使用此授权类型"
	case "unsupported_grant_type":
		return "不支持的授权类型"
	case "invalid_scope":
		return "请求的作用域无效或超出范围"
	case "server_error":
		return "服务器内部错误"
	default:
		return "未知错误"
	}
}

// getRevocationErrorDescription 获取撤销错误描述
func (oc OIDCController) getRevocationErrorDescription(errorCode string) string {
	switch errorCode {
	case "invalid_request":
		return "请求格式错误或缺少必需参数"
	case "invalid_client":
		return "客户端认证失败"
	case "invalid_grant":
		return "令牌无效或不属于该客户端"
	case "unsupported_token_type":
		return "不支持的令牌类型"
	case "server_error":
		return "服务器内部错误"
	default:
		return "未知错误"
	}
}
