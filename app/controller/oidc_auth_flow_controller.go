package controller

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
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

// LoginByCodeRequest 验证码登录请求
type LoginByCodeRequest struct {
	SessionID  string `json:"sessionId" binding:"required"`
	Type       string `json:"type" binding:"required,oneof=email phone"`
	Receiver   string `json:"receiver" binding:"required"`
	Code       string `json:"code" binding:"required,len=6"`
	RememberMe bool   `json:"rememberMe"` // 记住我
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username              string             `json:"username" binding:"omitempty"`
	Email                 string             `json:"email" binding:"omitempty,email"`
	EmailCode             string             `json:"emailCode" binding:"omitempty"`
	Phone                 string             `json:"phone" binding:"omitempty"`
	PhoneCode             string             `json:"phoneCode" binding:"omitempty"`
	Password              string             `json:"password" binding:"required,min=6"`
	RealName              string             `json:"realName" binding:"omitempty"`
	Nickname              string             `json:"nickname" binding:"omitempty"`
	SessionID             string             `json:"sessionId,omitempty"`
	RememberMe            bool               `json:"rememberMe"` // 记住我
	AcceptedAgreementKeys []string           `json:"acceptedAgreementKeys,omitempty"`
	CaptchaTicket         *dto.CaptchaTicket `json:"captchaTicket,omitempty"` // 验证码凭证（可选）
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Success                   bool                      `json:"success"`
	Message                   string                    `json:"message"`
	Error                     *dto.AuthorizationFailure `json:"error,omitempty"`
	UserID                    uint                      `json:"userId,omitempty"`
	AutoLogin                 bool                      `json:"autoLogin"`
	NextURL                   string                    `json:"nextUrl,omitempty"`
	AccessToken               string                    `json:"accessToken,omitempty"`
	ExpiresIn                 int64                     `json:"expiresIn,omitempty"`
	RequiresProfileCompletion bool                      `json:"requiresProfileCompletion,omitempty"`
	MissingFields             []string                  `json:"missingFields,omitempty"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Success                   bool                      `json:"success"`
	Message                   string                    `json:"message"`
	Error                     *dto.AuthorizationFailure `json:"error,omitempty"`
	NextURL                   string                    `json:"nextUrl,omitempty"`
	RequiresProfileCompletion bool                      `json:"requiresProfileCompletion,omitempty"`
	MissingFields             []string                  `json:"missingFields,omitempty"`
	UserInfo                  *struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		RealName string `json:"realName"`
	} `json:"userInfo,omitempty"`
	AccessToken string `json:"accessToken,omitempty"` // 访问令牌
	ExpiresIn   int64  `json:"expiresIn,omitempty"`   // 过期时间（秒）
}

// ConsentRequest 授权确认请求。
// CsrfToken 由 /api/oidc/auth-info 下发给前端，必须在 POST 中原样带回，
// 用于防止 CSRF（攻击者仅凭已知 sessionId 无法伪造授权）。
type ConsentRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
	Approved  bool   `json:"approved"`
	CsrfToken string `json:"csrfToken" binding:"required"`
}

// SwitchAccountRequest switches the authenticated user while preserving the
// current OIDC authorization request.
type SwitchAccountRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
	CsrfToken string `json:"csrfToken" binding:"required"`
}

type RegistrationChoiceRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
	Choice    string `json:"choice" binding:"required,oneof=create current cancel"`
	CsrfToken string `json:"csrfToken" binding:"required"`
}

type RegistrationChoiceResponse struct {
	Success     bool                      `json:"success"`
	Message     string                    `json:"message"`
	RedirectURL string                    `json:"redirectUrl,omitempty"`
	Error       *dto.AuthorizationFailure `json:"error,omitempty"`
}

// ConsentResponse 授权确认响应
type ConsentResponse struct {
	Success     bool                      `json:"success"`
	Message     string                    `json:"message"`
	RedirectURL string                    `json:"redirectUrl,omitempty"`
	Error       *dto.AuthorizationFailure `json:"error,omitempty"`
}

// AuthInfoResponse 授权信息响应。
// CsrfToken 为当前会话绑定的一次性令牌，供前端回传到 /webhook/oidc/consent 做 CSRF 校验。
type AuthInfoResponse struct {
	SessionID string `json:"sessionId"`
	CsrfToken string `json:"csrfToken"`
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

// ClientConfigResponse OIDC 登录页客户端配置响应。
// v1 先用应用基础信息和固定默认值补齐前端契约，不引入额外持久化字段。
type ClientConfigResponse struct {
	ClientID         string                    `json:"clientId"`
	AppName          string                    `json:"appName"`
	Description      string                    `json:"description,omitempty"`
	Logo             string                    `json:"logo,omitempty"`
	Appearance       ClientAppearanceConfig    `json:"appearance"`
	Features         ClientFeaturesConfig      `json:"features"`
	LoginMethods     ClientLoginMethods        `json:"loginMethods"`
	SocialProviders  []string                  `json:"socialProviders"`
	Locale           string                    `json:"locale"`
	PrivacyPolicyURL string                    `json:"privacyPolicyUrl,omitempty"`
	TermsURL         string                    `json:"termsUrl,omitempty"`
	Agreements       []dto.ClientAgreementLink `json:"agreements"`
}

type ClientAppearanceConfig struct {
	PrimaryColor string `json:"primaryColor,omitempty"`
	BorderRadius string `json:"borderRadius,omitempty"`
	DarkMode     string `json:"darkMode,omitempty"`
}

type ClientFeaturesConfig struct {
	Registration     bool `json:"registration"`
	SocialLogin      bool `json:"socialLogin"`
	EnterpriseLogin  bool `json:"enterpriseLogin"`
	RememberMe       bool `json:"rememberMe"`
	ForgotPassword   bool `json:"forgotPassword"`
	PasswordStrength bool `json:"passwordStrength"`
}

type ClientLoginMethods struct {
	Username bool `json:"username"`
	Email    bool `json:"email"`
	Phone    bool `json:"phone"`
}

// SendVerificationCodeRequest 发送验证码请求
type SendVerificationCodeRequest struct {
	SessionID     string             `json:"sessionId" binding:"required"`
	Type          string             `json:"type" binding:"required,oneof=email phone"`
	Receiver      string             `json:"receiver" binding:"required"`
	CaptchaTicket *dto.CaptchaTicket `json:"captchaTicket,omitempty"` // 验证码凭证（可选）
}

// SendLoginVerificationCodeRequest 发送登录验证码请求
type SendLoginVerificationCodeRequest struct {
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

func buildClientConfigResponse(app *model.Application, agreements []dto.ClientAgreementLink) ClientConfigResponse {
	response := ClientConfigResponse{
		ClientID:    app.UUID,
		AppName:     app.Name,
		Description: app.Description,
		Logo:        app.Icon,
		Appearance: ClientAppearanceConfig{
			PrimaryColor: "#3b82f6",
			DarkMode:     "auto",
		},
		Features: ClientFeaturesConfig{
			Registration:     service.ApplicationAllowsRegistration(app),
			SocialLogin:      true,
			EnterpriseLogin:  true,
			RememberMe:       true,
			ForgotPassword:   false,
			PasswordStrength: true,
		},
		LoginMethods: ClientLoginMethods{
			Username: true,
			Email:    true,
			Phone:    true,
		},
		SocialProviders: []string{},
		Locale:          "zh-CN",
		Agreements:      agreements,
	}
	for _, agreement := range agreements {
		if agreement.Scope != string(model.AgreementScopeGlobal) {
			continue
		}
		switch agreement.Code {
		case model.AgreementCodePrivacy:
			response.PrivacyPolicyURL = agreement.URL
		case model.AgreementCodeTerms:
			response.TermsURL = agreement.URL
		}
	}

	return response
}

func validateClientConfigSession(clientID string, session *service.OIDCSession) error {
	if session == nil {
		return fmt.Errorf("会话已过期或不存在，请重新发起授权")
	}
	if session.ClientID != clientID {
		return fmt.Errorf("会话与客户端不匹配")
	}
	return nil
}

func resolveClientConfigClientID(
	clientID string,
	sessionID string,
	getSession func(string) (*service.OIDCSession, error),
) (string, error) {
	clientID = strings.TrimSpace(clientID)
	sessionID = strings.TrimSpace(sessionID)
	if clientID == "" && sessionID == "" {
		return "", fmt.Errorf("缺少client_id或session_id参数，请重新发起授权")
	}
	if sessionID == "" {
		return clientID, nil
	}
	if getSession == nil {
		return "", fmt.Errorf("无法读取授权会话，请重新发起授权")
	}

	session, err := getSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("无效的会话ID或会话已过期")
	}
	if session == nil || strings.TrimSpace(session.ClientID) == "" {
		return "", fmt.Errorf("会话缺少客户端信息，请重新发起授权")
	}
	if clientID == "" {
		return strings.TrimSpace(session.ClientID), nil
	}
	if err := validateClientConfigSession(clientID, session); err != nil {
		return "", err
	}
	return clientID, nil
}

// SendVerificationCodeResponse 发送验证码响应
type SendVerificationCodeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	TraceID string `json:"traceId,omitempty"`
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

// GetClientConfig 获取 OIDC 登录页客户端配置
// GET /api/oidc/client-config
func (c OIDCAuthFlowController) GetClientConfig(ctx httpInterfaces.RouterContextInterface) {
	clientID, err := resolveClientConfigClientID(
		ctx.Query("client_id"),
		ctx.Query("session_id"),
		func(sessionID string) (*service.OIDCSession, error) {
			sessionService := service.NewOIDCSessionService(helper.GetRedis())
			return getBoundOIDCSession(ctx, sessionService, sessionID)
		},
	)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	db := helper.GetDatabase()
	appService := service.NewApplicationService(db)
	app, err := appService.GetApplicationByClientID(clientID)
	if err != nil {
		errCode := constants.ErrCodeInternal
		if errors.Is(err, service.ErrApplicationNotFound) {
			errCode = constants.ErrCodeNotFound
		}
		c.Error(ctx, errCode, err.Error())
		return
	}
	if !app.IsActive {
		c.Error(ctx, constants.ErrCodeForbidden, "应用已被禁用")
		return
	}

	agreementService := service.NewAgreementService(db)
	agreementLinks, err := agreementService.BuildClientAgreementLinks(app)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "获取协议配置失败: "+err.Error())
		return
	}

	response := buildClientConfigResponse(app, agreementLinks)
	// 找回密码依赖验证码推送服务，未配置时不向前端暴露入口
	response.Features.ForgotPassword = getVerificationCodeConfig() != nil

	c.Success(ctx, response)
}

// Login OIDC登录接口
// POST /api/oidc/login
func (c OIDCAuthFlowController) Login(ctx httpInterfaces.RouterContextInterface) {
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
	session, err := getBoundOIDCSession(ctx, sessionService, req.SessionID)
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

	response, err := c.completeOIDCLogin(ctx, sessionService, session, req.SessionID, user, req.RememberMe)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}

	c.Success(ctx, response)
}

func (c OIDCAuthFlowController) completeOIDCLogin(ctx httpInterfaces.RouterContextInterface, sessionService *service.OIDCSessionService, session *service.OIDCSession, sessionID string, user *dto.UserResponse, rememberMe bool) (*LoginResponse, error) {
	// 应用访问范围是登录事务的授权闸门。必须在标记会话、写 Cookie 和
	// 签发任何应用 Token 之前完成，避免无权用户获得已认证事务。
	app, failure, err := checkOIDCUserAccess(session, user.ID)
	if err != nil {
		return nil, err
	}
	if failure != nil {
		if err := markOIDCSessionFailed(ctx.Request().Context(), sessionService, sessionID, failure); err != nil {
			return nil, err
		}
		return &LoginResponse{
			Success: false,
			Message: failure.Description,
			Error:   failure,
		}, nil
	}

	// 在改变当前浏览器的账号前，先准备新账号的全部凭据。
	inheritSID := ""
	if session.RegistrationDecision == service.OIDCRegistrationChoiceCurrent {
		if raw, err := ctx.Cookie(middleware.UserCookieName); err == nil {
			if claims, err := middleware.ValidateUserSessionToken(raw); err == nil && claims.UserID == user.ID {
				inheritSID = claims.SessionID
			}
		}
		if inheritSID == "" {
			return nil, dao.ErrLoginSessionInvalid
		}
	}
	userSessionToken, userSessionMaxAge, loginSID, err := prepareUserSessionCookie(ctx, user, rememberMe, inheritSID)
	if err != nil {
		return nil, fmt.Errorf("准备用户会话失败: %w", err)
	}
	completed := false
	defer func() {
		if !completed && inheritSID == "" {
			_ = service.NewLoginSessionService(helper.GetDatabase()).Revoke(user.ID, loginSID, "", "login_failed", nil)
		}
	}()
	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
	issuer := utils.GetBaseURL(ctx, "/oidc")
	scopes := authService.ParseScopes(session.Scope)
	tokenResponse, err := authService.GenerateTokens(app, user.ID, scopes, session.Nonce, issuer, loginSID)
	if err != nil {
		return nil, fmt.Errorf("生成 Token 失败: %w", err)
	}

	// 标记会话为已认证
	if err := sessionService.MarkAsAuthenticated(ctx.Request().Context(), sessionID, user.ID, loginSID); err != nil {
		_ = dao.NewOIDCTokenDAO(helper.GetDatabase()).RevokeByRefreshToken(tokenResponse.RefreshToken)
		return nil, fmt.Errorf("更新会话失败: %w", err)
	}

	// 新会话就绪后再撤销旧 refresh token。失败时回滚新会话并保留旧 Cookie。
	oldRefreshToken, _ := ctx.Cookie("refresh_token")
	if oldRefreshToken != "" && oldRefreshToken != tokenResponse.RefreshToken {
		if err := dao.NewOIDCTokenDAO(helper.GetDatabase()).RevokeByRefreshToken(oldRefreshToken); err != nil {
			_ = sessionService.ResetAuthentication(ctx.Request().Context(), sessionID)
			_ = dao.NewOIDCTokenDAO(helper.GetDatabase()).RevokeByRefreshToken(tokenResponse.RefreshToken)
			return nil, fmt.Errorf("撤销旧登录会话失败: %w", err)
		}
	}

	if err := finishUserSession(ctx, userSessionToken, userSessionMaxAge, loginSID, user.ID); err != nil {
		return nil, err
	}
	completed = true
	recordSuccessfulUserLogin(ctx, user)
	refreshMaxAge := 0
	if rememberMe {
		refreshMaxAge = 7 * 24 * 60 * 60
	}
	isSecure := ctx.Request().TLS != nil
	if isSecure {
		ctx.SetSameSite(http.SameSiteNoneMode)
	} else {
		ctx.SetSameSite(http.SameSiteLaxMode)
	}
	ctx.SetCookie("refresh_token", tokenResponse.RefreshToken, refreshMaxAge, "/", "", isSecure, true)

	// 检查 scope 是否需要用户完善信息
	missingFields := c.checkMissingUserInfo(session.Scope, user)

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
		AccessToken: tokenResponse.AccessToken,
		ExpiresIn:   tokenResponse.ExpiresIn,
	}

	if len(missingFields) > 0 {
		// 需要完善信息，跳转到信息完善页面
		response.RequiresProfileCompletion = true
		response.MissingFields = missingFields
		response.NextURL = "/auth/#/oidc/complete-profile?session_id=" + sessionID + "&missing_fields=" + strings.Join(missingFields, ",")
	} else {
		// 信息完整，直接跳转到授权确认页面
		response.NextURL = "/auth/#/oidc/consent?session_id=" + sessionID
	}

	return &response, nil
}

// LoginByCode OIDC 验证码登录接口
// POST /api/oidc/login-by-code
func (c OIDCAuthFlowController) LoginByCode(ctx httpInterfaces.RouterContextInterface) {
	var req LoginByCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessionService, req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}

	verificationType := requestVerificationType(req.Type)
	userService := service.NewUserService(helper.GetDatabase())
	user, err := userService.GetVerifiedUserForCodeLogin(verificationType, req.Receiver)
	if err != nil {
		// 主表未注册但迁移数据里有可认领的旧用户时不报错：等验证码证明归属后再认领建号。
		// 其余错误（禁用/未验证）维持原判。
		claimable := errors.Is(err, service.ErrLoginAccountNotFound) &&
			service.NewLegacyLoginService(helper.GetDatabase()).HasPendingCodeLoginUser(verificationType, req.Receiver)
		if !claimable {
			code := constants.ErrCodeBadRequest
			if strings.Contains(err.Error(), "禁用") {
				code = constants.ErrCodeUnauthorized
			}
			c.Error(ctx, code, err.Error())
			return
		}
		user = nil
	}

	config := getVerificationCodeConfig()
	if config == nil {
		c.Error(ctx, constants.ErrCodeInternal, "验证码服务未配置")
		return
	}

	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)
	loginVerificationSessionID := buildLoginVerificationSessionID(req.SessionID, verificationType, req.Receiver)
	if err := verificationService.VerifyCode(ctx.Request().Context(), req.Receiver, verificationType, loginVerificationSessionID, req.Code); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "验证码错误或已过期")
		return
	}

	if user == nil {
		// 验证码已证明联系方式归属：从迁移数据认领旧用户，免密自动注册后继续登录。
		user, err = service.NewLegacyLoginService(helper.GetDatabase()).PromoteByVerifiedReceiver(verificationType, req.Receiver)
		if err != nil {
			c.Error(ctx, constants.ErrCodeInternal, "旧系统账号迁移失败，请稍后重试")
			return
		}
	}

	response, err := c.completeOIDCLogin(ctx, sessionService, session, req.SessionID, user, req.RememberMe)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}

	c.Success(ctx, response)
}

// Register OIDC用户注册接口
// POST /api/oidc/register
func (c OIDCAuthFlowController) Register(ctx httpInterfaces.RouterContextInterface) {
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	var registrationAppID *uint
	var oidcSession *service.OIDCSession

	// 1. 如果有sessionId，验证会话是否存在并解析应用
	if req.SessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		session, err := getBoundOIDCSession(ctx, sessionService, req.SessionID)
		if err != nil {
			c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
			return
		}
		appService := service.NewApplicationService(helper.GetDatabase())
		app, err := appService.GetApplicationByClientID(session.ClientID)
		if err != nil {
			c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话应用")
			return
		}
		if !service.ApplicationAllowsRegistration(app) {
			c.Error(ctx, constants.ErrCodeForbidden, oidcRegistrationUnavailableDescription)
			return
		}
		if err := validateOIDCRegistrationChoice(ctx, session); err != nil {
			errCode := constants.ErrCodeConflict
			if !errors.Is(err, service.ErrOIDCRegistrationChoiceResolved) &&
				!errors.Is(err, service.ErrOIDCRegistrationChoiceRequired) {
				errCode = constants.ErrCodeInternal
			}
			c.Error(ctx, errCode, err.Error())
			return
		}
		registrationAppID = &app.ID
		oidcSession = session
	}

	// 2. 验证码校验。先校验应用注册策略，避免禁止注册的会话触发无意义的验证码流程。
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene("register", req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// 3. 根据配置验证注册字段
	settingService := service.NewSettingService(helper.GetDatabase())
	createUserReq := &dto.CreateUserRequest{
		Username: strings.TrimSpace(req.Username),
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

	if createUserReq.Username == "" {
		userService := service.NewUserService(helper.GetDatabase())
		generatedUsername, err := userService.GenerateAvailableUsername()
		if err != nil {
			c.Error(ctx, constants.ErrCodeInternal, err.Error())
			return
		}
		createUserReq.Username = generatedUsername
	}

	agreementService := service.NewAgreementService(helper.GetDatabase())
	requiredAgreements, err := agreementService.ValidateRegistrationAcceptance(registrationAppID, req.AcceptedAgreementKeys)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// 4. 验证邮箱和手机验证码
	emailCodeVerified := false
	phoneCodeVerified := false
	config := getVerificationCodeConfig()
	if config != nil {
		verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)

		// 验证邮箱验证码（如果提供了邮箱）
		if req.Email != "" && req.EmailCode != "" {
			sessionID := "register:" + req.Email
			if err := verificationService.VerifyCode(ctx.Request().Context(), req.Email, service.VerificationTypeEmail, sessionID, req.EmailCode); err != nil {
				c.Error(ctx, constants.ErrCodeBadRequest, "邮箱验证码错误或已过期")
				return
			}
			emailCodeVerified = true
		}

		// 验证手机验证码（如果提供了手机号）
		if req.Phone != "" && req.PhoneCode != "" {
			sessionID := "register:" + req.Phone
			if err := verificationService.VerifyCode(ctx.Request().Context(), req.Phone, service.VerificationTypePhone, sessionID, req.PhoneCode); err != nil {
				c.Error(ctx, constants.ErrCodeBadRequest, "手机验证码错误或已过期")
				return
			}
			phoneCodeVerified = true
		}
	}

	// 5. 创建用户并记录协议同意
	var userResponse *dto.UserResponse
	err = helper.GetDatabase().Transaction(func(tx *gorm.DB) error {
		txUserService := service.NewUserService(tx)
		resp, err := txUserService.CreateUser(createUserReq)
		if err != nil {
			return err
		}
		txAgreementService := service.NewAgreementService(tx)
		if err := txAgreementService.RecordAcceptances(resp.ID, registrationAppID, requiredAgreements); err != nil {
			return err
		}
		// 验证码已证明联系方式归属：标记已验证，并尝试关联迁移数据里的同联系方式旧用户
		// （合入 legacy 身份映射属性、回填迁移状态）。
		if err := txUserService.MarkContactVerified(resp.ID, emailCodeVerified, phoneCodeVerified); err != nil {
			return err
		}
		verifiedEmail, verifiedPhone := "", ""
		if emailCodeVerified {
			verifiedEmail = createUserReq.Email
		}
		if phoneCodeVerified {
			verifiedPhone = createUserReq.Phone // CreateUser 已规范化为 E.164
		}
		if err := service.NewLegacyLoginService(tx).LinkOnRegistration(resp.ID, verifiedEmail, verifiedPhone); err != nil {
			return err
		}
		userResponse = resp
		return nil
	})
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

	// 6. 如果有会话，自动登录并衔接授权流程（生成 token、写 refresh_token cookie、返回 NextURL）
	if req.SessionID != "" && oidcSession != nil {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		loginResp, err := c.completeOIDCLogin(ctx, sessionService, oidcSession, req.SessionID, userResponse, req.RememberMe)
		if err != nil {
			// 注册成功但自动登录失败，降级为手动登录
			response := RegisterResponse{
				Success: true,
				Message: "注册成功，但会话更新失败，请重新登录",
				UserID:  userResponse.ID,
			}
			c.Success(ctx, response)
			return
		}
		if loginResp.Error != nil {
			c.Success(ctx, RegisterResponse{
				Success: true,
				Message: "注册成功，但" + loginResp.Error.Description,
				Error:   loginResp.Error,
				UserID:  userResponse.ID,
			})
			return
		}

		c.Success(ctx, RegisterResponse{
			Success:                   true,
			Message:                   "注册成功",
			UserID:                    userResponse.ID,
			AutoLogin:                 true,
			NextURL:                   loginResp.NextURL,
			AccessToken:               loginResp.AccessToken,
			ExpiresIn:                 loginResp.ExpiresIn,
			RequiresProfileCompletion: loginResp.RequiresProfileCompletion,
			MissingFields:             loginResp.MissingFields,
		})
		return
	}

	// 7. 返回成功响应
	response := RegisterResponse{
		Success: true,
		Message: "注册成功",
		UserID:  userResponse.ID,
	}

	c.Success(ctx, response)
}

func validateOIDCRegistrationChoice(
	ctx httpInterfaces.RouterContextInterface,
	session *service.OIDCSession,
) error {
	if session == nil || !promptHas(session.Prompt, "create") || session.RegistrationCandidateUserID == nil {
		return nil
	}
	switch session.RegistrationDecision {
	case service.OIDCRegistrationChoiceCreate:
		return nil
	case "":
		currentUser, err := resolveExistingIAMUser(ctx)
		if err != nil {
			return err
		}
		if currentUser != nil {
			return service.ErrOIDCRegistrationChoiceRequired
		}
		return nil
	default:
		return service.ErrOIDCRegistrationChoiceResolved
	}
}

// Consent OIDC授权确认接口
// POST /webhook/oidc/consent
func (c OIDCAuthFlowController) Consent(ctx httpInterfaces.RouterContextInterface) {
	var req ConsentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 1. 获取会话
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessionService, req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}
	if session.Error != nil {
		c.Success(ctx, ConsentResponse{
			Success: false,
			Message: session.Error.Description,
			Error:   session.Error,
		})
		return
	}

	// 2. CSRF 校验：req.CsrfToken 必须等于 session 中绑定的令牌（常量时间比较）。
	// 这可阻断"攻击者仅凭已知 sessionId 即伪造授权"的 CSRF 链路。
	if session.CsrfToken == "" ||
		subtle.ConstantTimeCompare([]byte(req.CsrfToken), []byte(session.CsrfToken)) != 1 {
		c.Error(ctx, constants.ErrCodeUnauthorized, "CSRF 校验失败")
		return
	}

	// 3. 检查会话是否已认证
	if !session.IsAuthenticated || session.UserID == nil {
		c.Error(ctx, constants.ErrCodeUnauthorized, "用户尚未认证")
		return
	}
	// 3. 处理用户拒绝授权的情况
	if !req.Approved {
		redirectURL, redirectErr := buildOIDCErrorRedirectURL(
			session.RedirectURI,
			"access_denied",
			"User denied the request",
			session.State,
		)
		if redirectErr != nil {
			c.Error(ctx, constants.ErrCodeInternal, redirectErr.Error())
			return
		}

		// 删除会话
		sessionService.DeleteSession(ctx.Request().Context(), req.SessionID)

		response := ConsentResponse{
			Success:     false,
			Message:     "用户拒绝授权",
			RedirectURL: redirectURL,
		}

		c.Success(ctx, response)
		return
	}

	// 4. 生成授权码（服务层会再次检查应用访问范围，防止登录后权限被撤销）
	authService := service.NewOIDCAuthorizationService(helper.GetDatabase())
	authReq := &dto.AuthorizeRequest{
		LoginSessionID:      session.LoginSessionID,
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
	issuer := utils.GetBaseURL(ctx, fmt.Sprintf("/oidc/%s", session.ClientID))

	authResponse, err := authService.ProcessAuthorize(authReq, *session.UserID, issuer)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "处理授权请求失败: "+err.Error())
		return
	}

	// 5. 检查是否有错误
	if authResponse.Error != "" {
		redirectURL, redirectErr := buildOIDCErrorRedirectURL(
			session.RedirectURI,
			authResponse.Error,
			authResponse.ErrorDescription,
			session.State,
		)
		if redirectErr != nil {
			c.Error(ctx, constants.ErrCodeInternal, redirectErr.Error())
			return
		}

		failure := &dto.AuthorizationFailure{
			Code:        authResponse.Error,
			Description: authResponse.ErrorDescription,
			RedirectURL: redirectURL,
		}
		if err := markOIDCSessionFailed(ctx.Request().Context(), sessionService, req.SessionID, failure); err != nil {
			c.Error(ctx, constants.ErrCodeInternal, err.Error())
			return
		}

		response := ConsentResponse{
			Success: false,
			Message: authResponse.ErrorDescription,
			Error:   failure,
		}

		c.Success(ctx, response)
		return
	}

	// 6. 标记会话为已授权
	if err := sessionService.MarkAsConsentGranted(ctx.Request().Context(), req.SessionID); err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "更新会话失败: "+err.Error())
		return
	}

	// 7. 构建成功重定向URL
	redirectURL, redirectErr := buildOIDCSuccessRedirectURL(session.RedirectURI, authResponse.Code, session.State)
	if redirectErr != nil {
		c.Error(ctx, constants.ErrCodeInternal, redirectErr.Error())
		return
	}

	// 删除会话（授权码已生成，会话不再需要）
	sessionService.DeleteSession(ctx.Request().Context(), req.SessionID)

	response := ConsentResponse{
		Success:     true,
		Message:     "授权成功",
		RedirectURL: redirectURL,
	}

	c.Success(ctx, response)
}

// GetAuthInfo 获取授权信息接口
// GET /api/oidc/auth-info
func (c OIDCAuthFlowController) GetAuthInfo(ctx httpInterfaces.RouterContextInterface) {
	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		c.Error(ctx, constants.ErrCodeBadRequest, "缺少session_id参数")
		return
	}

	// 1. 获取会话
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessionService, sessionID)
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
		CsrfToken: session.CsrfToken,
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

func newVerificationCodeTraceID() string {
	v7, err := uuid.NewV7()
	if err != nil {
		v7 = uuid.New()
	}
	return v7.String()
}

func withTraceID(message, traceID string) string {
	if traceID == "" {
		return message
	}
	return fmt.Sprintf("%s（追踪ID：%s）", message, traceID)
}

func shortControllerLogValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return value
	}
	return value[:8] + "..." + value[len(value)-4:]
}

func requestVerificationType(value string) service.VerificationType {
	if value == "email" {
		return service.VerificationTypeEmail
	}
	return service.VerificationTypePhone
}

func buildLoginVerificationSessionID(sessionID string, verificationType service.VerificationType, receiver string) string {
	return fmt.Sprintf("login:%s:%s:%s", sessionID, verificationType, receiver)
}

func (c OIDCAuthFlowController) logVerificationCodeControllerInfo(traceID, format string, args ...any) {
	if logger := helper.GetLogger(); logger != nil {
		logger.Info(formatVerificationCodeControllerLog(traceID, format, args...))
	}
}

func (c OIDCAuthFlowController) logVerificationCodeControllerWarn(traceID, format string, args ...any) {
	if logger := helper.GetLogger(); logger != nil {
		logger.Warn(formatVerificationCodeControllerLog(traceID, format, args...))
	}
}

func (c OIDCAuthFlowController) logVerificationCodeControllerError(traceID, format string, args ...any) {
	if logger := helper.GetLogger(); logger != nil {
		logger.Error(formatVerificationCodeControllerLog(traceID, format, args...))
	}
}

func formatVerificationCodeControllerLog(traceID, format string, args ...any) string {
	if traceID == "" {
		traceID = "-"
	}
	return fmt.Sprintf("[verification-code] traceId=%s %s", traceID, fmt.Sprintf(format, args...))
}

// SendLoginVerificationCode 发送登录验证码接口
// POST /api/oidc/send-login-verification-code
func (c OIDCAuthFlowController) SendLoginVerificationCode(ctx httpInterfaces.RouterContextInterface) {
	traceID := newVerificationCodeTraceID()
	var req SendLoginVerificationCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.logVerificationCodeControllerError(traceID, "登录验证码请求参数错误 err=%v", err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID("请求参数错误: "+err.Error(), traceID))
		return
	}

	verificationType := requestVerificationType(req.Type)
	captchaScene := "sendSms"
	if verificationType == service.VerificationTypeEmail {
		captchaScene = "sendEmail"
	}
	c.logVerificationCodeControllerInfo(traceID, "发送登录验证码请求 scene=%s type=%s receiver=%s session=%s clientIp=%s", captchaScene, req.Type, service.MaskVerificationReceiver(req.Receiver), shortControllerLogValue(req.SessionID), ctx.ClientIP())

	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene(captchaScene, req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.logVerificationCodeControllerWarn(traceID, "登录验证码校验失败 scene=%s err=%v", captchaScene, err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID(err.Error(), traceID))
		return
	}
	c.logVerificationCodeControllerInfo(traceID, "登录验证码校验通过 scene=%s", captchaScene)

	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	if _, err := getBoundOIDCSession(ctx, sessionService, req.SessionID); err != nil {
		c.logVerificationCodeControllerWarn(traceID, "登录验证码OIDC会话不存在或过期 session=%s err=%v", shortControllerLogValue(req.SessionID), err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID("无效的会话ID或会话已过期", traceID))
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	if _, err := userService.GetVerifiedUserForCodeLogin(verificationType, req.Receiver); err != nil {
		// 主表未注册但迁移数据里有可认领的旧用户时放行发码：登录时验证码通过即认领迁移。
		claimable := errors.Is(err, service.ErrLoginAccountNotFound) &&
			service.NewLegacyLoginService(helper.GetDatabase()).HasPendingCodeLoginUser(verificationType, req.Receiver)
		if !claimable {
			c.logVerificationCodeControllerWarn(traceID, "登录验证码账号校验失败 type=%s receiver=%s err=%v", verificationType, service.MaskVerificationReceiver(req.Receiver), err)
			c.Error(ctx, constants.ErrCodeBadRequest, withTraceID(err.Error(), traceID))
			return
		}
		c.logVerificationCodeControllerInfo(traceID, "主表未注册但命中可认领旧用户，放行发码 type=%s receiver=%s", verificationType, service.MaskVerificationReceiver(req.Receiver))
	}

	config := getVerificationCodeConfig()
	if config == nil {
		c.logVerificationCodeControllerError(traceID, "登录验证码服务未配置")
		c.Error(ctx, constants.ErrCodeInternal, withTraceID("验证码服务未配置", traceID))
		return
	}

	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)
	loginVerificationSessionID := buildLoginVerificationSessionID(req.SessionID, verificationType, req.Receiver)
	if err := verificationService.SendCode(ctx.Request().Context(), req.Receiver, verificationType, loginVerificationSessionID, traceID); err != nil {
		c.logVerificationCodeControllerError(traceID, "登录验证码发送失败 type=%s receiver=%s err=%v", verificationType, service.MaskVerificationReceiver(req.Receiver), err)
		c.Error(ctx, constants.ErrCodeInternal, withTraceID(err.Error(), traceID))
		return
	}

	response := SendVerificationCodeResponse{
		Success: true,
		Message: "验证码已发送",
		TraceID: traceID,
	}

	c.Success(ctx, response)
}

// SendVerificationCode 发送验证码接口
// POST /api/oidc/send-verification-code
func (c OIDCAuthFlowController) SendVerificationCode(ctx httpInterfaces.RouterContextInterface) {
	traceID := newVerificationCodeTraceID()
	var req SendVerificationCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.logVerificationCodeControllerError(traceID, "请求参数错误 err=%v", err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID("请求参数错误: "+err.Error(), traceID))
		return
	}

	// 0. 验证码校验（根据类型决定场景）
	captchaScene := "sendSms"
	if req.Type == "email" {
		captchaScene = "sendEmail"
	}
	c.logVerificationCodeControllerInfo(traceID, "发送验证码请求 scene=%s type=%s receiver=%s session=%s clientIp=%s", captchaScene, req.Type, service.MaskVerificationReceiver(req.Receiver), shortControllerLogValue(req.SessionID), ctx.ClientIP())
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene(captchaScene, req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.logVerificationCodeControllerWarn(traceID, "验证码校验失败 scene=%s err=%v", captchaScene, err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID(err.Error(), traceID))
		return
	}
	c.logVerificationCodeControllerInfo(traceID, "验证码校验通过 scene=%s", captchaScene)

	// 1. 验证会话是否存在且已认证
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessionService, req.SessionID)
	if err != nil {
		c.logVerificationCodeControllerWarn(traceID, "OIDC会话不存在或过期 session=%s err=%v", shortControllerLogValue(req.SessionID), err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID("无效的会话ID或会话已过期", traceID))
		return
	}

	if !session.IsAuthenticated || session.UserID == nil {
		c.logVerificationCodeControllerWarn(traceID, "OIDC会话未认证 session=%s", shortControllerLogValue(req.SessionID))
		c.Error(ctx, constants.ErrCodeUnauthorized, withTraceID("用户尚未认证", traceID))
		return
	}

	// 2. 获取验证码服务配置
	config := getVerificationCodeConfig()
	if config == nil {
		c.logVerificationCodeControllerError(traceID, "验证码服务未配置")
		c.Error(ctx, constants.ErrCodeInternal, withTraceID("验证码服务未配置", traceID))
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

	if err := verificationService.SendCode(ctx.Request().Context(), req.Receiver, verificationType, req.SessionID, traceID); err != nil {
		c.logVerificationCodeControllerError(traceID, "验证码发送失败 type=%s receiver=%s err=%v", verificationType, service.MaskVerificationReceiver(req.Receiver), err)
		c.Error(ctx, constants.ErrCodeInternal, withTraceID(err.Error(), traceID))
		return
	}

	response := SendVerificationCodeResponse{
		Success: true,
		Message: "验证码已发送",
		TraceID: traceID,
	}

	c.Success(ctx, response)
}

// GetRegisterSettings 获取注册字段配置（公开接口）
// GET /api/oidc/register-settings
func (c OIDCAuthFlowController) GetRegisterSettings(ctx httpInterfaces.RouterContextInterface) {
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
func (c OIDCAuthFlowController) CompleteProfile(ctx httpInterfaces.RouterContextInterface) {
	var req CompleteProfileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 1. 验证会话是否存在且已认证
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessionService, req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}

	if !session.IsAuthenticated || session.UserID == nil {
		c.Error(ctx, constants.ErrCodeUnauthorized, "用户尚未认证")
		return
	}

	// 2. 获取验证码服务配置
	config := getVerificationCodeConfig()
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
		if err := verificationService.VerifyCode(ctx.Request().Context(), req.Email, service.VerificationTypeEmail, req.SessionID, req.EmailCode); err != nil {
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
		if err := verificationService.VerifyCode(ctx.Request().Context(), req.Phone, service.VerificationTypePhone, req.SessionID, req.PhoneCode); err != nil {
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

	// 只更新用户提供的字段，未提供的保留原值；
	// 本接口写入的邮箱/手机号均已通过验证码校验，需同步标记为已验证
	verified := true
	if req.Email != "" {
		updateReq.Email = req.Email
		updateReq.EmailVerified = &verified
	} else if currentUser.Email != nil {
		updateReq.Email = *currentUser.Email
	}

	if req.Phone != "" {
		updateReq.Phone = req.Phone
		updateReq.PhoneVerified = &verified
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
func (c OIDCAuthFlowController) SendRegisterVerificationCode(ctx httpInterfaces.RouterContextInterface) {
	traceID := newVerificationCodeTraceID()
	var req SendRegisterVerificationCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.logVerificationCodeControllerError(traceID, "注册验证码请求参数错误 err=%v", err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID("请求参数错误: "+err.Error(), traceID))
		return
	}

	// 0. 验证码校验（根据类型决定场景）
	captchaScene := "sendSms"
	if req.Type == "email" {
		captchaScene = "sendEmail"
	}
	c.logVerificationCodeControllerInfo(traceID, "发送注册验证码请求 scene=%s type=%s receiver=%s clientIp=%s", captchaScene, req.Type, service.MaskVerificationReceiver(req.Receiver), ctx.ClientIP())
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene(captchaScene, req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.logVerificationCodeControllerWarn(traceID, "注册验证码校验失败 scene=%s err=%v", captchaScene, err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID(err.Error(), traceID))
		return
	}
	c.logVerificationCodeControllerInfo(traceID, "注册验证码校验通过 scene=%s", captchaScene)

	// 1. 获取验证码服务配置
	config := getVerificationCodeConfig()
	if config == nil {
		c.logVerificationCodeControllerError(traceID, "注册验证码服务未配置")
		c.Error(ctx, constants.ErrCodeInternal, withTraceID("验证码服务未配置", traceID))
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

	if err := verificationService.SendCode(ctx.Request().Context(), req.Receiver, verificationType, sessionID, traceID); err != nil {
		c.logVerificationCodeControllerError(traceID, "注册验证码发送失败 type=%s receiver=%s err=%v", verificationType, service.MaskVerificationReceiver(req.Receiver), err)
		c.Error(ctx, constants.ErrCodeInternal, withTraceID(err.Error(), traceID))
		return
	}

	response := SendVerificationCodeResponse{
		Success: true,
		Message: "验证码已发送",
		TraceID: traceID,
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
// POST /api/oidc/auth/refresh
func (c OIDCAuthFlowController) RefreshToken(ctx httpInterfaces.RouterContextInterface) {
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

	// 历史 Refresh Token 也必须服从当前用户状态；禁用后立即拒绝并清除 Cookie。
	userModel, err := service.NewUserService(helper.GetDatabase()).GetUserModelByID(oldToken.UserID)
	if err != nil || !userModel.CanLogin() {
		ctx.SetCookie("refresh_token", "", -1, "/", "", false, true)
		c.Error(ctx, constants.ErrCodeUnauthorized, "用户已停用")
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
	issuer := utils.GetBaseURL(ctx, "/oidc")
	newTokenResponse, err := authService.RotateLoginToken(oldToken, app, "", issuer)
	if err != nil {
		c.Error(ctx, 401, "刷新令牌无效或会话已撤销")
		return
	}

	// 5. 更新 Cookie
	isSecure := ctx.Request().TLS != nil
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
// POST /api/oidc/auth/logout
func (c OIDCAuthFlowController) Logout(ctx httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(ctx) {
		return
	}
	if err := LogoutBrowserSession(ctx, false); err != nil {
		c.Error(ctx, 500, "退出登录失败，请重试")
		return
	}
	c.Success(ctx, map[string]any{"success": true, "message": "登出成功"})
}

// RegistrationChoice resolves the explicit choice shown when prompt=create is
// started from a browser that already has a valid IAM login.
// POST /api/oidc/registration-choice
func (c OIDCAuthFlowController) RegistrationChoice(ctx httpInterfaces.RouterContextInterface) {
	var req RegistrationChoiceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	sessions := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessions, req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}
	if session.Error != nil {
		c.Success(ctx, RegistrationChoiceResponse{
			Success: false,
			Message: session.Error.Description,
			Error:   session.Error,
		})
		return
	}
	if session.RegistrationCandidateUserID == nil {
		c.Error(ctx, constants.ErrCodeBadRequest, service.ErrOIDCRegistrationChoiceNotRequired.Error())
		return
	}

	switch req.Choice {
	case service.OIDCRegistrationChoiceCancel:
		c.cancelRegistrationChoice(ctx, sessions, session, &req)
	case service.OIDCRegistrationChoiceCreate:
		if _, err := sessions.ApplyRegistrationChoice(
			ctx.Request().Context(), req.SessionID, req.CsrfToken, req.Choice, nil,
		); err != nil {
			c.registrationChoiceError(ctx, err)
			return
		}
		c.Success(ctx, RegistrationChoiceResponse{Success: true, Message: "已确认创建新账号"})
	case service.OIDCRegistrationChoiceCurrent:
		c.continueWithCurrentRegistrationUser(ctx, sessions, session, &req)
	default:
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的注册意图选择")
	}
}

func (c OIDCAuthFlowController) cancelRegistrationChoice(
	ctx httpInterfaces.RouterContextInterface,
	sessions *service.OIDCSessionService,
	session *service.OIDCSession,
	req *RegistrationChoiceRequest,
) {
	failure, err := newOIDCFailure(session, "access_denied", "用户取消创建新账号")
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}
	if _, err := sessions.ApplyRegistrationChoice(
		ctx.Request().Context(), req.SessionID, req.CsrfToken, req.Choice, nil,
	); err != nil {
		c.registrationChoiceError(ctx, err)
		return
	}
	c.Success(ctx, RegistrationChoiceResponse{
		Success:     false,
		Message:     "已取消创建新账号",
		RedirectURL: failure.RedirectURL,
	})
}

func (c OIDCAuthFlowController) continueWithCurrentRegistrationUser(
	ctx httpInterfaces.RouterContextInterface,
	sessions *service.OIDCSessionService,
	session *service.OIDCSession,
	req *RegistrationChoiceRequest,
) {
	currentUser, matches, err := sameOIDCBrowserUser(ctx, *session.RegistrationCandidateUserID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "读取当前登录状态失败")
		return
	}
	if !matches {
		c.Error(ctx, constants.ErrCodeUnauthorized, "当前 IAM 登录状态已失效或账号已变更")
		return
	}
	if _, failure, accessErr := checkOIDCUserAccess(session, currentUser.ID); accessErr != nil {
		c.Error(ctx, constants.ErrCodeInternal, "检查应用访问权限失败")
		return
	} else if failure != nil {
		if err := markOIDCSessionFailed(ctx.Request().Context(), sessions, req.SessionID, failure); err != nil {
			c.Error(ctx, constants.ErrCodeInternal, err.Error())
			return
		}
		c.Success(ctx, RegistrationChoiceResponse{Success: false, Message: failure.Description, Error: failure})
		return
	}

	currentUserID := currentUser.ID
	updated, err := sessions.ApplyRegistrationChoice(
		ctx.Request().Context(), req.SessionID, req.CsrfToken, req.Choice, &currentUserID,
	)
	if err != nil {
		c.registrationChoiceError(ctx, err)
		return
	}
	loginResponse, err := c.completeOIDCLogin(ctx, sessions, updated, req.SessionID, currentUser, true)
	if err != nil {
		_ = sessions.ResetRegistrationChoice(ctx.Request().Context(), req.SessionID)
		c.Error(ctx, constants.ErrCodeInternal, "使用当前账号失败: "+err.Error())
		return
	}
	if loginResponse.Error != nil {
		c.Success(ctx, RegistrationChoiceResponse{
			Success: false,
			Message: loginResponse.Error.Description,
			Error:   loginResponse.Error,
		})
		return
	}
	c.Success(ctx, RegistrationChoiceResponse{Success: true, Message: "已使用当前账号继续"})
}

func (c OIDCAuthFlowController) registrationChoiceError(ctx httpInterfaces.RouterContextInterface, err error) {
	switch {
	case errors.Is(err, service.ErrOIDCRegistrationChoiceCSRF):
		c.Error(ctx, constants.ErrCodeUnauthorized, "CSRF 校验失败")
	case errors.Is(err, service.ErrOIDCRegistrationChoiceResolved):
		c.Error(ctx, constants.ErrCodeConflict, err.Error())
	case errors.Is(err, service.ErrOIDCRegistrationUserChanged):
		c.Error(ctx, constants.ErrCodeUnauthorized, err.Error())
	default:
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
	}
}

// SwitchAccount clears the current user from an OIDC authorization session,
// preserving the client request so the next login can continue the same flow.
// It also clears the browser-level SSO cookie to prevent the previous account
// from being silently selected on a later authorization request.
// POST /api/oidc/auth/switch-account
func (c OIDCAuthFlowController) SwitchAccount(ctx httpInterfaces.RouterContextInterface) {
	var req SwitchAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessionService, req.SessionID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的会话ID或会话已过期")
		return
	}

	if !session.IsAuthenticated || session.UserID == nil {
		c.Error(ctx, constants.ErrCodeUnauthorized, "用户尚未认证")
		return
	}

	if !validSwitchAccountCSRF(session, req.CsrfToken) {
		c.Error(ctx, constants.ErrCodeUnauthorized, "CSRF 校验失败")
		return
	}

	if err := service.NewLoginSessionService(helper.GetDatabase()).Revoke(*session.UserID, session.LoginSessionID, "", "logout", session.UserID); err != nil {
		c.Error(ctx, 500, "退出原会话失败，请重试")
		return
	}
	ClearLoginSessionCookies(ctx, session.LoginSessionID)
	if err := sessionService.ResetAuthentication(ctx.Request().Context(), req.SessionID); err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "切换账号失败: "+err.Error())
		return
	}

	c.Success(ctx, map[string]any{"success": true, "message": "已切换账号"})
}

func validSwitchAccountCSRF(session *service.OIDCSession, csrfToken string) bool {
	return session != nil && session.CsrfToken != "" &&
		subtle.ConstantTimeCompare([]byte(csrfToken), []byte(session.CsrfToken)) == 1
}

// FlowStatusResponse 流程状态响应
type FlowStatusResponse struct {
	SessionID string `json:"sessionId"`
	Status    string `json:"status"` // needs_registration_confirmation | needs_login | needs_consent | needs_profile | error
	Prompt    string `json:"prompt,omitempty"`
	// CsrfToken 在注册意图确认和授权确认状态下下发。
	CsrfToken     string                    `json:"csrfToken,omitempty"`
	AutoApprove   bool                      `json:"autoApprove,omitempty"`
	Client        *FlowStatusClient         `json:"client,omitempty"`
	User          *FlowStatusUser           `json:"user,omitempty"`
	ExistingUser  *FlowStatusUser           `json:"existingUser,omitempty"`
	Scopes        []FlowStatusScope         `json:"scopes,omitempty"`
	MissingFields []string                  `json:"missingFields,omitempty"`
	Error         *dto.AuthorizationFailure `json:"error,omitempty"`
}

// FlowStatusClient 流程状态中的客户端信息
type FlowStatusClient struct {
	ClientID    string `json:"clientId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Logo        string `json:"logo,omitempty"`
}

func buildFlowStatusClient(app *model.Application, clientID string) *FlowStatusClient {
	return &FlowStatusClient{
		ClientID:    clientID,
		Name:        app.Name,
		Description: app.Description,
		Logo:        app.Icon,
	}
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

// GetFlowStatus 获取 OIDC 授权流程当前状态
// GET /api/oidc/flow-status
func (c OIDCAuthFlowController) GetFlowStatus(ctx httpInterfaces.RouterContextInterface) {
	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		c.Error(ctx, constants.ErrCodeBadRequest, "缺少session_id参数")
		return
	}

	// 1. 获取会话
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessionService, sessionID)
	if err != nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "error",
			Error: &dto.AuthorizationFailure{
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
			Error: &dto.AuthorizationFailure{
				Code:        "invalid_client",
				Description: "客户端配置错误",
			},
		})
		return
	}

	clientInfo := buildFlowStatusClient(app, session.ClientID)

	// 3. 终态错误优先于登录/授权步骤，页面刷新后仍保持失败展示
	if session.Error != nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "error",
			Prompt:    session.Prompt,
			Client:    clientInfo,
			Error:     session.Error,
		})
		return
	}

	// 4. prompt=create 遇到当前浏览器已登录的 IAM 账号时，先要求用户确认。
	if session.RegistrationCandidateUserID != nil && session.RegistrationDecision == "" {
		existingUser, matches, resolveErr := sameOIDCBrowserUser(ctx, *session.RegistrationCandidateUserID)
		if resolveErr != nil {
			c.Error(ctx, constants.ErrCodeInternal, "读取当前登录状态失败")
			return
		}
		if existingUser != nil {
			if !matches {
				if err := sessionService.SetRegistrationCandidate(ctx.Request().Context(), sessionID, existingUser.ID); err != nil {
					c.Error(ctx, constants.ErrCodeInternal, "更新当前登录账号失败")
					return
				}
			}
			c.Success(ctx, FlowStatusResponse{
				SessionID: sessionID,
				Status:    "needs_registration_confirmation",
				Prompt:    session.Prompt,
				CsrfToken: session.CsrfToken,
				Client:    clientInfo,
				ExistingUser: &FlowStatusUser{
					ID:       existingUser.ID,
					Username: existingUser.Username,
					RealName: existingUser.RealName,
				},
			})
			return
		}
	}

	// 5. 如果未认证，返回 needs_login
	if !session.IsAuthenticated || session.UserID == nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "needs_login",
			Prompt:    session.Prompt,
			Client:    clientInfo,
		})
		return
	}

	// 5. 对已认证会话再次检查应用访问范围，兼容 SSO/外部身份入口并
	// 防止页面刷新期间继续推进一个已经失权的事务。
	_, failure, accessErr := checkOIDCUserAccess(session, *session.UserID)
	if accessErr != nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "error",
			Client:    clientInfo,
			Error: &dto.AuthorizationFailure{
				Code:        "server_error",
				Description: "检查应用访问权限失败",
			},
		})
		return
	}
	if failure != nil {
		if err := markOIDCSessionFailed(ctx.Request().Context(), sessionService, sessionID, failure); err != nil {
			c.Error(ctx, constants.ErrCodeInternal, err.Error())
			return
		}
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "error",
			Prompt:    session.Prompt,
			Client:    clientInfo,
			Error:     failure,
		})
		return
	}

	// 6. 已认证，获取用户信息
	userService := service.NewUserService(helper.GetDatabase())
	user, err := userService.GetUserByID(*session.UserID)
	if err != nil {
		c.Success(ctx, FlowStatusResponse{
			SessionID: sessionID,
			Status:    "error",
			Error: &dto.AuthorizationFailure{
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

	// 7. 检查是否需要补充信息
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

	// 8. 信息完整，需要授权确认
	scopes := c.parseScopesForFlow(session.Scope)
	c.Success(ctx, FlowStatusResponse{
		SessionID:   sessionID,
		Status:      "needs_consent",
		Prompt:      session.Prompt,
		CsrfToken:   session.CsrfToken,
		AutoApprove: service.IsAdminConsoleApplication(app),
		Client:      clientInfo,
		User:        userInfo,
		Scopes:      scopes,
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
	Prompt              string `json:"prompt"`
}

// InitSessionResponse 初始化OIDC会话响应
type InitSessionResponse struct {
	SessionID   string `json:"sessionId"`
	LoginURL    string `json:"loginUrl"` // 供 iframe src 使用的登录表单 URL
	LoginTicket string `json:"loginTicket"`
}

// BindSession consumes the one-time login ticket in the browser that will
// actually render the login UI and installs the HttpOnly binding cookie.
func (c OIDCAuthFlowController) BindSession(ctx httpInterfaces.RouterContextInterface) {
	ticket := ctx.Query("ticket")
	sessions := service.NewOIDCSessionService(helper.GetRedis())
	value, err := sessions.ConsumeLoginTicket(ctx.Request().Context(), ticket)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}
	setOIDCBrowserBindingCookie(ctx, value.SessionID, value.BrowserToken)
	ctx.Redirect(http.StatusFound, safeOIDCReturnTo(ctx.Query("return_to"), value.SessionID))
}

// InitSession 初始化OIDC会话
// POST /api/oidc/init-session
// 用于iframe嵌入方式：第三方调用此接口获取session_id，然后将loginUrl赋给iframe src
func (c OIDCAuthFlowController) InitSession(ctx httpInterfaces.RouterContextInterface) {
	var req InitSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 1. 验证客户端应用是否存在
	appService := service.NewApplicationService(helper.GetDatabase())
	app, err := appService.GetApplicationByClientID(req.ClientID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的客户端ID")
		return
	}
	if err := validateOIDCRedirectURI(app, req.RedirectURI); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "无效的重定向URI")
		return
	}
	prompt, err := normalizeAndValidateOIDCPrompt(req.Prompt)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}
	req.Prompt = prompt

	// 2. 创建OIDC会话
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	session, err := sessionService.CreateSession(
		ctx.Request().Context(),
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
		c.Error(ctx, constants.ErrCodeInternal, "创建会话失败: "+err.Error())
		return
	}
	if err := markOIDCRegistrationUnavailable(ctx.Request().Context(), sessionService, app, session); err != nil {
		_ = sessionService.DeleteSession(ctx.Request().Context(), session.SessionID)
		c.Error(ctx, constants.ErrCodeInternal, "记录注册状态失败: "+err.Error())
		return
	}
	if err := captureOIDCRegistrationCandidate(ctx, sessionService, app, session); err != nil {
		_ = sessionService.DeleteSession(ctx.Request().Context(), session.SessionID)
		c.Error(ctx, constants.ErrCodeInternal, "读取当前登录状态失败: "+err.Error())
		return
	}

	loginTicket, err := sessionService.CreateLoginTicket(ctx.Request().Context(), session)
	if err != nil {
		_ = sessionService.DeleteSession(ctx.Request().Context(), session.SessionID)
		c.Error(ctx, constants.ErrCodeInternal, "创建一次性登录票据失败: "+err.Error())
		return
	}
	loginURL := oidcLoginURL(loginTicket, session.SessionID)

	c.Success(ctx, InitSessionResponse{
		SessionID:   session.SessionID,
		LoginURL:    loginURL,
		LoginTicket: loginTicket,
	})
}
