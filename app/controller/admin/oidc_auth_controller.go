package admin

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gorm.io/gorm"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

const adminAuthCookiePath = "/api/admin/auth"

func requestIsSecure(c httpInterfaces.RouterContextInterface) bool {
	return c.Request().TLS != nil || strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}

func adminAuthBrowserCookieName(state string) string {
	digest := sha256.Sum256([]byte(state))
	return "admin_oidc_" + hex.EncodeToString(digest[:8])
}

func setAdminAuthBrowserCookie(c httpInterfaces.RouterContextInterface, state, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(adminAuthBrowserCookieName(state), token, 10*60, adminAuthCookiePath, "", requestIsSecure(c), true)
}

func clearAdminAuthBrowserCookie(c httpInterfaces.RouterContextInterface, state string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(adminAuthBrowserCookieName(state), "", -1, adminAuthCookiePath, "", requestIsSecure(c), true)
}

func safeAdminReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/dashboard"
	}
	if len(value) > 2048 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "#\\") {
		return "/dashboard"
	}
	return value
}

func adminAuthErrorURL(message string) string {
	return "/admin/#/auth/login?error=" + url.QueryEscape(message)
}

// Authorize starts the first-party OIDC Authorization Code + PKCE flow.
// GET /api/admin/auth/authorize
func (ac AuthController) Authorize(c httpInterfaces.RouterContextInterface) {
	app, err := service.NewApplicationService(helper.GetDatabase()).GetApplicationByClientID(service.AdminConsoleApplicationUUID)
	if err != nil || !service.IsAdminConsoleApplication(app) {
		app, err = service.EnsureAdminConsoleApplication(helper.GetDatabase(), 0)
		if err != nil {
			c.Redirect(http.StatusSeeOther, adminAuthErrorURL("管理后台认证应用初始化失败"))
			return
		}
	}

	redirectURI := utils.GetBaseURL(c, service.AdminConsoleCallbackPath)
	transaction, browserToken, err := service.NewAdminAuthFlowService(helper.GetRedis()).Start(
		c.Request().Context(),
		redirectURI,
		safeAdminReturnTo(c.Query("return_to")),
	)
	if err != nil {
		helper.GetLogger().Error("创建后台统一认证事务失败: " + err.Error())
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("无法启动统一认证，请稍后重试"))
		return
	}
	setAdminAuthBrowserCookie(c, transaction.State, browserToken)

	query := url.Values{
		"client_id":             {service.AdminConsoleApplicationUUID},
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
	authorizeURL := fmt.Sprintf("/oidc/%s/authorize?%s", service.AdminConsoleApplicationUUID, query.Encode())
	c.Redirect(http.StatusFound, authorizeURL)
}

// Callback consumes the one-time authorization transaction and establishes an
// administrator-only HttpOnly cookie session.
// GET /api/admin/auth/callback
func (ac AuthController) Callback(c httpInterfaces.RouterContextInterface) {
	state := strings.TrimSpace(c.Query("state"))
	if state == "" {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("统一认证回调缺少 state"))
		return
	}
	browserToken, err := c.Cookie(adminAuthBrowserCookieName(state))
	if err != nil || browserToken == "" {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("统一认证事务与当前浏览器不匹配"))
		return
	}
	transaction, err := service.NewAdminAuthFlowService(helper.GetRedis()).Consume(c.Request().Context(), state, browserToken)
	clearAdminAuthBrowserCookie(c, state)
	if err != nil {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL(err.Error()))
		return
	}
	if providerError := strings.TrimSpace(c.Query("error")); providerError != "" {
		description := strings.TrimSpace(c.Query("error_description"))
		if description == "" {
			description = providerError
		}
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL(description))
		return
	}
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("统一认证回调缺少授权码"))
		return
	}
	if transaction.RedirectURI != utils.GetBaseURL(c, service.AdminConsoleCallbackPath) {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("统一认证回调地址不匹配"))
		return
	}

	issuer := utils.GetBaseURL(c, fmt.Sprintf("/oidc/%s", service.AdminConsoleApplicationUUID))
	tokenResponse, err := service.NewOIDCAuthorizationService(helper.GetDatabase()).ExchangeCodeForToken(&dto.TokenRequest{
		GrantType:    "authorization_code",
		Code:         code,
		RedirectURI:  transaction.RedirectURI,
		ClientID:     service.AdminConsoleApplicationUUID,
		CodeVerifier: transaction.CodeVerifier,
	}, issuer)
	if err != nil {
		helper.GetLogger().Warn("后台统一认证授权码交换失败: " + err.Error())
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("统一认证授权码无效或已过期"))
		return
	}

	tokenDAO := dao.NewOIDCTokenDAO(helper.GetDatabase())
	token, err := tokenDAO.GetValidByAccessToken(tokenResponse.AccessToken)
	if err != nil || token.ClientID != service.AdminConsoleApplicationUUID {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("统一认证令牌无效"))
		return
	}
	defer func() {
		if err := tokenDAO.RevokeToken(tokenResponse.AccessToken); err != nil {
			helper.GetLogger().Warn("撤销后台临时 OIDC 令牌失败: " + err.Error())
		}
	}()

	isAdmin, err := service.IsSuperAdmin(helper.GetDatabase(), token.UserID)
	if err != nil {
		helper.GetLogger().Error("检查后台管理员角色失败: " + err.Error())
	}
	if err != nil || !isAdmin {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("当前账号不是有效的超级管理员，请使用其他账号"))
		return
	}
	user, err := service.NewUserService(helper.GetDatabase()).GetUserByID(token.UserID)
	if err != nil {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("管理员账号不存在或不可用"))
		return
	}
	jwtService, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("后台会话配置错误"))
		return
	}
	adminToken, err := jwtService.GenerateAdminToken(user, token.LoginSessionID)
	if err != nil {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("创建后台会话失败"))
		return
	}
	if err := helper.GetDatabase().Transaction(func(tx *gorm.DB) error {
		if err := dao.NewLoginSessionDAO(tx).Lock(token.LoginSessionID, user.ID); err != nil {
			return err
		}
		if err := tx.Model(&model.LoginSession{}).Where("id = ?", token.LoginSessionID).Update("has_admin_access", true).Error; err != nil {
			return err
		}
		return dao.NewLoginSessionDAO(tx).Extend(token.LoginSessionID, time.Now().Add(jwtService.GetConfig().AccessTokenTTL))
	}); err != nil {
		c.Redirect(http.StatusSeeOther, adminAuthErrorURL("登录会话已失效"))
		return
	}
	setAdminAccessCookie(c, adminToken, jwtService.GetConfig().GetAccessTokenHours()*3600)
	c.Redirect(http.StatusSeeOther, "/admin/#"+safeAdminReturnTo(transaction.ReturnTo))
}
