package controller

import (
	"fmt"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type ExternalAuthController struct{ BaseResponse }

func (ec ExternalAuthController) GetConnections(c httpInterfaces.RouterContextInterface) {
	connections, err := service.NewIdentityConnectionService(helper.GetDatabase()).PublicList()
	if err != nil {
		ec.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	ec.Success(c, connections)
}

func (ec ExternalAuthController) Authorize(c httpInterfaces.RouterContextInterface) {
	identifier := c.Param("identifier")
	response, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).GetAuthURL(identifier, c.Query("session_id"))
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	c.Redirect(302, response.AuthURL)
}

func (ec ExternalAuthController) Bind(c httpInterfaces.RouterContextInterface) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		ec.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}
	response, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).GetBindAuthURL(c.Param("identifier"), userID)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	c.Redirect(302, response.AuthURL)
}

func (ec ExternalAuthController) Callback(c httpInterfaces.RouterContextInterface) {
	identifier, code, state := c.Param("identifier"), c.Query("code"), c.Query("state")
	if identifier == "" || code == "" || state == "" {
		ec.Error(c, constants.ErrCodeBadRequest, "连接标识、授权码和 state 不能为空")
		return
	}
	authService := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis())
	stateData, valid := authService.ConsumeState(state)
	if !valid {
		ec.Error(c, constants.ErrCodeBadRequest, "无效或已使用的 state")
		return
	}
	if stateData.IsBind() {
		redirectBindResult(c, authService.BindWithCode(stateData.BindUserID, identifier, code))
		return
	}
	response, err := authService.HandleCallback(identifier, code, stateData.SessionID)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	if response.NeedsBind {
		temp, _ := authService.GetTempProfile(response.BindToken)
		profilePayload := ""
		if temp != nil && temp.Profile != nil {
			profilePayload = fmt.Sprintf(`externalInfo: { nickname: '%s', avatar: '%s', email: '%s', category: '%s' },`,
				escapeJS(temp.Profile.DisplayName), escapeJS(temp.Profile.Avatar), escapeJS(temp.Profile.Email), escapeJS(string(temp.Category)))
		}
		ec.popup(c, fmt.Sprintf(`type: 'oidc-external-callback', needsBind: true, bindToken: '%s', sessionId: '%s', %s`,
			escapeJS(response.BindToken), escapeJS(response.SessionID), profilePayload), "账号绑定")
		return
	}
	issueUserSessionCookie(c, response.User, false)
	if response.SessionID != "" {
		if err := service.NewOIDCSessionService(helper.GetRedis()).MarkAsAuthenticated(c.Request().Context(), response.SessionID, response.User.ID); err != nil {
			ec.Error(c, constants.ErrCodeInternal, err.Error())
			return
		}
		ec.popup(c, fmt.Sprintf(`type: 'oidc-external-callback', needsBind: false, sessionId: '%s'`, escapeJS(response.SessionID)), "登录成功")
		return
	}
	ec.SuccessWithMessage(c, "登录成功", response)
}

func (ec ExternalAuthController) BindWithLogin(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialBindWithLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	response, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).BindWithExistingUser(req.BindToken, req.Username, req.Password)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	ec.completeLoginResponse(c, response, req.SessionID, "绑定成功")
}

func (ec ExternalAuthController) Register(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	response, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).Register(&req)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	ec.completeLoginResponse(c, response, req.SessionID, "注册成功")
}

func (ec ExternalAuthController) MiniProgramLogin(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialMiniProgramLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	response, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).HandleMiniProgramLogin(c.Param("identifier"), req.JsCode, req.SessionID)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	if response.User != nil {
		issueUserSessionCookie(c, response.User, false)
	}
	ec.Success(c, response)
}

func (ec ExternalAuthController) CreateMiniProgramQRCode(c httpInterfaces.RouterContextInterface) {
	var req dto.MPQRCodeCreateRequest
	_ = c.ShouldBindJSON(&req)
	response, err := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis()).CreateSession(c.Request().Context(), c.Param("identifier"), &req)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	ec.Success(c, response)
}

func (ec ExternalAuthController) GetMiniProgramQRCodeStatus(c httpInterfaces.RouterContextInterface) {
	response, err := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis()).GetStatusForConnection(c.Request().Context(), c.Param("identifier"), c.Param("ticket"))
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	if response.State == service.QRStateConfirmed && response.SessionID != "" && response.User != nil {
		if err := service.NewOIDCSessionService(helper.GetRedis()).MarkAsAuthenticated(c.Request().Context(), response.SessionID, response.User.ID); err == nil {
			response.Token = ""
		}
	}
	if response.State == service.QRStateConfirmed && response.User != nil {
		issueUserSessionCookie(c, response.User, false)
	}
	ec.Success(c, response)
}

func (ec ExternalAuthController) ScanMiniProgramQRCode(c httpInterfaces.RouterContextInterface) {
	var req dto.MPQRCodeScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	response, err := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis()).ScanForConnection(c.Request().Context(), c.Param("identifier"), &req)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	ec.Success(c, response)
}

func (ec ExternalAuthController) ConfirmMiniProgramQRCode(c httpInterfaces.RouterContextInterface) {
	var req dto.MPQRCodeConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis()).ConfirmForConnection(c.Request().Context(), c.Param("identifier"), &req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	ec.SuccessWithMessage(c, "操作成功", nil)
}

func (ec ExternalAuthController) ListUserIdentities(c httpInterfaces.RouterContextInterface) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		ec.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}
	identities, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).ListUserIdentities(userID)
	if err != nil {
		ec.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	ec.Success(c, identities)
}

func (ec ExternalAuthController) DeleteUserIdentity(c httpInterfaces.RouterContextInterface) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		ec.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "无效的外部身份 ID")
		return
	}
	if err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).Unbind(userID, uint(id)); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	ec.SuccessWithMessage(c, "解绑成功", nil)
}

func (ec ExternalAuthController) completeLoginResponse(c httpInterfaces.RouterContextInterface, response *dto.SocialAuthCallbackResponse, sessionID, message string) {
	if response == nil || response.User == nil {
		ec.Error(c, constants.ErrCodeInternal, "认证响应为空")
		return
	}
	if sessionID != "" {
		if err := service.NewOIDCSessionService(helper.GetRedis()).MarkAsAuthenticated(c.Request().Context(), sessionID, response.User.ID); err == nil {
			response.Token = ""
		}
	}
	issueUserSessionCookie(c, response.User, false)
	ec.SuccessWithMessage(c, message, response)
}

func (ec ExternalAuthController) popup(c httpInterfaces.RouterContextInterface, payload, title string) {
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	c.String(200, fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><title>%s</title></head><body><script>window.opener.postMessage({%s}, '*');window.close();</script><p>正在跳转…</p></body></html>`, escapeJS(title), payload))
}
