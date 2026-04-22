package controller

import (
	"fmt"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// SocialAuthController 社会化登录控制器（用户端）
type SocialAuthController struct {
	BaseResponse
}

// GetEnabledProviders 获取启用的提供商列表（公开接口）
// GET /api/social/providers
func (sc SocialAuthController) GetEnabledProviders(c httpInterfaces.RouterContextInterface) {
	providerService := service.NewSocialProviderService(helper.GetDatabase(), helper.GetRedis())
	providers, err := providerService.GetEnabledProviders()
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	sc.Success(c, providers)
}

// InitiateAuth 发起OAuth授权
// GET /api/social/auth/:provider?session_id=xxx
func (sc SocialAuthController) InitiateAuth(c httpInterfaces.RouterContextInterface) {
	providerIdentifier := c.Param("provider")
	if providerIdentifier == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "提供商标识符不能为空")
		return
	}

	// 获取可选的session_id参数（用于OIDC流程）
	sessionID := c.Query("session_id")

	authService := service.NewSocialAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.GetAuthURLWithSessionID(providerIdentifier, sessionID)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	// 重定向到OAuth授权页面
	c.Redirect(302, response.AuthURL)
}

// HandleCallback 处理OAuth回调
// GET /api/social/callback/:provider
func (sc SocialAuthController) HandleCallback(c httpInterfaces.RouterContextInterface) {
	providerIdentifier := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	if providerIdentifier == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "提供商标识符不能为空")
		return
	}
	if code == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "授权码不能为空")
		return
	}
	if state == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "state参数不能为空")
		return
	}

	authService := service.NewSocialAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.HandleCallback(providerIdentifier, code, state)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	// 防御性检查：确保响应不为空
	if response == nil {
		sc.Error(c, constants.ErrCodeInternal, "回调处理失败：响应为空")
		return
	}

	// 从response中获取sessionID（已从state中恢复）
	sessionID := response.SessionID

	// 如果需要绑定，返回HTML页面，通过postMessage通知父窗口
	if response.NeedsBind {
		// 从 BindToken 获取社交账号信息用于预填
		authService := service.NewSocialAuthService(helper.GetDatabase(), helper.GetRedis())
		tempUserInfo, err := authService.GetSocialUserTempInfo(response.BindToken)

		// 准备社交账号信息（用于前端预填）
		socialInfo := ""
		if err == nil && tempUserInfo != nil {
			socialInfo = fmt.Sprintf(`
            socialInfo: {
                nickname: '%s',
                avatar: '%s',
                email: '%s'
            },`,
				escapeJS(tempUserInfo.Nickname),
				escapeJS(tempUserInfo.Avatar),
				escapeJS(tempUserInfo.Email))
		}

		html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>账号绑定</title>
</head>
<body>
    <script>
        window.opener.postMessage({
            type: 'oidc-social-callback',
            needsBind: true,
            bindToken: '%s',
            sessionId: '%s',%s
        }, '*');
        window.close();
    </script>
    <p>正在跳转...</p>
</body>
</html>
`, response.BindToken, sessionID, socialInfo)
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		c.String(200, html)
		return
	}

	// 已绑定，直接登录成功
	// 如果有 sessionId，标记 OIDC 会话为已认证
	if sessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionService.MarkAsAuthenticated(c.Request().Context(), sessionID, response.User.ID); err != nil {
			sc.Error(c, constants.ErrCodeInternal, "标记会话失败: "+err.Error())
			return
		}

		// 返回HTML页面，通过postMessage通知父窗口
		html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>登录成功</title>
</head>
<body>
    <script>
        window.opener.postMessage({
            type: 'oidc-social-callback',
            needsBind: false,
            sessionId: '%s'
        }, '*');
        window.close();
    </script>
    <p>登录成功，正在关闭...</p>
</body>
</html>
`, sessionID)
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		c.String(200, html)
		return
	}

	// 非OIDC流程，返回JSON响应
	if response.IsNewUser {
		sc.SuccessWithMessage(c, "注册并登录成功", response)
	} else {
		sc.SuccessWithMessage(c, "登录成功", response)
	}
}

// BindAccount 绑定社交账号（需要认证）
// POST /api/social/bind/:provider
func (sc SocialAuthController) BindAccount(c httpInterfaces.RouterContextInterface) {
	// 从上下文获取当前用户ID（由认证中间件设置）
	userID := c.Get("user_id")
	if userID == nil {
		sc.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}

	providerIdentifier := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	if providerIdentifier == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "提供商标识符不能为空")
		return
	}
	if code == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "授权码不能为空")
		return
	}
	if state == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "state参数不能为空")
		return
	}

	authService := service.NewSocialAuthService(helper.GetDatabase(), helper.GetRedis())
	err := authService.BindSocialAccount(userID.(uint), providerIdentifier, code, state)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	sc.SuccessWithMessage(c, "绑定成功", nil)
}

// UnbindAccount 解绑社交账号（需要认证）
// DELETE /api/social/unbind/:accountId
func (sc SocialAuthController) UnbindAccount(c httpInterfaces.RouterContextInterface) {
	// 从上下文获取当前用户ID
	userID := c.Get("user_id")
	if userID == nil {
		sc.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}

	accountIDStr := c.Param("accountId")
	accountID, err := strconv.ParseUint(accountIDStr, 10, 32)
	if err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "无效的账号ID")
		return
	}

	authService := service.NewSocialAuthService(helper.GetDatabase(), helper.GetRedis())
	err = authService.UnbindSocialAccount(userID.(uint), uint(accountID))
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	sc.SuccessWithMessage(c, "解绑成功", nil)
}

// GetUserSocialAccounts 获取当前用户的社交账号列表（需要认证）
// GET /api/user/social-accounts
func (sc SocialAuthController) GetUserSocialAccounts(c httpInterfaces.RouterContextInterface) {
	// 从上下文获取当前用户ID
	userID := c.Get("user_id")
	if userID == nil {
		sc.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}

	accountService := service.NewUserSocialAccountService(helper.GetDatabase())
	accounts, err := accountService.GetUserSocialAccounts(userID.(uint))
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	sc.Success(c, accounts)
}

// BindWithLogin 用社交账号绑定到已有账户
// POST /api/social/bind-with-login
func (sc SocialAuthController) BindWithLogin(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialBindWithLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	authService := service.NewSocialAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.BindWithExistingUser(req.BindToken, req.Username, req.Password)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	// 防御性检查：确保响应不为空
	if response == nil || response.User == nil {
		sc.Error(c, constants.ErrCodeInternal, "绑定失败：响应为空")
		return
	}

	// 如果有 sessionId，标记 OIDC 会话为已认证
	if req.SessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionService.MarkAsAuthenticated(c.Request().Context(), req.SessionID, response.User.ID); err != nil {
			// 记录日志但不阻断流程，因为可能是非OIDC流程或session已过期
			fmt.Printf("警告: 标记OIDC会话失败 (sessionID: %s): %v\n", req.SessionID, err)
			// 对于非OIDC流程，继续返回成功，因为绑定本身已经成功
			// sc.Error(c, constants.ErrCodeInternal, "标记会话失败: "+err.Error())
			// return
		} else {
			// OIDC流程不需要返回JWT token
			response.Token = ""
		}
	}

	sc.SuccessWithMessage(c, "绑定成功", response)
}

// RegisterWithSocial 用社交账号注册新账户
// POST /api/social/register-with-social
func (sc SocialAuthController) RegisterWithSocial(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 验证隐私政策同意
	if !req.AgreePolicy {
		sc.Error(c, constants.ErrCodeBadRequest, "请同意隐私政策和服务条款")
		return
	}

	authService := service.NewSocialAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.RegisterWithSocial(&req)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	// 防御性检查：确保响应不为空
	if response == nil {
		sc.Error(c, constants.ErrCodeInternal, "注册失败：响应为空")
		return
	}

	// 防御性检查：确保用户信息不为空
	if response.User == nil {
		sc.Error(c, constants.ErrCodeInternal, "注册失败：用户信息为空")
		return
	}

	// 如果有 sessionId，标记 OIDC 会话为已认证
	if req.SessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionService.MarkAsAuthenticated(c.Request().Context(), req.SessionID, response.User.ID); err != nil {
			// 记录日志但不阻断流程，因为可能是非OIDC流程或session已过期
			fmt.Printf("警告: 标记OIDC会话失败 (sessionID: %s): %v\n", req.SessionID, err)
			// 对于非OIDC流程，继续返回成功，因为注册本身已经成功
			// sc.Error(c, constants.ErrCodeInternal, "标记会话失败: "+err.Error())
			// return
		} else {
			// OIDC流程不需要返回JWT token
			response.Token = ""
		}
	}

	sc.SuccessWithMessage(c, "注册成功", response)
}

// MiniProgramLogin 处理微信小程序登录（无浏览器回调）
// POST /social/miniprogram/login/:provider
func (sc SocialAuthController) MiniProgramLogin(c httpInterfaces.RouterContextInterface) {
	providerIdentifier := c.Param("provider")
	if providerIdentifier == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "提供商标识符不能为空")
		return
	}

	var req dto.SocialMiniProgramLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	authService := service.NewSocialAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.HandleMiniProgramLogin(providerIdentifier, req.JsCode, req.SessionID)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	if response == nil {
		sc.Error(c, constants.ErrCodeInternal, "登录失败：响应为空")
		return
	}

	// 可选：关联 OIDC 会话
	if req.SessionID != "" && response.User != nil && response.Token != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionService.MarkAsAuthenticated(c.Request().Context(), req.SessionID, response.User.ID); err != nil {
			fmt.Printf("警告: 标记OIDC会话失败 (sessionID: %s): %v\n", req.SessionID, err)
		} else {
			response.Token = ""
		}
	}

	sc.Success(c, response)
}

// ==================== 微信小程序扫码登录（PC 侧发起） ====================

// CreateMPQRCode 创建扫码登录票据（PC 端调用）
// POST /social/miniprogram/qrcode/:provider
func (sc SocialAuthController) CreateMPQRCode(c httpInterfaces.RouterContextInterface) {
	providerIdentifier := c.Param("provider")
	if providerIdentifier == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "提供商标识符不能为空")
		return
	}

	var req dto.MPQRCodeCreateRequest
	_ = c.ShouldBindJSON(&req) // 全部字段可选，允许空 body

	svc := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis())
	resp, err := svc.CreateSession(c.Request().Context(), providerIdentifier, &req)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	sc.Success(c, resp)
}

// GetMPQRCodeStatus PC 端轮询扫码状态
// GET /social/miniprogram/qrcode/status/:ticket
func (sc SocialAuthController) GetMPQRCodeStatus(c httpInterfaces.RouterContextInterface) {
	ticket := c.Param("ticket")
	if ticket == "" {
		sc.Error(c, constants.ErrCodeBadRequest, "ticket 不能为空")
		return
	}

	svc := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis())
	resp, err := svc.GetStatus(c.Request().Context(), ticket)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	// 若已 confirmed 且带 sessionID，顺手把 OIDC 会话标记为已认证。
	if resp.State == service.QRStateConfirmed && resp.SessionID != "" && resp.User != nil && resp.Token != "" {
		sessionSvc := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionSvc.MarkAsAuthenticated(c.Request().Context(), resp.SessionID, resp.User.ID); err != nil {
			fmt.Printf("警告: 标记OIDC会话失败 (sessionID: %s): %v\n", resp.SessionID, err)
		} else {
			resp.Token = ""
		}
	}

	sc.Success(c, resp)
}

// ScanMPQRCode 小程序扫码后上报 jsCode
// POST /social/miniprogram/qrcode/scan
func (sc SocialAuthController) ScanMPQRCode(c httpInterfaces.RouterContextInterface) {
	var req dto.MPQRCodeScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	svc := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis())
	resp, err := svc.Scan(c.Request().Context(), &req)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	sc.Success(c, resp)
}

// ConfirmMPQRCode 小程序端确认/取消
// POST /social/miniprogram/qrcode/confirm
func (sc SocialAuthController) ConfirmMPQRCode(c httpInterfaces.RouterContextInterface) {
	var req dto.MPQRCodeConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	svc := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis())
	if err := svc.Confirm(c.Request().Context(), &req); err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	sc.SuccessWithMessage(c, "操作成功", nil)
}

// escapeJS 转义 JavaScript 字符串中的特殊字符
func escapeJS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}
