package controller

import (
	"fmt"
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// OIDCAuthFlowController OIDC授权流程控制器
type OIDCAuthFlowController struct {
	BaseResponse
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	SessionID string `json:"sessionId" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	NextURL  string `json:"nextUrl,omitempty"`
	UserInfo *struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		RealName string `json:"realName"`
	} `json:"userInfo,omitempty"`
}

// ConsentRequest 授权确认请求
type ConsentRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
	Approved  bool   `json:"approved"`
}

// ConsentResponse 授权确认响应
type ConsentResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	RedirectURL string `json:"redirectUrl,omitempty"`
}

// AuthInfoResponse 授权信息响应
type AuthInfoResponse struct {
	SessionID string `json:"sessionId"`
	Client    struct {
		ClientID    string `json:"clientId"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"client"`
	Scopes []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"scopes"`
	User struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		RealName string `json:"realName"`
	} `json:"user"`
}

// Login OIDC登录接口
// POST /api/oidc/login
func (c OIDCAuthFlowController) Login(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 1. 验证会话是否存在
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	_, err := sessionService.GetSession(req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}

	// 2. 验证用户凭据
	userService := service.NewUserService(helper.GetDatabase())
	user, err := userService.ValidateUserForLogin(req.Username, req.Password)
	if err != nil {
		response := LoginResponse{
			Success: false,
			Message: "用户名或密码错误",
		}
		ctx.JSON(http.StatusUnauthorized, response)
		return
	}

	// 3. 标记会话为已认证
	if err := sessionService.MarkAsAuthenticated(req.SessionID, user.ID); err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "更新会话失败: "+err.Error())
		return
	}

	// 4. 返回成功响应，引导用户到授权确认页面
	response := LoginResponse{
		Success: true,
		Message: "登录成功",
		NextURL: "/auth/#/oidc/consent?session_id=" + req.SessionID,
		UserInfo: &struct {
			ID       uint   `json:"id"`
			Username string `json:"username"`
			RealName string `json:"realName"`
		}{
			ID:       user.ID,
			Username: user.Username,
			RealName: user.RealName,
		},
	}

	c.Success(ctx, response)
}

// Consent OIDC授权确认接口
// POST /api/oidc/consent
func (c OIDCAuthFlowController) Consent(ctx *gin.Context, helper interfaces.HelperInterface) {
	appUuid := ctx.Param("uuid")
	var req ConsentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 1. 获取会话
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.GetSession(req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}

	// 2. 检查会话是否已认证
	if !session.IsAuthenticated || session.UserID == nil {
		c.Error(ctx, constants.ErrCodeUnauthorized, "用户尚未认证")
		return
	}

	// 3. 处理用户拒绝授权的情况
	if !req.Approved {
		// 构建错误重定向URL
		redirectURL := session.RedirectURI + "?error=access_denied&error_description=User denied the request"
		if session.State != "" {
			redirectURL += "&state=" + session.State
		}

		// 删除会话
		sessionService.DeleteSession(req.SessionID)

		response := ConsentResponse{
			Success:     false,
			Message:     "用户拒绝授权",
			RedirectURL: redirectURL,
		}

		ctx.JSON(http.StatusOK, response)
		return
	}

	// 4. 标记会话为已授权
	if err := sessionService.MarkAsConsentGranted(req.SessionID); err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "更新会话失败: "+err.Error())
		return
	}

	// 5. 生成授权码
	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
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

	// 获取issuer，确保与Discovery端点一致
	issuer := utils.GetBaseURL(ctx, helper, fmt.Sprintf("/oidc/%s", appUuid))

	authResponse, err := authService.ProcessAuthorize(authReq, *session.UserID, issuer)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "处理授权请求失败: "+err.Error())
		return
	}

	// 6. 检查是否有错误
	if authResponse.Error != "" {
		redirectURL := session.RedirectURI + "?error=" + authResponse.Error
		if authResponse.ErrorDescription != "" {
			redirectURL += "&error_description=" + authResponse.ErrorDescription
		}
		if session.State != "" {
			redirectURL += "&state=" + session.State
		}

		// 删除会话
		sessionService.DeleteSession(req.SessionID)

		response := ConsentResponse{
			Success:     false,
			Message:     authResponse.ErrorDescription,
			RedirectURL: redirectURL,
		}

		ctx.JSON(http.StatusOK, response)
		return
	}

	// 7. 构建成功重定向URL
	redirectURL := session.RedirectURI + "?code=" + authResponse.Code
	if session.State != "" {
		redirectURL += "&state=" + session.State
	}

	// 删除会话（授权码已生成，会话不再需要）
	sessionService.DeleteSession(req.SessionID)

	response := ConsentResponse{
		Success:     true,
		Message:     "授权成功",
		RedirectURL: redirectURL,
	}

	c.Success(ctx, response)
}

// GetAuthInfo 获取授权信息接口
// GET /api/oidc/auth-info
func (c OIDCAuthFlowController) GetAuthInfo(ctx *gin.Context, helper interfaces.HelperInterface) {
	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		c.Error(ctx, constants.ErrCodeBadRequest, "缺少session_id参数")
		return
	}

	// 1. 获取会话
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.GetSession(sessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}

	// 2. 检查会话是否已认证
	if !session.IsAuthenticated || session.UserID == nil {
		c.Error(ctx, constants.ErrCodeUnauthorized, "用户尚未认证")
		return
	}

	// 3. 获取客户端信息
	appService := service.NewApplicationService(helper.GetDatabase())
	app, err := appService.GetApplicationByClientID(session.ClientID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "获取客户端信息失败: "+err.Error())
		return
	}

	// 4. 获取用户信息
	userService := service.NewUserService(helper.GetDatabase())
	user, err := userService.GetUserByID(*session.UserID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "获取用户信息失败: "+err.Error())
		return
	}

	// 5. 构建响应
	response := AuthInfoResponse{
		SessionID: sessionID,
	}

	response.Client.ClientID = session.ClientID
	response.Client.Name = app.Name
	response.Client.Description = app.Description

	// 解析scopes并提供描述
	response.Scopes = c.parseScopesWithDescription(session.Scope)

	response.User.ID = user.ID
	response.User.Username = user.Username
	response.User.RealName = user.RealName

	c.Success(ctx, response)
}

// 私有方法

// parseScopesWithDescription 解析scopes并返回带描述的列表
func (c OIDCAuthFlowController) parseScopesWithDescription(scopeStr string) []struct {
	Name        string `json:"name"`
	Description string `json:"description"`
} {
	scopeDescriptions := map[string]string{
		"openid":         "验证您的身份",
		"profile":        "访问您的基本资料（昵称、头像等）",
		"email":          "访问您的电子邮件地址",
		"phone":          "访问您的手机号码",
		"address":        "访问您的地址信息",
		"offline_access": "在您离线时访问您的数据",
	}

	scopes := []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}{}

	// 解析scope字符串
	scopeList := service.NewOIDCAuthorizationService(nil).ParseScopes(scopeStr)
	for _, scope := range scopeList {
		description, ok := scopeDescriptions[scope]
		if !ok {
			description = "访问 " + scope + " 权限"
		}
		scopes = append(scopes, struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}{
			Name:        scope,
			Description: description,
		})
	}

	return scopes
}
