package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"cnb.cool/mliev/open/go-web/pkg/container"
	envimpl "cnb.cool/mliev/open/go-web/pkg/server/env/impl"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTwoFactorHTTPLoginEnrollmentOIDCAndExternal(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", strings.Repeat("two-factor-controller-", 4))
	t.Setenv(credentialcrypto.EnvironmentKeys, "")
	t.Setenv(credentialcrypto.EnvironmentKey, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("t", 32))))
	db := registerControllerTestDatabase(t)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.Setting{}, &model.User{}, &model.LoginSession{}, &model.Role{}, &model.UserRole{}, &model.CaptchaConfig{}, &model.OIDCToken{}, &model.AuthorizationCode{}, &model.TwoFactorMethod{}, &model.TwoFactorRecoveryCode{}, &model.TwoFactorEvent{}, &model.ApplicationOIDCKey{}, &model.ExternalPrincipal{}, &model.IdentityConnection{}, &model.Passkey{}))
	server := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { redisClient.Close() })
	priority := int(atomic.AddInt64(&controllerTestDBProviderPriority, 1))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Enver](), envimpl.NewEnv(), priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[*redis.Client](), redisClient, priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), priority))
	userService := service.NewUserService(db)
	salt, hash, err := userService.GeneratePasswordHash("correct-password")
	require.NoError(t, err)
	user := model.User{Username: "http-factor-user", IsActivated: true, Status: model.UserStatusActive, Salt: salt, PasswordHash: hash}
	require.NoError(t, db.Create(&user).Error)
	app, err := service.NewApplicationService(db).CreateApplication(&dto.CreateApplicationRequest{Name: "2FA application", ProtocolType: "oidc", ProtocolConfig: map[string]interface{}{"redirectUris": []interface{}{"https://rp.example/callback"}, "grantTypes": []interface{}{"authorization_code", "refresh_token"}, "responseTypes": []interface{}{"code"}, "scopes": []interface{}{"openid", "profile"}, "clientType": "public", "tokenLifetime": 3600, "signingAlgorithm": "HS256"}}, user.ID)
	require.NoError(t, err)
	clientID := app.ProtocolConfig["clientId"].(string)
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.POST("/api/user/login", UserController{}.ValidateLogin)
	router.POST("/api/oidc/login", OIDCAuthFlowController{}.Login)
	router.GET("/api/oidc/flow-status", OIDCAuthFlowController{}.GetFlowStatus)
	router.GET("/api/oidc/auth-info", OIDCAuthFlowController{}.GetAuthInfo)
	router.POST("/api/oidc/consent", OIDCAuthFlowController{}.Consent)
	router.GET("/api/2fa/challenges/:id", TwoFactorController{}.Get)
	router.POST("/api/2fa/challenges/:id/verify", TwoFactorController{}.Verify)
	router.POST("/api/2fa/challenges/:id/cancel", TwoFactorController{}.Cancel)
	protected := router.Group("/api/user")
	protected.Use(middleware.UserAuthMiddleware())
	protected.GET("/profile", UserController{}.GetUserProfile)
	protected.POST("/2fa/reauthenticate", TwoFactorController{}.Reauthenticate)
	protected.POST("/2fa/enroll", TwoFactorController{}.Enroll)
	protected.POST("/2fa/enable", TwoFactorController{}.Mutate)
	// Exercise the shared external/miniprogram completion gate after upstream identity verification.
	router.POST("/external-auth/test-completion", func(c httpInterfaces.RouterContextInterface) {
		profile, err := userService.GetUserByID(user.ID)
		require.NoError(t, err)
		ExternalAuthController{}.completeLoginResponse(c, &dto.SocialAuthCallbackResponse{User: profile}, "", "登录成功")
	})
	origin, _ := url.Parse("http://iam.test")
	newJar := func() http.CookieJar { jar, err := cookiejar.New(nil); require.NoError(t, err); return jar }
	request := func(jar http.CookieJar, method, path string, payload any) (*httptest.ResponseRecorder, map[string]any) {
		t.Helper()
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(method, origin.String()+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		for _, cookie := range jar.Cookies(origin) {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		jar.SetCookies(origin, rec.Result().Cookies())
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
		result, _ := body["data"].(map[string]any)
		return rec, result
	}
	setup := newJar()
	rec, _ := request(setup, "POST", "/api/user/login", map[string]any{"username": user.Username, "password": "correct-password"})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	rec, auth := request(setup, "POST", "/api/user/2fa/reauthenticate", map[string]any{"password": "correct-password"})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	rec, enrollment := request(setup, "POST", "/api/user/2fa/enroll", map[string]any{"method": "totp", "operationToken": auth["operationToken"]})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	id := enrollment["twoFactorChallenge"].(map[string]any)["id"].(string)
	code, err := totp.GenerateCode(enrollment["secret"].(string), time.Now())
	require.NoError(t, err)
	rec, result := request(setup, "POST", "/api/2fa/challenges/"+id+"/verify", map[string]any{"method": "totp", "code": code})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, true, result["twoFactorUpdated"])
	require.Empty(t, result["recoveryCodes"])
	rec, auth = request(setup, "POST", "/api/user/2fa/reauthenticate", map[string]any{"password": "correct-password"})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	rec, result = request(setup, "POST", "/api/user/2fa/enable", map[string]any{"operationToken": auth["operationToken"]})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	recovery := result["recoveryCodes"].([]any)
	require.Len(t, recovery, 10)
	rec, _ = request(setup, "GET", "/api/user/profile", nil)
	require.Equal(t, 200, rec.Code, "current browser receives a new valid session")
	for _, entry := range []string{"account", "external", "oidc"} {
		t.Run(entry, func(t *testing.T) {
			browser := newJar()
			path := "/api/user/login"
			body := map[string]any{"username": user.Username, "password": "correct-password"}
			var oidc *service.OIDCSession
			sessions := service.NewOIDCSessionService(redisClient)
			if entry == "external" {
				path = "/external-auth/test-completion"
				body = map[string]any{}
			}
			if entry == "oidc" {
				oidc, err = sessions.CreateSession(context.Background(), clientID, "https://rp.example/callback", "openid profile", "state", "nonce", "code", strings.Repeat("A", 43), "S256", "")
				require.NoError(t, err)
				browser.SetCookies(origin, []*http.Cookie{{Name: oidcBrowserCookieName(oidc.SessionID), Value: oidc.BrowserBindingToken, Path: "/"}})
				path = "/api/oidc/login"
				body["sessionId"] = oidc.SessionID
			}
			var before int64
			require.NoError(t, db.Model(&model.LoginSession{}).Count(&before).Error)
			rec, pending := request(browser, "POST", path, body)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Equal(t, true, pending["requiresTwoFactor"])
			require.Empty(t, pending["token"])
			require.Empty(t, pending["accessToken"])
			for _, cookie := range rec.Result().Cookies() {
				require.NotEqual(t, middleware.UserCookieName, cookie.Name)
				require.NotEqual(t, middleware.AdminCookieName, cookie.Name)
				require.NotEqual(t, "refresh_token", cookie.Name)
			}
			var after int64
			require.NoError(t, db.Model(&model.LoginSession{}).Count(&after).Error)
			require.Equal(t, before, after, "first factor must not create a login session")
			id := pending["twoFactorChallenge"].(map[string]any)["id"].(string)
			rec, _ = request(newJar(), "GET", "/api/2fa/challenges/"+id, nil)
			require.NotEqual(t, 200, rec.Code, "challenge is browser bound")
			rec, _ = request(browser, "GET", "/api/user/profile", nil)
			require.Equal(t, 401, rec.Code)
			if oidc != nil {
				current, err := sessions.GetSession(context.Background(), oidc.SessionID)
				require.NoError(t, err)
				require.False(t, current.IsAuthenticated)
				require.False(t, current.HasLoginInteraction)
				require.Nil(t, current.UserID)
				rec, flow := request(browser, "GET", "/api/oidc/flow-status?session_id="+oidc.SessionID, nil)
				require.Equal(t, 200, rec.Code, rec.Body.String())
				require.Equal(t, "needs_two_factor", flow["status"])
				require.NotEqual(t, true, flow["autoApprove"])
				rec, _ = request(browser, "GET", "/api/oidc/auth-info?session_id="+oidc.SessionID, nil)
				require.NotEqual(t, 200, rec.Code)
				rec, _ = request(browser, "POST", "/api/oidc/consent", map[string]any{"sessionId": oidc.SessionID, "approved": true, "csrfToken": oidc.CsrfToken})
				require.NotEqual(t, 200, rec.Code)
			}
			i := map[string]int{"account": 0, "external": 1, "oidc": 2}[entry]
			rec, done := request(browser, "POST", "/api/2fa/challenges/"+id+"/verify", map[string]any{"method": "recovery", "code": recovery[i]})
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.NotEqual(t, true, done["requiresTwoFactor"])
			rec, _ = request(browser, "GET", "/api/user/profile", nil)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			rec, _ = request(browser, "POST", "/api/2fa/challenges/"+id+"/verify", map[string]any{"method": "recovery", "code": recovery[i]})
			require.NotEqual(t, 200, rec.Code)
			if oidc != nil {
				current, err := sessions.GetSession(context.Background(), oidc.SessionID)
				require.NoError(t, err)
				require.True(t, current.IsAuthenticated)
				require.True(t, current.HasLoginInteraction)
				require.NotEmpty(t, current.LoginSessionID)
				rec, flow := request(browser, "GET", "/api/oidc/flow-status?session_id="+oidc.SessionID, nil)
				require.Equal(t, 200, rec.Code, rec.Body.String())
				require.Equal(t, true, flow["autoApprove"])
			}
		})
	}
	if os.Getenv("IAM_TWO_FACTOR_BROWSER_TEST") == "1" {
		// Opt-in local browser fixture: isolated SQLite/Redis and synthetic accounts.
		router.GET("/api/oidc/client-config", OIDCAuthFlowController{}.GetClientConfig)
		router.POST("/api/captcha/params", CaptchaController{}.GetCaptchaParams)
		protected.GET("/2fa", TwoFactorController{}.Status)
		protected.POST("/2fa/recovery-codes/regenerate", TwoFactorController{}.Mutate)
		protected.POST("/2fa/methods/:method/remove", TwoFactorController{}.Mutate)
		protected.POST("/2fa/disable", TwoFactorController{}.Mutate)
		protected.GET("/sessions", LoginSessionController{}.ListMine)
		protected.GET("/external-identities", func(c httpInterfaces.RouterContextInterface) { BaseResponse{}.Success(c, []any{}) })
		router.POST("/api/auth/logout", UserController{}.Logout)
		done := make(chan struct{}, 1)
		engine.GET("/__test/recovery", func(c *gin.Context) { c.JSON(200, recovery[3:]) })
		engine.POST("/__test/stop", func(c *gin.Context) {
			select {
			case done <- struct{}{}:
			default:
			}
			c.Status(204)
		})
		engine.Static("/auth", "../../static/auth")
		httpServer := httptest.NewServer(engine)
		defer httpServer.Close()
		fmt.Println("TWO_FACTOR_BROWSER_URL=" + httpServer.URL)
		select {
		case <-done:
		case <-time.After(5 * time.Minute):
			t.Fatal("browser fixture timed out")
		}
		return
	}
	var codesLeft int64
	require.NoError(t, db.Model(&model.TwoFactorRecoveryCode{}).Where("used_at IS NULL").Count(&codesLeft).Error)
	require.Equal(t, int64(7), codesLeft, fmt.Sprint(codesLeft))
}
