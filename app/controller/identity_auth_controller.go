package controller

import (
	"fmt"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type IdentityAuthController struct {
	BaseResponse
}

func (ic IdentityAuthController) GetEnabledProviders(c httpInterfaces.RouterContextInterface) {
	providerService := service.NewIdentityProviderService(helper.GetDatabase())
	providers, err := providerService.GetEnabledProviders()
	if err != nil {
		ic.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	ic.Success(c, providers)
}

func (ic IdentityAuthController) InitiateAuth(c httpInterfaces.RouterContextInterface) {
	providerIdentifier := c.Param("provider")
	if providerIdentifier == "" {
		ic.Error(c, constants.ErrCodeBadRequest, "身份源标识符不能为空")
		return
	}
	sessionID := c.Query("session_id")
	authService := service.NewIdentityAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.GetAuthURLWithSessionID(providerIdentifier, sessionID)
	if err != nil {
		ic.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	c.Redirect(302, response.AuthURL)
}

func (ic IdentityAuthController) HandleCallback(c httpInterfaces.RouterContextInterface) {
	providerIdentifier := c.Param("provider")
	code := c.Query("code")
	if code == "" {
		code = c.Query("authCode")
	}
	state := c.Query("state")
	if providerIdentifier == "" {
		ic.Error(c, constants.ErrCodeBadRequest, "身份源标识符不能为空")
		return
	}
	if code == "" {
		ic.Error(c, constants.ErrCodeBadRequest, "授权码不能为空")
		return
	}
	if state == "" {
		ic.Error(c, constants.ErrCodeBadRequest, "state参数不能为空")
		return
	}

	authService := service.NewIdentityAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.HandleCallback(providerIdentifier, code, state)
	if err != nil {
		ic.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	if response == nil {
		ic.Error(c, constants.ErrCodeInternal, "回调处理失败：响应为空")
		return
	}

	sessionID := response.SessionID
	if response.NeedsBind {
		tempUserInfo, err := authService.GetIdentityUserTempInfo(response.BindToken)
		socialInfo := ""
		if err == nil && tempUserInfo != nil {
			socialInfo = fmt.Sprintf(`
            socialInfo: {
                nickname: '%s',
                avatar: '%s',
                email: '%s',
                source: 'identity'
            },`,
				escapeJS(tempUserInfo.Nickname),
				escapeJS(tempUserInfo.Avatar),
				escapeJS(tempUserInfo.Email))
		}
		ic.postPopupMessage(c, fmt.Sprintf(`
            type: 'oidc-identity-callback',
            needsBind: true,
            bindToken: '%s',
            sessionId: '%s',%s
        `, response.BindToken, sessionID, socialInfo), "账号绑定")
		return
	}

	if sessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionService.MarkAsAuthenticated(c.Request().Context(), sessionID, response.User.ID); err != nil {
			ic.Error(c, constants.ErrCodeInternal, "标记会话失败: "+err.Error())
			return
		}
		ic.postPopupMessage(c, fmt.Sprintf(`
            type: 'oidc-identity-callback',
            needsBind: false,
            sessionId: '%s'
        `, sessionID), "登录成功")
		return
	}

	if response.IsNewUser {
		ic.SuccessWithMessage(c, "注册并登录成功", response)
	} else {
		ic.SuccessWithMessage(c, "登录成功", response)
	}
}

func (ic IdentityAuthController) BindAccount(c httpInterfaces.RouterContextInterface) {
	userID := c.Get("user_id")
	if userID == nil {
		ic.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}
	providerIdentifier := c.Param("provider")
	code := c.Query("code")
	if code == "" {
		code = c.Query("authCode")
	}
	state := c.Query("state")
	if providerIdentifier == "" || code == "" || state == "" {
		ic.Error(c, constants.ErrCodeBadRequest, "身份源标识符、授权码和state不能为空")
		return
	}
	authService := service.NewIdentityAuthService(helper.GetDatabase(), helper.GetRedis())
	if err := authService.BindIdentityAccount(userID.(uint), providerIdentifier, code, state); err != nil {
		ic.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	ic.SuccessWithMessage(c, "绑定成功", nil)
}

func (ic IdentityAuthController) UnbindAccount(c httpInterfaces.RouterContextInterface) {
	userID := c.Get("user_id")
	if userID == nil {
		ic.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}
	accountID, err := strconv.ParseUint(c.Param("accountId"), 10, 32)
	if err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "无效的账号ID")
		return
	}
	authService := service.NewIdentityAuthService(helper.GetDatabase(), helper.GetRedis())
	if err := authService.UnbindIdentityAccount(userID.(uint), uint(accountID)); err != nil {
		ic.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	ic.SuccessWithMessage(c, "解绑成功", nil)
}

func (ic IdentityAuthController) GetUserIdentityAccounts(c httpInterfaces.RouterContextInterface) {
	userID := c.Get("user_id")
	if userID == nil {
		ic.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}
	accountService := service.NewUserIdentityAccountService(helper.GetDatabase())
	accounts, err := accountService.GetUserIdentityAccounts(userID.(uint))
	if err != nil {
		ic.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	ic.Success(c, accounts)
}

func (ic IdentityAuthController) BindWithLogin(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialBindWithLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	authService := service.NewIdentityAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.BindWithExistingUser(req.BindToken, req.Username, req.Password)
	if err != nil {
		ic.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	if response == nil || response.User == nil {
		ic.Error(c, constants.ErrCodeInternal, "绑定失败：响应为空")
		return
	}
	if req.SessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionService.MarkAsAuthenticated(c.Request().Context(), req.SessionID, response.User.ID); err != nil {
			fmt.Printf("警告: 标记OIDC会话失败 (sessionID: %s): %v\n", req.SessionID, err)
		} else {
			response.Token = ""
		}
	}
	ic.SuccessWithMessage(c, "绑定成功", response)
}

func (ic IdentityAuthController) RegisterWithProvider(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if !req.AgreePolicy {
		ic.Error(c, constants.ErrCodeBadRequest, "请同意隐私政策和服务条款")
		return
	}
	authService := service.NewIdentityAuthService(helper.GetDatabase(), helper.GetRedis())
	response, err := authService.RegisterWithProvider(&req)
	if err != nil {
		ic.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	if response == nil || response.User == nil {
		ic.Error(c, constants.ErrCodeInternal, "注册失败：响应为空")
		return
	}
	if req.SessionID != "" {
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		if err := sessionService.MarkAsAuthenticated(c.Request().Context(), req.SessionID, response.User.ID); err != nil {
			fmt.Printf("警告: 标记OIDC会话失败 (sessionID: %s): %v\n", req.SessionID, err)
		} else {
			response.Token = ""
		}
	}
	ic.SuccessWithMessage(c, "注册成功", response)
}

func (ic IdentityAuthController) postPopupMessage(c httpInterfaces.RouterContextInterface, payload string, title string) {
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>%s</title>
</head>
<body>
    <script>
        window.opener.postMessage({%s}, '*');
        window.close();
    </script>
    <p>正在跳转...</p>
</body>
</html>
`, title, payload)
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	c.String(200, html)
}
