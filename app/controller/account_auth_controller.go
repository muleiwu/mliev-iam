package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// AccountAuthController connects the personal center to the shared OIDC flow.
type AccountAuthController struct{}

const accountAuthCookiePath = "/api/user/auth"

func accountAuthBrowserCookieName(state string) string {
	digest := sha256.Sum256([]byte(state))
	return "account_oidc_" + hex.EncodeToString(digest[:8])
}

func setAccountAuthBrowserCookie(c httpInterfaces.RouterContextInterface, state, token string, maxAge int) {
	secure := c.Request().TLS != nil || strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(accountAuthBrowserCookieName(state), token, maxAge, accountAuthCookiePath, "", secure, true)
}

func safeAccountReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 2048 || (value != "/account" && !strings.HasPrefix(value, "/account?")) ||
		strings.ContainsAny(value, "#\\") || strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "/account"
	}
	return value
}

func accountAuthErrorURL(message, returnTo string) string {
	query := url.Values{"error": {message}, "redirect": {safeAccountReturnTo(returnTo)}}
	return "/auth/#/account/login?" + query.Encode()
}

// Authorize starts a browser-bound Authorization Code + PKCE transaction.
// GET /api/user/auth/authorize
func (AccountAuthController) Authorize(c httpInterfaces.RouterContextInterface) {
	returnTo := safeAccountReturnTo(c.Query("return_to"))
	if _, err := service.EnsureAccountCenterApplication(helper.GetDatabase()); err != nil {
		helper.GetLogger().Error("初始化个人中心认证应用失败: " + err.Error())
		c.Redirect(http.StatusSeeOther, accountAuthErrorURL("个人中心认证应用初始化失败", returnTo))
		return
	}
	redirectURI := utils.GetBaseURL(c, service.AccountCenterCallbackPath)
	transaction, browserToken, err := service.NewFirstPartyAuthFlowService(helper.GetRedis(), "account").Start(
		c.Request().Context(), redirectURI, returnTo,
	)
	if err != nil {
		helper.GetLogger().Error("创建个人中心统一认证事务失败: " + err.Error())
		c.Redirect(http.StatusSeeOther, accountAuthErrorURL("无法启动统一认证，请稍后重试", returnTo))
		return
	}
	setAccountAuthBrowserCookie(c, transaction.State, browserToken, 10*60)
	query := url.Values{
		"client_id":             {service.AccountCenterApplicationUUID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid profile"},
		"state":                 {transaction.State},
		"nonce":                 {transaction.Nonce},
		"code_challenge":        {transaction.CodeChallenge},
		"code_challenge_method": {"S256"},
	}
	if c.Query("force_login") == "1" {
		query.Set("prompt", "login")
	}
	c.Redirect(http.StatusFound, fmt.Sprintf("/oidc/%s/authorize?%s", service.AccountCenterApplicationUUID, query.Encode()))
}

// Callback consumes the transaction and restores only a user session.
// GET /api/user/auth/callback
func (AccountAuthController) Callback(c httpInterfaces.RouterContextInterface) {
	returnTo := "/account"
	fail := func(message string) {
		c.Redirect(http.StatusSeeOther, accountAuthErrorURL(message, returnTo))
	}
	state := strings.TrimSpace(c.Query("state"))
	if state == "" || len(state) > 128 {
		fail("统一认证回调缺少有效 state")
		return
	}
	browserToken, err := c.Cookie(accountAuthBrowserCookieName(state))
	if err != nil || browserToken == "" {
		fail("统一认证事务与当前浏览器不匹配")
		return
	}
	transaction, err := service.NewFirstPartyAuthFlowService(helper.GetRedis(), "account").Consume(c.Request().Context(), state, browserToken)
	setAccountAuthBrowserCookie(c, state, "", -1)
	if err != nil {
		fail("统一认证事务无效、已使用或已过期，请重新登录")
		return
	}
	returnTo = safeAccountReturnTo(transaction.ReturnTo)
	if providerError := strings.TrimSpace(c.Query("error")); providerError != "" {
		description := strings.TrimSpace(c.Query("error_description"))
		if description == "" {
			description = providerError
		}
		fail(description)
		return
	}
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		fail("统一认证回调缺少授权码")
		return
	}
	if transaction.RedirectURI != utils.GetBaseURL(c, service.AccountCenterCallbackPath) {
		fail("统一认证回调地址不匹配")
		return
	}
	issuer := utils.GetBaseURL(c, "/oidc/"+service.AccountCenterApplicationUUID)
	tokens, err := service.NewOIDCAuthorizationService(helper.GetDatabase()).ExchangeCodeForToken(&dto.TokenRequest{
		GrantType: "authorization_code", Code: code, RedirectURI: transaction.RedirectURI,
		ClientID: service.AccountCenterApplicationUUID, CodeVerifier: transaction.CodeVerifier,
	}, issuer)
	if err != nil {
		helper.GetLogger().Warn("个人中心统一认证授权码交换失败: " + err.Error())
		fail("统一认证授权码无效或已过期")
		return
	}
	tokenDAO := dao.NewOIDCTokenDAO(helper.GetDatabase())
	defer func() {
		if err := tokenDAO.RevokeToken(tokens.AccessToken); err != nil {
			helper.GetLogger().Warn("撤销个人中心临时 OIDC 令牌失败: " + err.Error())
		}
	}()
	token, err := tokenDAO.GetValidByAccessToken(tokens.AccessToken)
	if err != nil || token.ClientID != service.AccountCenterApplicationUUID {
		fail("统一认证令牌无效")
		return
	}
	user, err := service.NewUserService(helper.GetDatabase()).GetUserModelByID(token.UserID)
	if err != nil || !user.CanLogin() {
		fail("账号不存在或不可用")
		return
	}
	profile := dto.ConvertToUserResponse(user)
	if err := EstablishExistingUserSession(c, &profile, token.LoginSessionID); err != nil {
		fail("登录会话已失效，请重新登录")
		return
	}
	c.Redirect(http.StatusSeeOther, "/auth/#"+returnTo)
}
