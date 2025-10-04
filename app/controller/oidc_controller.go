package controller

import (
	"net/http"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
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

	// 获取当前登录用户ID（这里应该从认证中间件或会话中获取）
	// 暂时使用查询参数模拟
	userIDStr := c.Query("userId")
	if userIDStr == "" {
		// 重定向到登录页面，并在登录后返回
		loginURL := "/login?redirect_uri=" + c.Request.URL.String()
		c.Redirect(http.StatusFound, loginURL)
		return
	}

	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		oc.redirectWithError(c, req.RedirectURI, "invalid_request", "用户ID格式错误", req.State)
		return
	}

	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
	response, err := authService.ProcessAuthorize(&req, uint(userID))
	if err != nil {
		oc.Error(c, constants.ErrCodeInternal, "处理授权请求失败: "+err.Error())
		return
	}

	// 检查是否有错误
	if response.Error != "" {
		oc.redirectWithError(c, req.RedirectURI, response.Error, response.ErrorDescription, req.State)
		return
	}

	// 成功响应
	redirectURL := req.RedirectURI + "?code=" + response.Code
	if response.State != "" {
		redirectURL += "&state=" + response.State
	}

	c.Redirect(http.StatusFound, redirectURL)
}

// Token OIDC令牌端点
// POST /oidc/token
func (oc OIDCController) Token(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.TokenRequest
	if err := c.ShouldBind(&req); err != nil {
		oc.tokenError(c, "invalid_request", "请求参数错误: "+err.Error())
		return
	}

	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())

	var response *dto.TokenResponse
	var err error

	switch req.GrantType {
	case "authorization_code":
		response, err = authService.ExchangeCodeForToken(&req)
	case "refresh_token":
		response, err = authService.RefreshToken(&req)
	default:
		oc.tokenError(c, "unsupported_grant_type", "不支持的授权类型")
		return
	}

	if err != nil {
		// 解析错误类型
		errorCode := err.Error()
		errorDescription := oc.getTokenErrorDescription(errorCode)
		oc.tokenError(c, errorCode, errorDescription)
		return
	}

	c.JSON(http.StatusOK, response)
}

// UserInfo OIDC用户信息端点
// GET /oidc/userinfo
func (oc OIDCController) UserInfo(c *gin.Context, helper interfaces.HelperInterface) {
	// 从Authorization头获取访问令牌
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}

	accessToken := parts[1]

	userInfoService := service.NewOIDCUserInfoService(helper.GetDatabase())
	response, err := userInfoService.GetUserInfo(accessToken)
	if err != nil {
		errorCode := err.Error()
		statusCode := http.StatusUnauthorized
		if errorCode == "insufficient_scope" {
			statusCode = http.StatusForbidden
		}
		c.JSON(statusCode, gin.H{"error": errorCode})
		return
	}

	c.JSON(http.StatusOK, response)
}

// Discovery OIDC发现文档端点
// GET /.well-known/openid-configuration
func (oc OIDCController) Discovery(c *gin.Context, helper interfaces.HelperInterface) {
	// 构建基础URL
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	baseURL := scheme + "://" + c.Request.Host

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
	// 这里应该返回用于验证JWT的公钥集合
	// 暂时返回一个占位符
	jwks := gin.H{
		"keys": []gin.H{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": "default",
				"n":   "placeholder_n",
				"e":   "AQAB",
			},
		},
	}

	c.JSON(http.StatusOK, jwks)
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
func (oc OIDCController) tokenError(c *gin.Context, error, errorDescription string) {
	c.JSON(http.StatusBadRequest, dto.TokenErrorResponse{
		Error:            error,
		ErrorDescription: errorDescription,
	})
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
