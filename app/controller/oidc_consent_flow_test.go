package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/stretchr/testify/require"
)

func newConsentTestApplication(t *testing.T, f *accountAuthFixture) *dto.ApplicationResponse {
	t.Helper()
	app, err := service.NewApplicationService(f.db).CreateApplication(&dto.CreateApplicationRequest{
		Name: "Consent test", ProtocolType: "oidc",
		ProtocolConfig: map[string]interface{}{
			"redirectUris":  []interface{}{"https://rp.example/callback"},
			"grantTypes":    []interface{}{"authorization_code", "refresh_token"},
			"responseTypes": []interface{}{"code"}, "scopes": []interface{}{"openid", "profile", "email", "phone"},
			"clientType": "public", "signingAlgorithm": "HS256",
		},
	}, f.user.ID)
	require.NoError(t, err)
	return app
}

func newConsentTestSession(t *testing.T, f *accountAuthFixture, jar http.CookieJar, clientID, scope, prompt string) *service.OIDCSession {
	t.Helper()
	session, err := service.NewOIDCSessionService(f.redis).CreateSession(context.Background(), clientID, "https://rp.example/callback", scope, "original-state", "nonce", "code", strings.Repeat("A", 43), "S256", prompt)
	require.NoError(t, err)
	jar.SetCookies(f.origin, []*http.Cookie{{Name: oidcBrowserCookieName(session.SessionID), Value: session.BrowserBindingToken, Path: "/"}})
	return session
}

func consentTestLogin(t *testing.T, f *accountAuthFixture, jar http.CookieJar, sessionID string) {
	t.Helper()
	login := accountTestData(t, f.request(t, jar, "POST", "/api/oidc/login", map[string]any{
		"sessionId": sessionID, "username": f.user.Username, "password": "correct-password",
	}))
	require.Equal(t, true, login["success"])
}

func TestOIDCConsentInteractiveLoginAndSSO(t *testing.T) {
	f := newAccountAuthFixture(t)
	app := newConsentTestApplication(t, f)
	jar := accountTestJar(t)
	sessions := service.NewOIDCSessionService(f.redis)
	session := newConsentTestSession(t, f, jar, app.UUID, "openid profile", "")
	failed := f.request(t, jar, "POST", "/api/oidc/login", map[string]any{"sessionId": session.SessionID, "username": f.user.Username, "password": "wrong-password"})
	require.Equal(t, http.StatusUnauthorized, failed.Code)
	current, err := sessions.GetSession(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.False(t, current.HasLoginInteraction)
	consentTestLogin(t, f, jar, session.SessionID)
	for range 2 { // Reloads of the same authorization transaction retain the decision.
		flow := accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+session.SessionID, nil))
		require.Equal(t, "needs_consent", flow["status"])
		require.Equal(t, true, flow["autoApprove"])
	}
	consent := accountTestData(t, f.request(t, jar, "POST", "/api/oidc/consent", map[string]any{"sessionId": session.SessionID, "approved": true, "csrfToken": session.CsrfToken}))
	callback, err := url.Parse(consent["redirectUrl"].(string))
	require.NoError(t, err)
	require.NotEmpty(t, callback.Query().Get("code"))
	require.Equal(t, "original-state", callback.Query().Get("state"))

	for _, prompt := range []string{"", "consent", "create", "none"} {
		t.Run("reuse browser login prompt="+prompt, func(t *testing.T) {
			query := url.Values{"client_id": {app.UUID}, "redirect_uri": {"https://rp.example/callback"}, "response_type": {"code"}, "scope": {"openid profile"}, "state": {"sso-state"}, "code_challenge": {strings.Repeat("A", 43)}, "code_challenge_method": {"S256"}, "prompt": {prompt}}
			location := accountTestLocation(t, f.request(t, jar, "GET", "/oidc/"+app.UUID+"/authorize?"+query.Encode(), nil))
			if prompt == "none" {
				require.Equal(t, "rp.example", location.Host)
				require.NotEmpty(t, location.Query().Get("code"))
				require.Equal(t, "sso-state", location.Query().Get("state"))
				return
			}
			page := accountTestLocation(t, f.request(t, jar, "GET", location.String(), nil))
			fragment, err := url.Parse(page.Fragment)
			require.NoError(t, err)
			sid := fragment.Query().Get("session_id")
			flow := accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+sid, nil))
			if prompt == "create" {
				require.Equal(t, "needs_registration_confirmation", flow["status"])
				router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
				router.POST("/api/oidc/registration-choice", OIDCAuthFlowController{}.RegistrationChoice)
				choice := accountTestData(t, f.request(t, jar, "POST", "/api/oidc/registration-choice", map[string]any{"sessionId": sid, "choice": "current", "csrfToken": flow["csrfToken"]}))
				require.Equal(t, true, choice["success"])
				flow = accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+sid, nil))
			}
			require.Equal(t, "needs_consent", flow["status"])
			require.NotEqual(t, true, flow["autoApprove"])
			stored, err := sessions.GetSession(context.Background(), sid)
			require.NoError(t, err)
			require.False(t, stored.HasLoginInteraction)
			if prompt == "create" {
				// Switching away from a previously chosen current account and
				// submitting credentials is a new interactive login.
				router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
				router.POST("/api/oidc/switch-account", OIDCAuthFlowController{}.SwitchAccount)
				accountTestData(t, f.request(t, jar, "POST", "/api/oidc/switch-account", map[string]any{"sessionId": sid, "csrfToken": flow["csrfToken"]}))
				consentTestLogin(t, f, jar, sid)
				flow = accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+sid, nil))
				require.Equal(t, true, flow["autoApprove"])
			}
		})
	}
}

func TestOIDCConsentExplicitPromptAndAccountSwitch(t *testing.T) {
	f := newAccountAuthFixture(t)
	app := newConsentTestApplication(t, f)
	jar := accountTestJar(t)
	session := newConsentTestSession(t, f, jar, app.UUID, "openid profile", "login consent")
	consentTestLogin(t, f, jar, session.SessionID)
	flow := accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+session.SessionID, nil))
	require.Equal(t, "needs_consent", flow["status"])
	require.NotEqual(t, true, flow["autoApprove"])
	router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
	router.POST("/api/oidc/switch-account", OIDCAuthFlowController{}.SwitchAccount)
	accountTestData(t, f.request(t, jar, "POST", "/api/oidc/switch-account", map[string]any{"sessionId": session.SessionID, "csrfToken": flow["csrfToken"]}))
	stored, err := service.NewOIDCSessionService(f.redis).GetSession(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.False(t, stored.HasLoginInteraction)
	require.NotEqual(t, session.CsrfToken, stored.CsrfToken)
	flow = accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+session.SessionID, nil))
	require.Equal(t, "needs_login", flow["status"])
	require.NotEqual(t, true, flow["autoApprove"])
}

func TestOIDCConsentWaitsForProfileCompletion(t *testing.T) {
	f := newAccountAuthFixture(t)
	app := newConsentTestApplication(t, f)
	jar := accountTestJar(t)
	session := newConsentTestSession(t, f, jar, app.UUID, "openid profile email", "")
	consentTestLogin(t, f, jar, session.SessionID)
	flow := accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+session.SessionID, nil))
	require.Equal(t, "needs_profile", flow["status"])
	require.NotEqual(t, true, flow["autoApprove"])
	require.NoError(t, f.db.Model(&f.user).Updates(map[string]any{"email": "user@example.test", "email_verified_at": time.Now()}).Error)
	flow = accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+session.SessionID, nil))
	require.Equal(t, "needs_consent", flow["status"])
	require.Equal(t, true, flow["autoApprove"])
}

func TestOIDCConsentCodeLoginAndRegistration(t *testing.T) {
	for _, method := range []string{"email", "phone", "registration"} {
		t.Run(method, func(t *testing.T) {
			f := newAccountAuthFixture(t)
			app := newConsentTestApplication(t, f)
			jar := accountTestJar(t)
			session := newConsentTestSession(t, f, jar, app.UUID, "openid profile", "")
			router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
			var result map[string]any
			if method == "registration" {
				router.POST("/api/oidc/register", OIDCAuthFlowController{}.Register)
				result = accountTestData(t, f.request(t, jar, "POST", "/api/oidc/register", map[string]any{"sessionId": session.SessionID, "username": "new_user", "password": "correct-password", "email": "new@example.test", "realName": "New User"}))
				require.Equal(t, true, result["autoLogin"])
			} else {
				receiver := "user@example.test"
				if method == "phone" {
					receiver = "+8613800138000"
				}
				require.NoError(t, f.db.Model(&f.user).Updates(map[string]any{method: receiver, method + "_verified_at": time.Now()}).Error)
				require.NoError(t, f.db.Create(&model.Setting{Key: service.PushSettingsConfigKey, Value: `{"baseUrl":"https://push.invalid","appId":"test","appSecret":"test"}`}).Error)
				verificationID := buildLoginVerificationSessionID(session.SessionID, requestVerificationType(method), receiver)
				require.NoError(t, f.redis.Set(context.Background(), fmt.Sprintf("verification:code:%s:%s:%s", method, receiver, verificationID), "123456", time.Minute).Err())
				router.POST("/api/oidc/login-by-code", OIDCAuthFlowController{}.LoginByCode)
				result = accountTestData(t, f.request(t, jar, "POST", "/api/oidc/login-by-code", map[string]any{"sessionId": session.SessionID, "type": method, "receiver": receiver, "code": "123456"}))
			}
			require.Equal(t, true, result["success"])
			flow := accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+session.SessionID, nil))
			require.Equal(t, "needs_consent", flow["status"])
			require.Equal(t, true, flow["autoApprove"])
		})
	}
}

func TestOIDCConsentVerifiedExternalAndPasskeyCompletion(t *testing.T) {
	// Upstream provider/WebAuthn verification is covered by its own tests. These
	// routes exercise the production completion gates with a verified identity.
	for _, method := range []string{"social", "enterprise", "miniprogram", "passkey"} {
		t.Run(method, func(t *testing.T) {
			f := newAccountAuthFixture(t)
			app := newConsentTestApplication(t, f)
			jar := accountTestJar(t)
			session := newConsentTestSession(t, f, jar, app.UUID, "openid profile", "")
			router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
			router.POST("/external-auth/test-completion", func(c httpInterfaces.RouterContextInterface) {
				user, err := service.NewUserService(f.db).GetUserByID(f.user.ID)
				require.NoError(t, err)
				if method == "passkey" {
					c.Set("primary_auth_method", "passkey")
					c.Set("primary_passkey_proof", consentTestPasskeyProof(t, f, session.SessionID))
					result, err := (OIDCAuthFlowController{}).completeOIDCLogin(c, service.NewOIDCSessionService(f.redis), session, session.SessionID, user, oidcLoginOptions{})
					require.NoError(t, err)
					BaseResponse{}.Success(c, result)
					return
				}
				ExternalAuthController{}.completeLoginResponse(c, &dto.SocialAuthCallbackResponse{User: user, SessionID: session.SessionID}, session.SessionID, "登录成功")
			})
			accountTestData(t, f.request(t, jar, "POST", "/external-auth/test-completion", nil))
			flow := accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+session.SessionID, nil))
			require.Equal(t, "needs_consent", flow["status"])
			require.Equal(t, true, flow["autoApprove"])
		})
	}
}
