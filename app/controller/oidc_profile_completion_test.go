package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const profileVerifier = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNO123456"

type profileFixture struct {
	*accountAuthFixture
	jar       http.CookieJar
	app       *dto.ApplicationResponse
	session   *service.OIDCSession
	login     *model.LoginSession
	oldAccess string
	body      map[string]any
}

func newProfileFixture(t *testing.T, fields, prompt string, remember, registration bool) *profileFixture {
	t.Helper()
	f := newAccountAuthFixture(t)
	router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
	router.POST("/api/oidc/complete-profile", OIDCAuthFlowController{}.CompleteProfile)
	router.POST("/api/oidc/auth/refresh", OIDCAuthFlowController{}.RefreshToken)
	router.POST("/api/oidc/register", OIDCAuthFlowController{}.Register)
	router.POST("/oidc/:client_id/token", OIDCController{}.Token)
	router.GET("/oidc/:client_id/userinfo", OIDCController{}.UserInfo)
	require.NoError(t, f.db.Create(&model.Setting{Key: service.PushSettingsConfigKey, Value: `{"baseUrl":"https://push.invalid","appId":"test","appSecret":"test"}`}).Error)
	app := newConsentTestApplication(t, f)
	jar := accountTestJar(t)
	tr := newConsentTestSession(t, f, jar, app.UUID, "openid "+fields, prompt)
	sessions := service.NewOIDCSessionService(f.redis)
	sum := sha256.Sum256([]byte(profileVerifier))
	tr.CodeChallenge = base64.RawURLEncoding.EncodeToString(sum[:])
	require.NoError(t, sessions.UpdateSession(context.Background(), tr))
	path, username := "/api/oidc/login", f.user.Username
	if registration {
		path, username = "/api/oidc/register", "new_profile_user"
		require.NoError(t, f.db.Create(&model.Setting{Key: service.RegisterFieldsConfigKey, Value: `{"username":{"visible":true,"required":true},"email":{"visible":false,"required":false},"phone":{"visible":false,"required":false},"realName":{"visible":false,"required":false},"nickname":{"visible":false,"required":false}}`}).Error)
	}
	loginData := accountTestData(t, f.request(t, jar, "POST", path, map[string]any{"sessionId": tr.SessionID, "username": username, "password": "correct-password", "rememberMe": remember}))
	require.Equal(t, true, loginData["success"])
	stored, err := sessions.GetSession(context.Background(), tr.SessionID)
	require.NoError(t, err)
	login, err := dao.NewLoginSessionDAO(f.db).Valid(stored.LoginSessionID, *stored.UserID)
	require.NoError(t, err)
	body := map[string]any{"sessionId": tr.SessionID, "sessionRotationSupported": true}
	for _, field := range strings.Fields(fields) {
		value := "profile@example.test"
		if field == "phone" {
			value = "+8613800138000"
		}
		body[field], body[field+"Code"] = value, "123456"
		require.NoError(t, f.redis.Set(context.Background(), "verification:code:"+field+":"+value+":"+tr.SessionID, "123456", time.Minute).Err())
	}
	return &profileFixture{f, jar, app, stored, login, loginData["accessToken"].(string), body}
}

func (f *profileFixture) flow(t *testing.T) map[string]any {
	return accountTestData(t, f.request(t, f.jar, "GET", "/api/oidc/flow-status?session_id="+f.session.SessionID, nil))
}
func (f *profileFixture) complete(t *testing.T) *httptest.ResponseRecorder {
	return f.request(t, f.jar, "POST", "/api/oidc/complete-profile", f.body)
}
func (f *profileFixture) userInfo(access string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", f.origin.String()+"/oidc/"+f.app.UUID+"/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	return rec
}

func TestOIDCProfileCompletionContinuesAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, fields, prompt                  string
		remember, register, legacy, twoFactor bool
	}{
		{name: "registration email auto", fields: "email", register: true},
		{name: "email remembered auto", fields: "email", remember: true},
		{name: "phone manual", fields: "phone", prompt: "consent"},
		{name: "both remembered manual", fields: "email phone", prompt: "consent", remember: true},
		{name: "legacy unknown persistence", fields: "email", remember: true, legacy: true},
		{name: "existing two factor assurance", fields: "email", twoFactor: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProfileFixture(t, tc.fields, tc.prompt, tc.remember, tc.register)
			if tc.legacy {
				require.NoError(t, f.db.Model(f.login).Update("remember_me", nil).Error)
			}
			if tc.twoFactor {
				at := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
				require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", f.login.UserID).Update("two_factor_enabled", true).Error)
				require.NoError(t, f.db.Model(f.login).Updates(map[string]any{"two_factor_at": at, "two_factor_method": "totp"}).Error)
				f.login.TwoFactorAt, f.login.TwoFactorMethod = &at, "totp"
			}
			// A short remaining lifetime catches accidental extension by token issuance.
			until := time.Now().Add(20 * time.Minute).Truncate(time.Second)
			require.NoError(t, f.db.Model(f.login).Update("expires_at", until).Error)
			require.Equal(t, "needs_profile", f.flow(t)["status"])
			oldJar := accountTestJar(t)
			oldJar.SetCookies(f.origin, f.jar.Cookies(f.origin))
			completeRec := f.complete(t)
			complete := accountTestData(t, completeRec)
			require.Equal(t, true, complete["success"])
			require.Equal(t, tc.remember && !tc.legacy, complete["rememberMe"])
			require.NotEmpty(t, complete["accessToken"])
			flow := f.flow(t)
			require.Equal(t, "needs_consent", flow["status"], "profile succeeded but authorization failed: %+v", flow)
			require.Equal(t, tc.prompt == "", flow["autoApprove"] == true)
			stored, err := service.NewOIDCSessionService(f.redis).GetSession(context.Background(), f.session.SessionID)
			require.NoError(t, err)
			require.NotEqual(t, f.session.LoginSessionID, stored.LoginSessionID)
			same := *stored
			same.LoginSessionID = f.session.LoginSessionID
			require.Equal(t, *f.session, same, "authorization parameters, authentication time and expiry must be preserved")
			current, err := dao.NewLoginSessionDAO(f.db).Valid(stored.LoginSessionID, f.login.UserID)
			require.NoError(t, err)
			require.Equal(t, until.UTC(), current.ExpiresAt.UTC())
			require.Equal(t, f.login.LoginMethod, current.LoginMethod)
			require.Equal(t, f.login.TwoFactorMethod, current.TwoFactorMethod)
			if f.login.TwoFactorAt != nil {
				require.True(t, f.login.TwoFactorAt.Equal(*current.TwoFactorAt))
			}
			require.False(t, current.HasAdminAccess)
			for _, cookie := range completeRec.Result().Cookies() {
				if cookie.Value == "" {
					continue
				}
				if cookie.Name == middleware.UserCookieName || cookie.Name == "refresh_token" {
					if tc.remember && !tc.legacy {
						require.Positive(t, cookie.MaxAge)
						require.LessOrEqual(t, cookie.MaxAge, 1200)
					} else {
						require.Zero(t, cookie.MaxAge)
					}
				}
			}
			require.Equal(t, http.StatusUnauthorized, f.request(t, oldJar, "GET", "/api/user/profile", nil).Code)
			require.Equal(t, http.StatusUnauthorized, f.request(t, oldJar, "POST", "/api/oidc/auth/refresh", nil).Code)
			require.Equal(t, http.StatusUnauthorized, f.userInfo(f.oldAccess).Code)
			require.Equal(t, http.StatusOK, f.userInfo(complete["accessToken"].(string)).Code)
			profile := accountTestData(t, f.request(t, f.jar, "GET", "/api/user/profile", nil))
			for _, field := range strings.Fields(tc.fields) {
				require.Equal(t, f.body[field], profile[field])
			}
			refreshed := f.request(t, f.jar, "POST", "/api/oidc/auth/refresh", nil)
			require.NotEmpty(t, accountTestData(t, refreshed)["accessToken"])
			for _, cookie := range refreshed.Result().Cookies() {
				if cookie.Name == "refresh_token" && !(tc.remember && !tc.legacy) {
					require.Zero(t, cookie.MaxAge, "refresh must preserve session-cookie semantics")
				}
			}
			// A duplicate must not rotate again or overwrite credentials from the winner.
			duplicate := f.complete(t)
			require.Equal(t, http.StatusConflict, duplicate.Code)
			require.Empty(t, duplicate.Result().Cookies())
			consent := accountTestData(t, f.request(t, f.jar, "POST", "/api/oidc/consent", map[string]any{"sessionId": stored.SessionID, "approved": true, "csrfToken": flow["csrfToken"]}))
			callback, err := url.Parse(consent["redirectUrl"].(string))
			require.NoError(t, err)
			require.Equal(t, "original-state", callback.Query().Get("state"))
			form := url.Values{"grant_type": {"authorization_code"}, "client_id": {f.app.UUID}, "code": {callback.Query().Get("code")}, "redirect_uri": {"https://rp.example/callback"}, "code_verifier": {profileVerifier}}
			request := httptest.NewRequest("POST", f.origin.String()+"/oidc/"+f.app.UUID+"/token", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			exchange := httptest.NewRecorder()
			f.engine.ServeHTTP(exchange, request)
			require.Equal(t, http.StatusOK, exchange.Code, exchange.Body.String())
			var tokens map[string]any
			require.NoError(t, json.Unmarshal(exchange.Body.Bytes(), &tokens))
			require.NotEmpty(t, tokens["access_token"])
		})
	}
}

func TestOIDCProfileCompletionRejectsInvalidRequests(t *testing.T) {
	for _, name := range []string{"old page", "wrong code", "expired code", "occupied contact", "existing contact", "unrequested contact", "browser binding", "revoked login", "disabled application", "invalid format"} {
		t.Run(name, func(t *testing.T) {
			f := newProfileFixture(t, "email phone", "", false, false)
			want := 400
			switch name {
			case "old page":
				delete(f.body, "sessionRotationSupported")
			case "wrong code":
				f.body["phoneCode"] = "999999"
			case "expired code":
				f.clock.FastForward(2 * time.Minute)
			case "occupied contact":
				email := "profile@example.test"
				require.NoError(t, f.db.Create(&model.User{Username: "another_user", Email: &email, Status: model.UserStatusActive, IsActivated: true}).Error)
			case "existing contact":
				require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", f.login.UserID).Update("email", "already@example.test").Error)
				want = 409
			case "unrequested contact":
				f.session.Scope = "openid phone"
				require.NoError(t, service.NewOIDCSessionService(f.redis).UpdateSession(context.Background(), f.session))
			case "browser binding":
				f.jar.SetCookies(f.origin, []*http.Cookie{{Name: oidcBrowserCookieName(f.session.SessionID), Value: "wrong-binding", Path: "/"}})
				want = 409
			case "revoked login":
				require.NoError(t, service.NewLoginSessionService(f.db).Revoke(f.login.UserID, f.login.ID, "", "logout", nil))
				want = 409
			case "disabled application":
				require.NoError(t, f.db.Model(&model.Application{}).Where("uuid = ?", f.app.UUID).Update("is_active", false).Error)
				want = 403
			case "invalid format":
				f.body["email"] = "bad-email"
			}
			rec := f.complete(t)
			require.Equal(t, want, rec.Code, rec.Body.String())
			require.Empty(t, rec.Result().Cookies())
			user, err := service.NewUserService(f.db).GetUserByID(f.login.UserID)
			require.NoError(t, err)
			require.Empty(t, user.Phone)
			require.NotEqual(t, "profile@example.test", user.Email)
			if name != "revoked login" {
				_, err = dao.NewLoginSessionDAO(f.db).Valid(f.login.ID, f.login.UserID)
				require.NoError(t, err)
			}
			if name == "wrong code" || name == "old page" {
				// Failed validation must not consume either valid code.
				f.body["phoneCode"], f.body["sessionRotationSupported"] = "123456", true
				require.Equal(t, true, accountTestData(t, f.complete(t))["success"])
			}
		})
	}
}

type profileRedisHook struct {
	once  sync.Once
	watch func() error
}

func (h *profileRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (h *profileRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *profileRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		var err error
		if cmd.Name() == "watch" {
			h.once.Do(func() { err = h.watch() })
		}
		if err != nil {
			return err
		}
		return next(ctx, cmd)
	}
}

func TestOIDCProfileCompletionFailureIsClosed(t *testing.T) {
	for _, name := range []string{"redis unavailable", "authorization changed", "authorization deleted", "concurrent revocation", "credential signing"} {
		t.Run(name, func(t *testing.T) {
			f := newProfileFixture(t, "email", "", true, false)
			var pending model.LoginSession
			f.redis.AddHook(&profileRedisHook{watch: func() error {
				require.NoError(t, f.db.Where("activation_pending = ?", true).First(&pending).Error)
				_, err := dao.NewLoginSessionDAO(f.db).Valid(pending.ID, pending.UserID)
				require.Error(t, err, "pending sessions cannot authenticate")
				listed, err := service.NewLoginSessionService(f.db).List(service.LoginSessionQuery{UserID: pending.UserID}, "")
				require.NoError(t, err)
				require.Empty(t, listed.Items)
				jwt, err := service.NewJWTAuthServiceWithError()
				require.NoError(t, err)
				_, err = jwt.GenerateUserToken(&dto.UserResponse{ID: pending.UserID, Username: f.user.Username}, pending.ID)
				require.Error(t, err, "pending sessions cannot mint credentials")
				switch name {
				case "redis unavailable":
					return errors.New("injected Redis failure")
				case "authorization changed":
					require.NoError(t, service.NewOIDCSessionService(f.redis).ResetAuthentication(context.Background(), f.session.SessionID))
				case "authorization deleted":
					require.NoError(t, service.NewOIDCSessionService(f.redis).DeleteSession(context.Background(), f.session.SessionID))
				case "concurrent revocation":
					require.NoError(t, service.NewLoginSessionService(f.db).Revoke(pending.UserID, pending.ID, "", "admin_revoked", nil))
				case "credential signing":
					require.NoError(t, f.db.Exec("CREATE TRIGGER fail_profile_token BEFORE INSERT ON oidc_tokens BEGIN SELECT RAISE(ABORT, 'injected issuance failure'); END").Error)
				}
				return nil
			}})
			data := accountTestData(t, f.complete(t))
			require.Equal(t, false, data["success"])
			require.Empty(t, data["accessToken"])
			failure := data["error"].(map[string]any)
			require.Equal(t, "login_required", failure["code"])
			callback, err := url.Parse(failure["redirectUrl"].(string))
			require.NoError(t, err)
			require.Equal(t, "rp.example", callback.Host)
			require.Equal(t, "original-state", callback.Query().Get("state"))
			user, err := service.NewUserService(f.db).GetUserByID(f.login.UserID)
			require.NoError(t, err)
			require.Equal(t, "profile@example.test", user.Email)
			_, err = dao.NewLoginSessionDAO(f.db).Valid(pending.ID, pending.UserID)
			require.Error(t, err)
			require.Equal(t, http.StatusUnauthorized, f.userInfo(f.oldAccess).Code)
			require.NoError(t, f.db.Exec("DROP TRIGGER IF EXISTS fail_profile_token").Error)
			// A normal new login recovers the saved account; no repeat registration.
			fresh := newConsentTestSession(t, f.accountAuthFixture, f.jar, f.app.UUID, "openid email", "")
			consentTestLogin(t, f.accountAuthFixture, f.jar, fresh.SessionID)
			flow := accountTestData(t, f.request(t, f.jar, "GET", "/api/oidc/flow-status?session_id="+fresh.SessionID, nil))
			require.Equal(t, "needs_consent", flow["status"])
		})
	}
}

func TestOIDCProfileCompletionDatabaseRollback(t *testing.T) {
	f := newProfileFixture(t, "email", "", false, false)
	require.NoError(t, f.db.Exec("CREATE TRIGGER fail_profile_session BEFORE INSERT ON login_sessions WHEN NEW.activation_pending = 1 BEGIN SELECT RAISE(ABORT, 'injected database failure'); END").Error)
	rec := f.complete(t)
	require.Equal(t, 500, rec.Code)
	require.Contains(t, rec.Body.String(), "重新获取验证码")
	require.Empty(t, rec.Result().Cookies())
	user, err := service.NewUserService(f.db).GetUserByID(f.login.UserID)
	require.NoError(t, err)
	require.Empty(t, user.Email)
	_, err = dao.NewLoginSessionDAO(f.db).Valid(f.login.ID, f.login.UserID)
	require.NoError(t, err)
	require.NoError(t, f.db.Exec("DROP TRIGGER fail_profile_session").Error)
	require.Equal(t, 400, f.complete(t).Code, "consumed codes must be requested again after a DB rollback")
}

func TestOIDCProfileCompletionConcurrentSubmission(t *testing.T) {
	f := newProfileFixture(t, "email", "", false, false)
	payload, err := json.Marshal(f.body)
	require.NoError(t, err)
	cookies := f.jar.Cookies(f.origin)
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", f.origin.String()+"/api/oidc/complete-profile", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			for _, c := range cookies {
				req.AddCookie(c)
			}
			rec := httptest.NewRecorder()
			f.engine.ServeHTTP(rec, req)
			results <- rec
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for rec := range results {
		if rec.Code == 200 {
			require.Equal(t, true, accountTestData(t, rec)["success"])
			success++
		} else {
			require.Equal(t, 409, rec.Code, rec.Body.String())
			require.Empty(t, rec.Result().Cookies())
		}
	}
	require.Equal(t, 1, success)
	list, err := service.NewLoginSessionService(f.db).List(service.LoginSessionQuery{UserID: f.login.UserID}, "")
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.NotEqual(t, f.login.ID, list.Items[0].ID)
}
