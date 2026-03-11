package controller

import (
	"fmt"
	"net/http"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OIDCController OIDC控制器
type OIDCController struct {
	BaseResponse
}

// Authorize OIDC授权端点
// GET /oidc/authorize
func (oc OIDCController) Authorize(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.AuthorizeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		if req.RedirectURI == "" {
			oc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
			return
		}
		oc.redirectWithError(c, req.RedirectURI, "invalid_request", "请求参数错误", req.State)
		return
	}

	// 1. 验证客户端应用
	appService := service.NewApplicationService(helper.GetDatabase())
	_, err := appService.GetApplicationByClientID(req.ClientID)
	if err != nil {
		oc.redirectWithError(c, req.RedirectURI, "invalid_client", "无效的客户端", req.State)
		return
	}

	// 2. 创建 OIDC 会话（携带 prompt 参数）
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.CreateSession(
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

	// 3. prompt=none：不允许显示任何 UI
	if req.Prompt == "none" {
		refreshToken, cookieErr := c.Cookie("refresh_token")
		if cookieErr == nil && refreshToken != "" {
			tokenDAO := dao.NewOIDCTokenDAO(helper.GetDatabase())
			oldToken, tokenErr := tokenDAO.GetValidByRefreshToken(refreshToken)
			if tokenErr == nil && oldToken != nil {
				userService := service.NewUserService(helper.GetDatabase())
				user, userErr := userService.GetUserByID(oldToken.UserID)
				if userErr == nil && user != nil {
					// 检查是否需要补充信息：需要 UI 则返回 interaction_required
					authFlowCtrl := OIDCAuthFlowController{}
					missingFields := authFlowCtrl.checkMissingUserInfo(req.Scope, user)
					if len(missingFields) > 0 {
						sessionService.DeleteSession(session.SessionID)
						oc.redirectWithError(c, req.RedirectURI, "interaction_required", "需要用户补充信息", req.State)
						return
					}

					// 静默完成授权：标记认证 + 授权 + 生成授权码
					if err := sessionService.MarkAsAuthenticated(session.SessionID, oldToken.UserID); err != nil {
						sessionService.DeleteSession(session.SessionID)
						oc.redirectWithError(c, req.RedirectURI, "server_error", "更新会话失败", req.State)
						return
					}
					if err := sessionService.MarkAsConsentGranted(session.SessionID); err != nil {
						sessionService.DeleteSession(session.SessionID)
						oc.redirectWithError(c, req.RedirectURI, "server_error", "更新会话失败", req.State)
						return
					}

					authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
					issuer := utils.GetBaseURL(c, helper, fmt.Sprintf("/oidc/%s", session.ClientID))
					authReq := &dto.AuthorizeRequest{
						ResponseType:        session.ResponseType,
						ClientID:            session.ClientID,
						RedirectURI:         session.RedirectURI,
						Scope:               session.Scope,
						State:               session.State,
						Nonce:               session.Nonce,
						CodeChallenge:       session.CodeChallenge,
						CodeChallengeMethod: session.CodeChallengeMethod,
					}
					authResponse, authErr := authService.ProcessAuthorize(authReq, oldToken.UserID, issuer)
					sessionService.DeleteSession(session.SessionID)
					if authErr != nil {
						oc.redirectWithError(c, req.RedirectURI, "server_error", "处理授权失败", req.State)
						return
					}
					if authResponse.Error != "" {
						oc.redirectWithError(c, req.RedirectURI, authResponse.Error, authResponse.ErrorDescription, req.State)
						return
					}

					redirectURL := session.RedirectURI + "?code=" + authResponse.Code
					if session.State != "" {
						redirectURL += "&state=" + session.State
					}
					c.Redirect(http.StatusFound, redirectURL)
					return
				}
			}
		}
		// 无有效 SSO 会话
		sessionService.DeleteSession(session.SessionID)
		oc.redirectWithError(c, req.RedirectURI, "login_required", "用户未登录", req.State)
		return
	}

	// 4. 非 prompt=login 时尝试 SSO 自动认证
	if req.Prompt != "login" {
		refreshToken, cookieErr := c.Cookie("refresh_token")
		if cookieErr == nil && refreshToken != "" {
			tokenDAO := dao.NewOIDCTokenDAO(helper.GetDatabase())
			oldToken, tokenErr := tokenDAO.GetValidByRefreshToken(refreshToken)
			if tokenErr == nil && oldToken != nil {
				userService := service.NewUserService(helper.GetDatabase())
				user, userErr := userService.GetUserByID(oldToken.UserID)
				if userErr == nil && user != nil {
					sessionService.MarkAsAuthenticated(session.SessionID, oldToken.UserID)
					helper.GetLogger().Info(fmt.Sprintf("SSO: 用户 %d 通过 refresh_token 自动认证", oldToken.UserID))
				}
			}
		}
	}

	// 5. 重定向到统一的前端授权入口（编排器）
	authorizeURL := "/auth/#/oidc/authorize?session_id=" + session.SessionID
	c.Redirect(http.StatusFound, authorizeURL)
}

// Token OIDC令牌端点
// POST /oidc/token
func (oc OIDCController) Token(c *gin.Context, helper interfaces.HelperInterface) {
	appUuid := c.Param("uuid")
	var req dto.TokenRequest
	if err := c.ShouldBind(&req); err != nil {
		oc.tokenError(c, "invalid_request", "请求参数错误: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 支持HTTP Basic认证
	// 根据 RFC 6749 Section 2.3.1，客户端可以通过HTTP Basic认证或请求体传递凭据
	if req.ClientID == "" || req.ClientSecret == "" {
		clientID, clientSecret, hasAuth := c.Request.BasicAuth()
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
	issuer := utils.GetBaseURL(c, helper, fmt.Sprintf("/oidc/%s", appUuid))

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
func (oc OIDCController) UserInfo(c *gin.Context, helper interfaces.HelperInterface) {
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
			// RFC 6750 Section 3: scope 不足返回 403 + WWW-Authenticate 头，必须包含 scope 参数
			c.Header("WWW-Authenticate", fmt.Sprintf(`Bearer realm="userinfo", error="insufficient_scope", error_description="%s", scope="openid"`, errorDescription))
			c.JSON(http.StatusForbidden, gin.H{
				"error":             errorCode,
				"error_description": errorDescription,
			})
		} else {
			// 其他错误返回 401
			oc.userInfoError(c, errorCode, errorDescription)
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

// extractAccessToken 从请求中提取访问令牌
// 支持 Authorization 头 (Bearer token) 和请求体 (POST)
func (oc OIDCController) extractAccessToken(c *gin.Context) string {
	// 1. 优先从 Authorization 头获取
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return strings.TrimSpace(parts[1])
		}
	}

	// 2. 如果是 POST 请求，尝试从请求体获取
	if c.Request.Method == "POST" {
		token := c.PostForm("access_token")
		if token != "" {
			return strings.TrimSpace(token)
		}
	}

	return ""
}

// userInfoError 返回 UserInfo 端点的错误响应
// 根据 RFC 6750 Section 3，必须包含 WWW-Authenticate 头
func (oc OIDCController) userInfoError(c *gin.Context, error, errorDescription string) {
	// 构建 WWW-Authenticate 头
	wwwAuth := `Bearer realm="userinfo", error="` + error + `"`
	if errorDescription != "" {
		wwwAuth += `, error_description="` + errorDescription + `"`
	}
	c.Header("WWW-Authenticate", wwwAuth)

	c.JSON(http.StatusUnauthorized, gin.H{
		"error":             error,
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
func (oc OIDCController) Discovery(c *gin.Context, helper interfaces.HelperInterface) {
	clientId := c.Param("client_id")
	// 构建基础URL，支持反向代理场景
	baseURL := utils.GetBaseURL(c, helper, fmt.Sprintf("/oidc/%s", clientId))

	discoveryService := service.NewOIDCDiscoveryService(baseURL)
	response := discoveryService.GetDiscoveryDocument()

	c.JSON(http.StatusOK, response)
}

// Introspect 令牌内省端点
// POST /oidc/introspect
func (oc OIDCController) Introspect(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.IntrospectionRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "请求参数错误: " + err.Error(),
		})
		return
	}

	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
	response, err := authService.IntrospectToken(&req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":             "invalid_client",
			"error_description": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// Revoke 令牌撤销端点
// POST /oidc/revoke
func (oc OIDCController) Revoke(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.RevocationRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
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

		c.JSON(statusCode, gin.H{
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
func (oc OIDCController) JWKS(c *gin.Context, helper interfaces.HelperInterface) {
	db := helper.GetDatabase()

	// 要求必须提供 client_id 参数
	clientID := c.Param("client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "缺少必需的 client_id 参数",
		})
		return
	}

	// 获取应用的所有有效密钥的 JWKS
	jwks, err := oc.getJWKSArrayForApplication(db, clientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": "无法获取密钥",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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

// redirectWithError 重定向并携带错误信息
func (oc OIDCController) redirectWithError(c *gin.Context, redirectURI, error, errorDescription, state string) {
	url := redirectURI + "?error=" + error + "&error_description=" + errorDescription
	if state != "" {
		url += "&state=" + state
	}
	c.Redirect(http.StatusFound, url)
}

// tokenError 返回令牌错误响应
func (oc OIDCController) tokenError(c *gin.Context, error, errorDescription string, statusCode int) {
	c.JSON(statusCode, dto.TokenErrorResponse{
		Error:            error,
		ErrorDescription: errorDescription,
	})
}

// tokenErrorWithAuth 返回带WWW-Authenticate头的401错误
// 根据 RFC 6750 Section 3，当客户端认证失败时必须包含此头
func (oc OIDCController) tokenErrorWithAuth(c *gin.Context, error, errorDescription string) {
	c.Header("WWW-Authenticate", `Bearer realm="token"`)
	c.JSON(http.StatusUnauthorized, dto.TokenErrorResponse{
		Error:            error,
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
