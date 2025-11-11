package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
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
		// 对于授权端点的错误，需要重定向到客户端
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

	// 2. 创建OIDC会话
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
	)
	if err != nil {
		oc.redirectWithError(c, req.RedirectURI, "server_error", "创建会话失败", req.State)
		return
	}

	// 3. 检查用户是否已登录（从session或cookie中获取）
	// 这里简化处理，检查是否有userId的查询参数（用于测试）
	userIDStr := c.Query("userId")
	if userIDStr != "" {
		// 用户已提供ID（测试用途），直接标记为已认证
		userID, err := strconv.ParseUint(userIDStr, 10, 32)
		if err == nil {
			sessionService.MarkAsAuthenticated(session.SessionID, uint(userID))
			// 重定向到授权确认页面
			consentURL := "/auth/#/oidc/consent?session_id=" + session.SessionID
			c.Redirect(http.StatusFound, consentURL)
			return
		}
	}

	// 4. 用户未登录，重定向到登录页面
	loginURL := "/auth/#/oidc/login?session_id=" + session.SessionID
	c.Redirect(http.StatusFound, loginURL)
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
		response, err = authService.ExchangeClientCredentials(&req)
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

	// 支持通过 client_id 查询参数指定应用
	clientID := c.Param("client_id")

	var jwks []gin.H

	if clientID != "" {
		// 为特定应用返回 JWKS
		jwk, err := oc.getJWKSForApplication(db, clientID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":             "server_error",
				"error_description": "无法获取密钥",
			})
			return
		}
		if jwk != nil {
			jwks = append(jwks, jwk)
		}
	} else {
		// 返回所有应用的公钥（多租户模式）
		// 这里可以选择返回所有公钥，或者要求必须指定 client_id
		// 为安全起见，返回空集合，要求指定 client_id
		jwks = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{
		"keys": jwks,
	})
}

// getJWKSForApplication 获取指定应用的 JWK
func (oc OIDCController) getJWKSForApplication(db *gorm.DB, clientID string) (map[string]any, error) {
	// 1. 获取应用
	appService := service.NewApplicationService(db)
	app, err := appService.GetApplicationByClientID(clientID)
	if err != nil {
		return nil, err
	}

	// 2. 获取应用的主密钥管理器
	keyService := service.NewApplicationKeyService(db)
	keyManager, err := keyService.GetPrimaryKeyManagerForApplication(app.ID)
	if err != nil {
		return nil, err
	}

	// 3. 获取 JWK（对称加密算法会返回 nil）
	jwk, err := keyManager.GetJWK()
	if err != nil {
		return nil, err
	}

	return jwk, nil
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
