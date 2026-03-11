package controller

import (
	"fmt"
	"net/http"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
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
	Username      string             `json:"username" binding:"required"`
	Password      string             `json:"password" binding:"required"`
	SessionID     string             `json:"sessionId" binding:"required"`
	CaptchaTicket *dto.CaptchaTicket `json:"captchaTicket,omitempty"` // 验证码凭证（可选）
	RememberMe    bool               `json:"rememberMe"`              // 记住我
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username      string             `json:"username" binding:"required"`
	Email         string             `json:"email" binding:"omitempty,email"`
	EmailCode     string             `json:"emailCode" binding:"omitempty"`
	Phone         string             `json:"phone" binding:"omitempty"`
	PhoneCode     string             `json:"phoneCode" binding:"omitempty"`
	Password      string             `json:"password" binding:"required,min=6"`
	RealName      string             `json:"realName" binding:"omitempty"`
	Nickname      string             `json:"nickname" binding:"omitempty"`
	SessionID     string             `json:"sessionId,omitempty"`
	CaptchaTicket *dto.CaptchaTicket `json:"captchaTicket,omitempty"` // 验证码凭证（可选）
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	UserID  uint   `json:"userId,omitempty"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Success                   bool     `json:"success"`
	Message                   string   `json:"message"`
	NextURL                   string   `json:"nextUrl,omitempty"`
	RequiresProfileCompletion bool     `json:"requiresProfileCompletion,omitempty"`
	MissingFields             []string `json:"missingFields,omitempty"`
	UserInfo                  *struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		RealName string `json:"realName"`
	} `json:"userInfo,omitempty"`
	AccessToken string `json:"accessToken,omitempty"` // 访问令牌
	ExpiresIn   int64  `json:"expiresIn,omitempty"`   // 过期时间（秒）
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

// SendVerificationCodeRequest 发送验证码请求
type SendVerificationCodeRequest struct {
	SessionID     string             `json:"sessionId" binding:"required"`
	Type          string             `json:"type" binding:"required,oneof=email phone"`
	Receiver      string             `json:"receiver" binding:"required"`
	CaptchaTicket *dto.CaptchaTicket `json:"captchaTicket,omitempty"` // 验证码凭证（可选）
}

// SendRegisterVerificationCodeRequest 发送注册验证码请求（不需要sessionId和认证）
type SendRegisterVerificationCodeRequest struct {
	Type          string             `json:"type" binding:"required,oneof=email phone"`
	Receiver      string             `json:"receiver" binding:"required"`
	CaptchaTicket *dto.CaptchaTicket `json:"captchaTicket,omitempty"` // 验证码凭证（可选）
}

// SendVerificationCodeResponse 发送验证码响应
type SendVerificationCodeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CompleteProfileRequest 完善信息请求
type CompleteProfileRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
	Email     string `json:"email,omitempty" binding:"omitempty,email"`
	EmailCode string `json:"emailCode,omitempty"`
	Phone     string `json:"phone,omitempty"`
	PhoneCode string `json:"phoneCode,omitempty"`
}

// CompleteProfileResponse 完善信息响应
type CompleteProfileResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	NextURL string `json:"nextUrl,omitempty"`
}

// Login OIDC登录接口
// POST /api/oidc/login
func (c OIDCAuthFlowController) Login(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 0. 验证码校验
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene("login", req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// 1. 验证会话是否存在
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.GetSession(req.SessionID)
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

	// 4. 立即生成 OIDC Token
	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
	appService := service.NewApplicationService(helper.GetDatabase())
	app, err := appService.GetApplicationByClientID(session.ClientID)
	if err != nil {
		helper.GetLogger().Error(fmt.Sprintf("获取应用信息失败: %v", err))
		// 不阻塞登录流程，继续执行
	}

	var accessToken string
	var expiresIn int64

	if app != nil {
		issuer := utils.GetBaseURL(ctx, helper, "/oidc")
		scopes := authService.ParseScopes(session.Scope)
		tokenResponse, err := authService.GenerateTokens(app, user.ID, scopes, session.Nonce, issuer)
		if err != nil {
			helper.GetLogger().Error(fmt.Sprintf("生成 Token 失败: %v", err))
			// 不阻塞登录流程，继续执行
		} else {
			accessToken = tokenResponse.AccessToken
			expiresIn = tokenResponse.ExpiresIn

			// 5. 设置 Refresh Token 到 HttpOnly Cookie
			maxAge := 0 // 默认会话期
			if req.RememberMe {
				maxAge = 7 * 24 * 60 * 60 // 7天
			}

			isSecure := ctx.Request.TLS != nil
			// SameSite 必须在 SetCookie 之前调用才能生效
			// HTTPS 使用 None 支持 iframe 跨域场景；HTTP 使用 Lax（开发环境兼容）
			if isSecure {
				ctx.SetSameSite(http.SameSiteNoneMode)
			} else {
				ctx.SetSameSite(http.SameSiteLaxMode)
			}
			ctx.SetCookie("refresh_token", tokenResponse.RefreshToken, maxAge, "/", "", isSecure, true)
		}
	}

	// 6. 检查 scope 是否需要用户完善信息
	missingFields := c.checkMissingUserInfo(session.Scope, user)

	// 7. 返回成功响应
	response := LoginResponse{
		Success: true,
		Message: "登录成功",
		UserInfo: &struct {
			ID       uint   `json:"id"`
			Username string `json:"username"`
			RealName string `json:"realName"`
		}{
			ID:       user.ID,
			Username: user.Username,
			RealName: user.RealName,
		},
		AccessToken: accessToken, // 返回访问令牌
		ExpiresIn:   expiresIn,   // 返回过期时间
	}

	if len(missingFields) > 0 {
		// 需要完善信息，跳转到信息完善页面
		response.RequiresProfileCompletion = true
		response.MissingFields = missingFields
		response.NextURL = "/auth/#/oidc/complete-profile?session_id=" + req.SessionID + "&missing_fields=" + strings.Join(missingFields, ",")
	} else {
		// 信息完整，直接跳转到授权确认页面
		response.NextURL = "/auth/#/oidc/consent?session_id=" + req.SessionID
	}

	c.Success(ctx, response)
}

// Register OIDC用户注册接口
// POST /api/oidc/register
func (c OIDCAuthFlowController) Register(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 0. 验证码校验
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene("register", req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// 1. 如果有sessionId，验证会话是否存在
	if req.SessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		_, err := sessionService.GetSession(req.SessionID)
		if err != nil {
			c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
			return
		}
	}

	// 2. 根据配置验证注册字段
	settingService := service.NewSettingService(helper.GetDatabase())
	createUserReq := &dto.CreateUserRequest{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
		RealName: req.RealName,
		Phone:    req.Phone,
		Nickname: req.Nickname,
	}

	if err := settingService.ValidateRegisterFields(createUserReq); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// 3. 验证邮箱和手机验证码
	config := c.getVerificationCodeConfig(helper)
	if config != nil {
		verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)

		// 验证邮箱验证码（如果提供了邮箱）
		if req.Email != "" && req.EmailCode != "" {
			sessionID := "register:" + req.Email
			if err := verificationService.VerifyCode(ctx.Request.Context(), req.Email, service.VerificationTypeEmail, sessionID, req.EmailCode); err != nil {
				c.Error(ctx, constants.ErrCodeBadRequest, "邮箱验证码错误或已过期")
				return
			}
		}

		// 验证手机验证码（如果提供了手机号）
		if req.Phone != "" && req.PhoneCode != "" {
			sessionID := "register:" + req.Phone
			if err := verificationService.VerifyCode(ctx.Request.Context(), req.Phone, service.VerificationTypePhone, sessionID, req.PhoneCode); err != nil {
				c.Error(ctx, constants.ErrCodeBadRequest, "手机验证码错误或已过期")
				return
			}
		}
	}

	// 4. 创建用户
	userService := service.NewUserService(helper.GetDatabase())
	userResponse, err := userService.CreateUser(createUserReq)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "用户名已存在") || strings.Contains(errMsg, "用户已存在") {
			errCode = constants.ErrCodeUserAlreadyExists
		} else if strings.Contains(errMsg, "邮箱已存在") {
			errCode = constants.ErrCodeEmailAlreadyExists
		}

		c.Error(ctx, errCode, errMsg)
		return
	}

	// 5. 如果有会话，标记为已认证
	if req.SessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionService.MarkAsAuthenticated(req.SessionID, userResponse.ID); err != nil {
			// 注册成功但更新会话失败，仍返回成功
			response := RegisterResponse{
				Success: true,
				Message: "注册成功，但会话更新失败，请重新登录",
				UserID:  userResponse.ID,
			}
			c.Success(ctx, response)
			return
		}
	}

	// 6. 返回成功响应
	response := RegisterResponse{
		Success: true,
		Message: "注册成功",
		UserID:  userResponse.ID,
	}

	c.Success(ctx, response)
}

// Consent OIDC授权确认接口
// POST /webhook/oidc/consent
func (c OIDCAuthFlowController) Consent(ctx *gin.Context, helper interfaces.HelperInterface) {
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

		c.Success(ctx, response)
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

	// 获取issuer，确保与Discovery端点一致（从session中获取clientID，而非URL参数）
	issuer := utils.GetBaseURL(ctx, helper, fmt.Sprintf("/oidc/%s", session.ClientID))

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

		c.Success(ctx, response)
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

// checkMissingUserInfo 检查用户是否缺少 scope 所需的信息
func (c OIDCAuthFlowController) checkMissingUserInfo(scopeStr string, user *dto.UserResponse) []string {
	var missingFields []string

	// 解析 scope
	scopeList := service.NewOIDCAuthorizationService(nil).ParseScopes(scopeStr)

	for _, scope := range scopeList {
		switch scope {
		case "email":
			if user.Email == "" {
				missingFields = append(missingFields, "email")
			}
		case "phone":
			if user.Phone == "" {
				missingFields = append(missingFields, "phone")
			}
		}
	}

	return missingFields
}

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

// getVerificationCodeConfig 从数据库获取验证码服务配置
func (c OIDCAuthFlowController) getVerificationCodeConfig(helper interfaces.HelperInterface) *service.VerificationCodeConfig {
	// 从数据库获取推送服务配置
	settingService := service.NewSettingService(helper.GetDatabase())
	pushSettings, err := settingService.GetPushSettings()
	if err != nil {
		// 配置获取失败，返回 nil
		return nil
	}

	// 检查必填字段
	if pushSettings.BaseURL == "" || pushSettings.AppID == "" || pushSettings.AppSecret == "" {
		return nil
	}

	return &service.VerificationCodeConfig{
		BaseURL:        pushSettings.BaseURL,
		AppID:          pushSettings.AppID,
		AppSecret:      pushSettings.AppSecret,
		SMSChannelID:   pushSettings.SMSChannelID,
		EmailChannelID: pushSettings.EmailChannelID,
		SMSSignature:   pushSettings.SMSSignature,
		EmailSignature: pushSettings.EmailSignature,
	}
}

// SendVerificationCode 发送验证码接口
// POST /api/oidc/send-verification-code
func (c OIDCAuthFlowController) SendVerificationCode(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req SendVerificationCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 0. 验证码校验（根据类型决定场景）
	captchaScene := "sendSms"
	if req.Type == "email" {
		captchaScene = "sendEmail"
	}
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene(captchaScene, req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// 1. 验证会话是否存在且已认证
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.GetSession(req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}

	if !session.IsAuthenticated || session.UserID == nil {
		c.Error(ctx, constants.ErrCodeUnauthorized, "用户尚未认证")
		return
	}

	// 2. 获取验证码服务配置
	config := c.getVerificationCodeConfig(helper)
	if config == nil {
		c.Error(ctx, constants.ErrCodeInternal, "验证码服务未配置")
		return
	}

	// 3. 创建验证码服务并发送验证码
	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)

	var verificationType service.VerificationType
	if req.Type == "email" {
		verificationType = service.VerificationTypeEmail
	} else {
		verificationType = service.VerificationTypePhone
	}

	if err := verificationService.SendCode(ctx.Request.Context(), req.Receiver, verificationType, req.SessionID); err != nil {
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}

	response := SendVerificationCodeResponse{
		Success: true,
		Message: "验证码已发送",
	}

	c.Success(ctx, response)
}

// GetRegisterSettings 获取注册字段配置（公开接口）
// GET /api/oidc/register-settings
func (c OIDCAuthFlowController) GetRegisterSettings(ctx *gin.Context, helper interfaces.HelperInterface) {
	settingService := service.NewSettingService(helper.GetDatabase())

	config, err := settingService.GetRegisterFieldsSettings()
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "获取注册字段配置失败: "+err.Error())
		return
	}

	c.Success(ctx, config)
}

// CompleteProfile 完善用户信息接口
// POST /api/oidc/complete-profile
func (c OIDCAuthFlowController) CompleteProfile(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req CompleteProfileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 1. 验证会话是否存在且已认证
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.GetSession(req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}

	if !session.IsAuthenticated || session.UserID == nil {
		c.Error(ctx, constants.ErrCodeUnauthorized, "用户尚未认证")
		return
	}

	// 2. 获取验证码服务配置
	config := c.getVerificationCodeConfig(helper)
	if config == nil {
		c.Error(ctx, constants.ErrCodeInternal, "验证码服务未配置")
		return
	}

	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)

	// 3. 验证邮箱验证码（如果提供了邮箱）
	if req.Email != "" {
		if req.EmailCode == "" {
			c.Error(ctx, constants.ErrCodeBadRequest, "请输入邮箱验证码")
			return
		}
		if err := verificationService.VerifyCode(ctx.Request.Context(), req.Email, service.VerificationTypeEmail, req.SessionID, req.EmailCode); err != nil {
			c.Error(ctx, constants.ErrCodeBadRequest, "邮箱验证码错误或已过期")
			return
		}
	}

	// 4. 验证手机验证码（如果提供了手机号）
	if req.Phone != "" {
		if req.PhoneCode == "" {
			c.Error(ctx, constants.ErrCodeBadRequest, "请输入手机验证码")
			return
		}
		if err := verificationService.VerifyCode(ctx.Request.Context(), req.Phone, service.VerificationTypePhone, req.SessionID, req.PhoneCode); err != nil {
			c.Error(ctx, constants.ErrCodeBadRequest, "手机验证码错误或已过期")
			return
		}
	}

	// 5. 更新用户信息
	userService := service.NewUserService(helper.GetDatabase())

	// 先获取当前用户信息，以保留未修改的字段
	currentUser, err := userService.GetUserModelByID(*session.UserID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "获取用户信息失败: "+err.Error())
		return
	}

	updateReq := &dto.UpdateUserRequest{}

	// 只更新用户提供的字段，未提供的保留原值
	if req.Email != "" {
		updateReq.Email = req.Email
	} else if currentUser.Email != nil {
		updateReq.Email = *currentUser.Email
	}

	if req.Phone != "" {
		updateReq.Phone = req.Phone
	} else if currentUser.Phone != nil {
		updateReq.Phone = *currentUser.Phone
	}

	_, err = userService.UpdateUser(*session.UserID, updateReq)
	if err != nil {
		// 检查是否是邮箱或手机号已存在的错误
		errMsg := err.Error()
		if strings.Contains(errMsg, "邮箱已存在") || strings.Contains(errMsg, "email") {
			c.Error(ctx, constants.ErrCodeEmailAlreadyExists, "该邮箱已被其他用户使用")
			return
		}
		if strings.Contains(errMsg, "手机号已存在") || strings.Contains(errMsg, "phone") {
			c.Error(ctx, constants.ErrCodeBadRequest, "该手机号已被其他用户使用")
			return
		}
		c.Error(ctx, constants.ErrCodeInternal, "更新用户信息失败: "+err.Error())
		return
	}

	// 6. 返回成功响应，跳转到授权确认页面
	response := CompleteProfileResponse{
		Success: true,
		Message: "信息完善成功",
		NextURL: "/auth/#/oidc/consent?session_id=" + req.SessionID,
	}

	c.Success(ctx, response)
}

// SendRegisterVerificationCode 发送注册验证码接口（无需登录认证）
// POST /api/oidc/send-register-verification-code
func (c OIDCAuthFlowController) SendRegisterVerificationCode(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req SendRegisterVerificationCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 0. 验证码校验（根据类型决定场景）
	captchaScene := "sendSms"
	if req.Type == "email" {
		captchaScene = "sendEmail"
	}
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene(captchaScene, req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// 1. 获取验证码服务配置
	config := c.getVerificationCodeConfig(helper)
	if config == nil {
		c.Error(ctx, constants.ErrCodeInternal, "验证码服务未配置")
		return
	}

	// 2. 创建验证码服务并发送验证码
	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)

	var verificationType service.VerificationType
	if req.Type == "email" {
		verificationType = service.VerificationTypeEmail
	} else {
		verificationType = service.VerificationTypePhone
	}

	// 使用接收者作为sessionID（注册场景没有真实的session）
	// 这样可以保证每个接收者有独立的验证码
	sessionID := "register:" + req.Receiver

	if err := verificationService.SendCode(ctx.Request.Context(), req.Receiver, verificationType, sessionID); err != nil {
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}

	response := SendVerificationCodeResponse{
		Success: true,
		Message: "验证码已发送",
	}

	c.Success(ctx, response)
}

// RefreshTokenResponse Token 刷新响应
type RefreshTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int64  `json:"expiresIn"`
	TokenType   string `json:"tokenType"`
}

// RefreshToken 刷新访问令牌
// POST /api/auth/refresh
func (c OIDCAuthFlowController) RefreshToken(ctx *gin.Context, helper interfaces.HelperInterface) {
	// 1. 从 Cookie 获取 Refresh Token
	refreshToken, err := ctx.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		c.Error(ctx, constants.ErrCodeUnauthorized, "未找到刷新令牌")
		return
	}

	// 2. 验证 Refresh Token
	tokenDAO := dao.NewOIDCTokenDAO(helper.GetDatabase())
	oldToken, err := tokenDAO.GetValidByRefreshToken(refreshToken)
	if err != nil {
		// Token 无效，清除 Cookie
		ctx.SetCookie("refresh_token", "", -1, "/", "", false, true)
		c.Error(ctx, constants.ErrCodeUnauthorized, "刷新令牌无效或已过期")
		return
	}

	// 3. 生成新 Token
	appService := service.NewApplicationService(helper.GetDatabase())
	app, err := appService.GetApplicationByClientID(oldToken.ClientID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "获取应用信息失败: "+err.Error())
		return
	}

	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
	issuer := utils.GetBaseURL(ctx, helper, "/oidc")
	newTokenResponse, err := authService.GenerateTokens(app, oldToken.UserID, oldToken.Scopes, "", issuer)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "生成新令牌失败: "+err.Error())
		return
	}

	// 4. 撤销旧 Token
	if err := tokenDAO.RevokeByRefreshToken(oldToken.RefreshToken); err != nil {
		helper.GetLogger().Error(fmt.Sprintf("撤销旧令牌失败: %v", err))
		// 不阻塞流程，继续执行
	}

	// 5. 更新 Cookie
	isSecure := ctx.Request.TLS != nil
	if isSecure {
		ctx.SetSameSite(http.SameSiteNoneMode)
	} else {
		ctx.SetSameSite(http.SameSiteLaxMode)
	}
	ctx.SetCookie("refresh_token", newTokenResponse.RefreshToken, 7*24*60*60, "/", "", isSecure, true)

	// 6. 返回新 Access Token
	c.Success(ctx, RefreshTokenResponse{
		AccessToken: newTokenResponse.AccessToken,
		ExpiresIn:   newTokenResponse.ExpiresIn,
		TokenType:   newTokenResponse.TokenType,
	})
}

// Logout 登出接口
// POST /api/auth/logout
func (c OIDCAuthFlowController) Logout(ctx *gin.Context, helper interfaces.HelperInterface) {
	refreshToken, err := ctx.Cookie("refresh_token")
	if err == nil && refreshToken != "" {
		tokenDAO := dao.NewOIDCTokenDAO(helper.GetDatabase())
		if err := tokenDAO.RevokeByRefreshToken(refreshToken); err != nil {
			helper.GetLogger().Error(fmt.Sprintf("撤销令牌失败: %v", err))
		}
	}
	ctx.SetCookie("refresh_token", "", -1, "/", "", false, true)
	c.Success(ctx, gin.H{"success": true, "message": "登出成功"})
}

// FlowStatusResponse 流程状态响应
type FlowStatusResponse struct {
	SessionID     string            `json:"sessionId"`
	Status        string            `json:"status"` // needs_login | needs_consent | needs_profile | error
	Prompt        string            `json:"prompt,omitempty"`
	Client        *FlowStatusClient `json:"client,omitempty"`
	User          *FlowStatusUser   `json:"user,omitempty"`
	Scopes        []FlowStatusScope `json:"scopes,omitempty"`
	MissingFields []string          `json:"missingFields,omitempty"`
	Error         *FlowStatusError  `json:"error,omitempty"`
}

// FlowStatusClient 流程状态中的客户端信息
type FlowStatusClient struct {
	ClientID    string `json:"clientId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Logo        string `json:"logo,omitempty"`
}

// FlowStatusUser 流程状态中的用户信息
type FlowStatusUser struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	RealName string `json:"realName"`
}

// FlowStatusScope 流程状态中的授权范围信息
type FlowStatusScope struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FlowStatusError 流程错误信息
type FlowStatusError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// GetFlowStatus 获取 OIDC 授权流程当前状态
// GET /api/oidc/flow-status
func (c OIDCAuthFlowController) GetFlowStatus(ctx *gin.Context, helper interfaces.HelperInterface) {
	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		c.Error(ctx, constants.ErrCodeBadRequest, "缺少session_id参数")
		return
	}

	// 1. 获取会话
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.GetSession(sessionID)
	if err != nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "error",
			Error: &FlowStatusError{
				Code:        "session_expired",
				Description: "会话已过期或不存在，请重新发起授权",
			},
		})
		return
	}

	// 2. 获取客户端信息（所有状态都需要）
	appService := service.NewApplicationService(helper.GetDatabase())
	app, err := appService.GetApplicationByClientID(session.ClientID)
	if err != nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "error",
			Error: &FlowStatusError{
				Code:        "invalid_client",
				Description: "客户端配置错误",
			},
		})
		return
	}

	clientInfo := &FlowStatusClient{
		ClientID:    session.ClientID,
		Name:        app.Name,
		Description: app.Description,
	}

	// 3. 如果未认证，返回 needs_login
	if !session.IsAuthenticated || session.UserID == nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "needs_login",
			Prompt:    session.Prompt,
			Client:    clientInfo,
		})
		return
	}

	// 4. 已认证，获取用户信息
	userService := service.NewUserService(helper.GetDatabase())
	user, err := userService.GetUserByID(*session.UserID)
	if err != nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "error",
			Error: &FlowStatusError{
				Code:        "user_not_found",
				Description: "用户不存在或已被禁用",
			},
		})
		return
	}

	userInfo := &FlowStatusUser{
		ID:       user.ID,
		Username: user.Username,
		RealName: user.RealName,
	}

	// 5. 检查是否需要补充信息
	missingFields := c.checkMissingUserInfo(session.Scope, user)
	if len(missingFields) > 0 {
		c.Success(ctx, FlowStatusResponse{
			SessionID:     sessionID,
			Status:        "needs_profile",
			Prompt:        session.Prompt,
			Client:        clientInfo,
			User:          userInfo,
			MissingFields: missingFields,
		})
		return
	}

	// 6. 信息完整，需要授权确认
	scopes := c.parseScopesForFlow(session.Scope)
	c.Success(ctx, FlowStatusResponse{
		SessionID: sessionID,
		Status:    "needs_consent",
		Prompt:    session.Prompt,
		Client:    clientInfo,
		User:      userInfo,
		Scopes:    scopes,
	})
}

// parseScopesForFlow 解析 scope 并返回流程状态所需的描述列表
func (c OIDCAuthFlowController) parseScopesForFlow(scopeStr string) []FlowStatusScope {
	scopeDescriptions := map[string]string{
		"openid":         "验证您的身份",
		"profile":        "访问您的基本资料（昵称、头像等）",
		"email":          "访问您的电子邮件地址",
		"phone":          "访问您的手机号码",
		"address":        "访问您的地址信息",
		"offline_access": "在您离线时访问您的数据",
	}

	var result []FlowStatusScope
	scopeList := service.NewOIDCAuthorizationService(nil).ParseScopes(scopeStr)
	for _, scope := range scopeList {
		desc, ok := scopeDescriptions[scope]
		if !ok {
			desc = "访问 " + scope + " 权限"
		}
		result = append(result, FlowStatusScope{Name: scope, Description: desc})
	}
	return result
}

// InitSessionRequest 初始化OIDC会话请求（用于iframe嵌入方式，无需浏览器跳转）
type InitSessionRequest struct {
	ClientID            string `json:"clientId" binding:"required"`
	RedirectURI         string `json:"redirectUri" binding:"required"`
	Scope               string `json:"scope" binding:"required"`
	ResponseType        string `json:"responseType" binding:"required"`
	State               string `json:"state"`
	Nonce               string `json:"nonce"`
	CodeChallenge       string `json:"codeChallenge"`
	CodeChallengeMethod string `json:"codeChallengeMethod"`
}

// InitSessionResponse 初始化OIDC会话响应
type InitSessionResponse struct {
	SessionID string `json:"sessionId"`
	LoginURL  string `json:"loginUrl"` // 供 iframe src 使用的登录表单 URL
}

// InitSession 初始化OIDC会话
// POST /api/oidc/init-session
// 用于iframe嵌入方式：第三方调用此接口获取session_id，然后将loginUrl赋给iframe src
func (c OIDCAuthFlowController) InitSession(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req InitSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 1. 验证客户端应用是否存在
	appService := service.NewApplicationService(helper.GetDatabase())
	if _, err := appService.GetApplicationByClientID(req.ClientID); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的客户端ID")
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
		"",
	)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "创建会话失败: "+err.Error())
		return
	}

	// 3. 构建登录表单URL（供iframe src使用）
	loginURL := "/auth/#/oidc/authorize?session_id=" + session.SessionID

	c.Success(ctx, InitSessionResponse{
		SessionID: session.SessionID,
		LoginURL:  loginURL,
	})
}
