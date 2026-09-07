package controller

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type ExternalAuthController struct{ BaseResponse }

const externalContinuationCookiePrefix = "external_continuation_"

func continuationCookieName(csrf string) string {
	digest := sha256.Sum256([]byte(csrf))
	return externalContinuationCookiePrefix + hex.EncodeToString(digest[:8])
}

func setContinuationCookie(c httpInterfaces.RouterContextInterface, csrf, continuationID string) {
	isSecure := c.Request().TLS != nil
	if isSecure {
		c.SetSameSite(http.SameSiteNoneMode)
	} else {
		c.SetSameSite(http.SameSiteLaxMode)
	}
	c.SetCookie(continuationCookieName(csrf), continuationID, 15*60, "/external-auth", "", isSecure, true)
}

func clearContinuationCookie(c httpInterfaces.RouterContextInterface, csrf string) {
	c.SetCookie(continuationCookieName(csrf), "", -1, "/external-auth", "", c.Request().TLS != nil, true)
}

func trustedOpenerOrigin(c httpInterfaces.RouterContextInterface) (string, error) {
	scheme := "http"
	if c.Request().TLS != nil || strings.EqualFold(c.Request().Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	defaultOrigin := scheme + "://" + c.Request().Host
	requested := strings.TrimSpace(c.Query("opener_origin"))
	if requested == "" {
		return defaultOrigin, nil
	}
	parsed, err := url.Parse(requested)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("opener origin 无效")
	}
	originHeader := strings.TrimSpace(c.Request().Header.Get("Origin"))
	if originHeader != "" && originHeader != requested {
		return "", fmt.Errorf("opener origin 与请求来源不匹配")
	}
	return requested, nil
}

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
	sessionID := c.Query("session_id")
	if sessionID == "" {
		ec.Error(c, constants.ErrCodeBadRequest, "缺少 OIDC session_id")
		return
	}
	session, err := getBoundOIDCSession(c, service.NewOIDCSessionService(helper.GetRedis()), sessionID)
	if err != nil {
		ec.Error(c, constants.ErrCodeUnauthorized, err.Error())
		return
	}
	openerOrigin, err := trustedOpenerOrigin(c)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	response, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).GetAuthURL(identifier, sessionID, openerOrigin, session.BrowserBindingHash)
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
	openerOrigin, err := trustedOpenerOrigin(c)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	response, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).GetBindAuthURL(c.Param("identifier"), userID, openerOrigin)
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
	stateData, valid := authService.ConsumeState(c.Request().Context(), state)
	if !valid {
		ec.Error(c, constants.ErrCodeBadRequest, "无效或已使用的 state")
		return
	}
	if stateData.ConnectionIdentifier != identifier {
		ec.Error(c, constants.ErrCodeBadRequest, "OAuth state 与回调连接不匹配")
		return
	}
	if stateData.IsBind() {
		err := authService.BindWithCode(stateData.BindUserID, identifier, code)
		ec.popup(c, map[string]any{"type": "external-bind-callback", "success": err == nil, "error": errorText(err)}, stateData.OpenerOrigin, "账号绑定")
		return
	}
	boundSession, err := getBoundOIDCSession(c, service.NewOIDCSessionService(helper.GetRedis()), stateData.SessionID)
	if err != nil || boundSession.BrowserBindingHash != stateData.BrowserBindingHash {
		ec.Error(c, constants.ErrCodeUnauthorized, "OAuth 回调浏览器事务校验失败")
		return
	}
	response, err := authService.HandleCallback(identifier, code, stateData.SessionID)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	if response.NeedsBind {
		temp, _ := authService.GetTempProfile(response.ContinuationID)
		externalInfo := map[string]any{}
		if temp != nil && temp.Profile != nil {
			externalInfo = map[string]any{"nickname": temp.Profile.DisplayName, "avatar": temp.Profile.Avatar, "email": temp.Profile.Email, "category": temp.Category}
		}
		setContinuationCookie(c, response.ContinuationCSRF, response.ContinuationID)
		ec.popup(c, map[string]any{"type": "oidc-external-callback", "needsBind": true, "continuationCsrf": response.ContinuationCSRF, "sessionId": response.SessionID, "externalInfo": externalInfo}, stateData.OpenerOrigin, "账号绑定")
		return
	}
	if response.SessionID != "" {
		failure, err := terminalOIDCFailure(c, boundSession, response.User.ID)
		if err != nil {
			ec.Error(c, constants.ErrCodeInternal, err.Error())
			return
		}
		if failure != nil {
			ec.popup(c, map[string]any{
				"type":      "oidc-external-callback",
				"needsBind": false,
				"sessionId": response.SessionID,
				"error":     failure,
			}, stateData.OpenerOrigin, "登录失败")
			return
		}
		if err := completeUserLogin(c, response.User, false, response.SessionID); err != nil {
			ec.Error(c, 500, "创建登录会话失败: "+err.Error())
			return
		}
		ec.popup(c, map[string]any{"type": "oidc-external-callback", "needsBind": false, "sessionId": response.SessionID}, stateData.OpenerOrigin, "登录成功")
		return
	}
	if err := completeUserLogin(c, response.User, false, response.SessionID); err != nil {
		ec.Error(c, 500, "创建登录会话失败: "+err.Error())
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
	continuationID, cookieErr := c.Cookie(continuationCookieName(req.ContinuationCSRF))
	if cookieErr != nil || continuationID == "" {
		ec.Error(c, constants.ErrCodeUnauthorized, "continuation Cookie 不存在或已过期")
		return
	}
	authService := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis())
	if err := authService.CheckContinuationRateLimit(c.Request().Context(), continuationID, c.ClientIP(), req.Username); err != nil {
		ec.Error(c, constants.ErrCodeTooManyRequests, err.Error())
		return
	}
	if err := captcha.NewManager(helper.GetDatabase()).ValidateForScene("login", req.CaptchaTicket, c.ClientIP(), helper.GetRedis()); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	response, err := authService.BindWithExistingUser(c.Request().Context(), continuationID, req.ContinuationCSRF, req.Username, req.Password)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	clearContinuationCookie(c, req.ContinuationCSRF)
	ec.completeLoginResponse(c, response, response.SessionID, "绑定成功")
}

func (ec ExternalAuthController) Register(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	continuationID, cookieErr := c.Cookie(continuationCookieName(req.ContinuationCSRF))
	if cookieErr != nil || continuationID == "" {
		ec.Error(c, constants.ErrCodeUnauthorized, "continuation Cookie 不存在或已过期")
		return
	}
	authService := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis())
	if err := authService.CheckContinuationRateLimit(c.Request().Context(), continuationID, c.ClientIP(), req.Username); err != nil {
		ec.Error(c, constants.ErrCodeTooManyRequests, err.Error())
		return
	}
	if err := captcha.NewManager(helper.GetDatabase()).ValidateForScene("register", req.CaptchaTicket, c.ClientIP(), helper.GetRedis()); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	response, err := authService.Register(c.Request().Context(), continuationID, &req)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	clearContinuationCookie(c, req.ContinuationCSRF)
	ec.completeLoginResponse(c, response, response.SessionID, "注册成功")
}

func (ec ExternalAuthController) MiniProgramLogin(c httpInterfaces.RouterContextInterface) {
	var req dto.SocialMiniProgramLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	var boundSession *service.OIDCSession
	if req.SessionID != "" {
		var err error
		boundSession, err = getBoundOIDCSession(c, service.NewOIDCSessionService(helper.GetRedis()), req.SessionID)
		if err != nil {
			ec.Error(c, constants.ErrCodeUnauthorized, err.Error())
			return
		}
	}
	response, err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).HandleMiniProgramLogin(c.Param("identifier"), req.JsCode, req.SessionID)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	if response.User != nil {
		if boundSession != nil {
			failure, err := terminalOIDCFailure(c, boundSession, response.User.ID)
			if err != nil {
				ec.Error(c, constants.ErrCodeInternal, err.Error())
				return
			}
			if failure != nil {
				response.Token = ""
				response.Error = failure
				ec.Success(c, response)
				return
			}
		}
		if err := completeUserLogin(c, response.User, false, response.SessionID); err != nil {
			ec.Error(c, 500, "创建登录会话失败: "+err.Error())
			return
		}
	}
	if token, ok := c.Get("login_response_token").(string); ok && response.SessionID == "" {
		response.Token = token
	}
	ec.Success(c, response)
}

func (ec ExternalAuthController) CreateMiniProgramQRCode(c httpInterfaces.RouterContextInterface) {
	var req dto.MPQRCodeCreateRequest
	_ = c.ShouldBindJSON(&req)
	if req.SessionID != "" {
		if _, err := getBoundOIDCSession(c, service.NewOIDCSessionService(helper.GetRedis()), req.SessionID); err != nil {
			ec.Error(c, constants.ErrCodeUnauthorized, err.Error())
			return
		}
	}
	response, err := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis()).CreateSession(c.Request().Context(), c.Param("identifier"), &req)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	ec.Success(c, response)
}

func (ec ExternalAuthController) GetMiniProgramQRCodeStatus(c httpInterfaces.RouterContextInterface) {
	pollToken := c.Request().Header.Get("X-QR-Poll-Token")
	response, err := service.NewSocialQRCodeService(helper.GetDatabase(), helper.GetRedis()).GetStatusForConnection(c.Request().Context(), c.Param("identifier"), c.Param("scanHandle"), pollToken)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	if response.State == service.QRStateConfirmed && response.SessionID != "" && response.User != nil {
		boundSession, err := getBoundOIDCSession(c, service.NewOIDCSessionService(helper.GetRedis()), response.SessionID)
		if err != nil {
			ec.Error(c, constants.ErrCodeUnauthorized, err.Error())
			return
		}
		failure, err := terminalOIDCFailure(c, boundSession, response.User.ID)
		if err != nil {
			ec.Error(c, constants.ErrCodeInternal, err.Error())
			return
		}
		if failure != nil {
			response.State = oidcAccessDeniedCode
			response.Error = failure
			response.User = nil
			ec.Success(c, response)
			return
		}
	}
	if response.State == service.QRStateConfirmed && response.User != nil {
		if err := completeUserLogin(c, response.User, false, response.SessionID); err != nil {
			ec.Error(c, 500, "创建登录会话失败: "+err.Error())
			return
		}
	}
	if response.State == service.QRStateNeedsBind && response.ContinuationID != "" {
		setContinuationCookie(c, response.ContinuationCSRF, response.ContinuationID)
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
		sessionService := service.NewOIDCSessionService(helper.GetRedis())
		session, err := getBoundOIDCSession(c, sessionService, sessionID)
		if err != nil {
			ec.Error(c, constants.ErrCodeUnauthorized, err.Error())
			return
		}
		failure, err := terminalOIDCFailure(c, session, response.User.ID)
		if err != nil {
			ec.Error(c, constants.ErrCodeInternal, err.Error())
			return
		}
		response.Token = ""
		if failure != nil {
			response.Error = failure
			if response.IsNewUser {
				message = "注册成功，但" + failure.Description
			} else {
				message = failure.Description
			}
			ec.SuccessWithMessage(c, message, response)
			return
		}
	}
	if err := completeUserLogin(c, response.User, false, response.SessionID); err != nil {
		ec.Error(c, 500, "创建登录会话失败: "+err.Error())
		return
	}
	if token, ok := c.Get("login_response_token").(string); ok && response.SessionID == "" {
		response.Token = token
	}
	ec.SuccessWithMessage(c, message, response)
}

func terminalOIDCFailure(
	c httpInterfaces.RouterContextInterface,
	session *service.OIDCSession,
	userID uint,
) (*dto.AuthorizationFailure, error) {
	_, failure, err := checkOIDCUserAccess(session, userID)
	if err != nil {
		failure, err = newOIDCFailure(session, "server_error", "检查应用访问权限失败")
		if err != nil {
			return nil, err
		}
	}
	if failure == nil {
		return nil, nil
	}
	sessionService := service.NewOIDCSessionService(helper.GetRedis())
	if err := markOIDCSessionFailed(c.Request().Context(), sessionService, session.SessionID, failure); err != nil {
		return nil, err
	}
	return failure, nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (ec ExternalAuthController) popup(c httpInterfaces.RouterContextInterface, payload map[string]any, targetOrigin, title string) {
	payloadJSON, _ := json.Marshal(payload)
	targetJSON, _ := json.Marshal(targetOrigin)
	titleJSON, _ := json.Marshal(title)
	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		ec.Error(c, constants.ErrCodeInternal, "生成回调页面安全随机数失败")
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	const document = `<!doctype html><html><head><meta charset="utf-8"><title></title></head><body><script nonce="{{.Nonce}}">document.title={{.Title}};if(window.opener){window.opener.postMessage({{.Payload}},{{.TargetOrigin}})}window.close()</script><p>正在跳转…</p></body></html>`
	view := struct {
		Nonce        string
		Title        template.JS
		Payload      template.JS
		TargetOrigin template.JS
	}{Nonce: nonce, Title: template.JS(titleJSON), Payload: template.JS(payloadJSON), TargetOrigin: template.JS(targetJSON)}
	var output bytes.Buffer
	if err := template.Must(template.New("external-auth-callback").Parse(document)).Execute(&output, view); err != nil {
		ec.Error(c, constants.ErrCodeInternal, "生成回调页面失败")
		return
	}
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	c.SetHeader("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; base-uri 'none'; frame-ancestors 'none'")
	c.String(200, "%s", output.String())
}
