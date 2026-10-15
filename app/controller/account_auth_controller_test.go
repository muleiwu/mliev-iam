package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
	configimpl "cnb.cool/mliev/open/go-web/pkg/server/config/impl"
	envimpl "cnb.cool/mliev/open/go-web/pkg/server/env/impl"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type accountAuthFixture struct {
	db     *gorm.DB
	redis  *redis.Client
	clock  *miniredis.Miniredis
	engine *gin.Engine
	origin *url.URL
	user   model.User
}

func newAccountAuthFixture(t *testing.T) *accountAuthFixture {
	t.Helper()
	t.Setenv("JWT_SECRET_KEY", strings.Repeat("account-auth-test-", 4))
	db := registerControllerTestDatabase(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Setting{}, &model.User{}, &model.LoginSession{}, &model.Role{}, &model.UserRole{}, &model.CaptchaConfig{}, &model.OIDCToken{}, &model.AuthorizationCode{}, &model.ApplicationOIDCKey{}, &model.TwoFactorMethod{}, &model.TwoFactorEvent{}))
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { client.Close() })
	priority := int(atomic.AddInt64(&controllerTestDBProviderPriority, 1))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Provider](), configimpl.NewConfig(), priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Enver](), envimpl.NewEnv(), priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[*redis.Client](), client, priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), priority))
	salt, hash, err := service.NewUserService(db).GeneratePasswordHash("correct-password")
	require.NoError(t, err)
	user := model.User{Username: "ordinary-user", RealName: "Ordinary User", Status: model.UserStatusActive, IsActivated: true, Salt: salt, PasswordHash: hash}
	require.NoError(t, db.Create(&user).Error)
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.GET("/api/user/auth/authorize", AccountAuthController{}.Authorize)
	router.GET("/api/user/auth/callback", AccountAuthController{}.Callback)
	router.GET("/oidc/:client_id/authorize", OIDCController{}.Authorize)
	router.GET("/api/oidc/session-bind", OIDCAuthFlowController{}.BindSession)
	router.GET("/api/oidc/client-config", OIDCAuthFlowController{}.GetClientConfig)
	router.GET("/api/oidc/flow-status", OIDCAuthFlowController{}.GetFlowStatus)
	router.POST("/api/oidc/login", OIDCAuthFlowController{}.Login)
	router.POST("/api/oidc/consent", OIDCAuthFlowController{}.Consent)
	users := router.Group("/api/user")
	users.Use(middleware.UserAuthMiddleware())
	users.GET("/profile", UserController{}.GetUserProfile)
	admins := router.Group("/api/admin")
	admins.Use(middleware.AdminAuthMiddleware())
	admins.GET("/profile", func(c httpInterfaces.RouterContextInterface) { c.Status(http.StatusNoContent) })
	origin, err := url.Parse("http://iam.test")
	require.NoError(t, err)
	return &accountAuthFixture{db: db, redis: client, clock: server, engine: engine, origin: origin, user: user}
}

func (f *accountAuthFixture) request(t *testing.T, jar http.CookieJar, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	u, err := f.origin.Parse(target)
	require.NoError(t, err)
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, u.String(), bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range jar.Cookies(u) {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	jar.SetCookies(u, rec.Result().Cookies())
	return rec
}

func accountTestJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return jar
}

func accountTestData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	require.Zero(t, body.Code, rec.Body.String())
	return body.Data
}

func accountTestLocation(t *testing.T, rec *httptest.ResponseRecorder) *url.URL {
	t.Helper()
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, rec.Code, rec.Body.String())
	u, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	return u
}

func TestAccountAuthHTTPLoginAndCallback(t *testing.T) {
	for _, keepCookie := range []bool{true, false} {
		t.Run(map[bool]string{true: "preserve remembered cookie", false: "restore missing cookie"}[keepCookie], func(t *testing.T) {
			f := newAccountAuthFixture(t)
			jar := accountTestJar(t)
			authorize := accountTestLocation(t, f.request(t, jar, "GET", "/api/user/auth/authorize?return_to=%2Faccount%3Ftab%3Dsecurity", nil))
			require.Equal(t, service.AccountCenterApplicationUUID, authorize.Query().Get("client_id"))
			require.Equal(t, "S256", authorize.Query().Get("code_challenge_method"))
			bind := accountTestLocation(t, f.request(t, jar, "GET", authorize.String(), nil))
			page := accountTestLocation(t, f.request(t, jar, "GET", bind.String(), nil))
			fragment, err := url.Parse(page.Fragment)
			require.NoError(t, err)
			sessionID := fragment.Query().Get("session_id")
			require.NotEmpty(t, sessionID)
			config := accountTestData(t, f.request(t, jar, "GET", "/api/oidc/client-config?session_id="+sessionID, nil))
			require.Equal(t, false, config["features"].(map[string]any)["registration"])
			login := f.request(t, jar, "POST", "/api/oidc/login", map[string]any{"sessionId": sessionID, "username": f.user.Username, "password": "correct-password", "rememberMe": true})
			require.Equal(t, true, accountTestData(t, login)["success"])
			var remembered *http.Cookie
			for _, cookie := range login.Result().Cookies() {
				if cookie.Name == middleware.UserCookieName {
					remembered = cookie
				}
			}
			require.NotNil(t, remembered)
			require.Positive(t, remembered.MaxAge)
			claims, err := middleware.ValidateUserSessionToken(remembered.Value)
			require.NoError(t, err)
			flow := accountTestData(t, f.request(t, jar, "GET", "/api/oidc/flow-status?session_id="+sessionID, nil))
			require.Equal(t, "needs_consent", flow["status"])
			require.Equal(t, true, flow["autoApprove"])
			consent := accountTestData(t, f.request(t, jar, "POST", "/api/oidc/consent", map[string]any{"sessionId": sessionID, "approved": true, "csrfToken": flow["csrfToken"]}))
			callback := consent["redirectUrl"].(string)
			if !keepCookie {
				jar.SetCookies(f.origin, []*http.Cookie{{Name: middleware.UserCookieName, MaxAge: -1, Path: "/"}})
			}
			rec := f.request(t, jar, "GET", callback, nil)
			require.Equal(t, "/auth/#/account?tab=security", accountTestLocation(t, rec).String())
			for _, cookie := range rec.Result().Cookies() {
				require.NotEqual(t, middleware.AdminCookieName, cookie.Name)
				if keepCookie {
					require.NotEqual(t, middleware.UserCookieName, cookie.Name, "must preserve remember-me expiry")
				}
			}
			profile := accountTestData(t, f.request(t, jar, "GET", "/api/user/profile", nil))
			require.Equal(t, float64(f.user.ID), profile["id"])
			var sid string
			for _, cookie := range jar.Cookies(f.origin) {
				if cookie.Name == middleware.UserCookieName {
					current, err := middleware.ValidateUserSessionToken(cookie.Value)
					require.NoError(t, err)
					sid = current.SessionID
				}
			}
			require.Equal(t, claims.SessionID, sid)
			session, err := dao.NewLoginSessionDAO(f.db).Valid(sid, f.user.ID)
			require.NoError(t, err)
			require.False(t, session.HasAdminAccess)
			var activeTemporaryTokens int64
			require.NoError(t, f.db.Model(&model.OIDCToken{}).Where("authorization_code_id IS NOT NULL AND is_revoked = ?", false).Count(&activeTemporaryTokens).Error)
			require.Zero(t, activeTemporaryTokens)
			require.Equal(t, http.StatusUnauthorized, f.request(t, jar, "GET", "/api/admin/profile", nil).Code)
			replayed := accountTestLocation(t, f.request(t, jar, "GET", callback, nil))
			require.Contains(t, replayed.Fragment, "/account/login?", "callback replay must fail")
		})
	}
}

func TestAccountAuthCallbackRejectsInvalidTransactions(t *testing.T) {
	f := newAccountAuthFixture(t)
	for _, scenario := range []string{"missing state", "wrong browser", "expired", "missing code", "invalid code", "provider error", "wrong callback origin"} {
		t.Run(scenario, func(t *testing.T) {
			jar := accountTestJar(t)
			start := f.request(t, jar, "GET", "/api/user/auth/authorize?return_to=%2Faccount%3Ftab%3Dbindings&force_login=1", nil)
			authorize := accountTestLocation(t, start)
			require.Equal(t, "login", authorize.Query().Get("prompt"))
			state := authorize.Query().Get("state")
			callback := service.AccountCenterCallbackPath + "?state=" + url.QueryEscape(state)
			switch scenario {
			case "missing state":
				callback = service.AccountCenterCallbackPath
			case "wrong browser":
				other := accountTestJar(t)
				u, _ := f.origin.Parse(service.AccountCenterCallbackPath)
				other.SetCookies(u, []*http.Cookie{{Name: accountAuthBrowserCookieName(state), Value: "wrong", Path: accountAuthCookiePath}})
				jar = other
			case "expired":
				f.clock.FastForward(11 * time.Minute)
			case "invalid code":
				callback += "&code=invalid"
			case "provider error":
				callback += "&error=access_denied&error_description=cancelled"
			case "wrong callback origin":
				callback = "http://other.test" + callback + "&code=invalid"
				u, _ := url.Parse(callback)
				jar.SetCookies(u, start.Result().Cookies())
			}
			rec := f.request(t, jar, "GET", callback, nil)
			location := accountTestLocation(t, rec)
			fragment, err := url.Parse(location.Fragment)
			require.NoError(t, err)
			require.Equal(t, "/account/login", fragment.Path)
			require.NotEmpty(t, fragment.Query().Get("error"))
			if scenario == "provider error" || scenario == "missing code" || scenario == "invalid code" || scenario == "wrong callback origin" {
				require.Equal(t, "/account?tab=bindings", fragment.Query().Get("redirect"))
			}
			for _, cookie := range rec.Result().Cookies() {
				require.NotEqual(t, middleware.UserCookieName, cookie.Name)
				require.NotEqual(t, middleware.AdminCookieName, cookie.Name)
			}
			if scenario == "wrong browser" {
				_, err := service.NewAdminAuthFlowService(f.redis).Consume(context.Background(), state, start.Result().Cookies()[0].Value)
				require.Error(t, err, "admin namespace must not consume an account transaction")
				_, err = service.NewFirstPartyAuthFlowService(f.redis, "account").Consume(context.Background(), state, start.Result().Cookies()[0].Value)
				require.NoError(t, err, "wrong browser must not consume the legitimate transaction")
			}
		})
	}
}

func TestSafeAccountReturnTo(t *testing.T) {
	for _, value := range []string{"", "https://evil.example", "//evil.example", "/admin", "/account/login", "/account/../login", "/account#login", "/account\\login", "/account?tab=a\nother=b", "/account?" + strings.Repeat("x", 2048)} {
		require.Equal(t, "/account", safeAccountReturnTo(value))
	}
	for _, value := range []string{"/account", "/account?tab=security", "/account?tab=bindings&label=%E5%AE%89%E5%85%A8"} {
		require.Equal(t, value, safeAccountReturnTo(value))
	}
}
