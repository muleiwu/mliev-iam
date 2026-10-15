package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
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
)

// Explicit opt-in, isolated local data only. The fixture never connects to application services.
func TestPasskeyBrowser(t *testing.T) {
	if os.Getenv("IAM_PASSKEY_BROWSER_TEST") != "1" {
		t.Skip("set IAM_PASSKEY_BROWSER_TEST=1 for the local browser fixture")
	}
	t.Setenv("JWT_SECRET_KEY", strings.Repeat("passkey-browser-test-", 4))
	t.Setenv(credentialcrypto.EnvironmentKeys, "")
	t.Setenv(credentialcrypto.EnvironmentKey, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", 32))))
	db := registerControllerTestDatabase(t)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.Setting{}, &model.User{}, &model.LoginSession{}, &model.Role{}, &model.UserRole{}, &model.CaptchaConfig{}, &model.OIDCToken{}, &model.AuthorizationCode{}, &model.TwoFactorMethod{}, &model.TwoFactorRecoveryCode{}, &model.TwoFactorEvent{}, &model.ApplicationOIDCKey{}, &model.ExternalPrincipal{}, &model.IdentityConnection{}, &model.Passkey{}))
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	priority := int(atomic.AddInt64(&controllerTestDBProviderPriority, 1))
	cfg := configimpl.NewConfig()
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Provider](), cfg, priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Enver](), envimpl.NewEnv(), priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[*redis.Client](), client, priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), priority))
	us := service.NewUserService(db)
	salt, hash, err := us.GeneratePasswordHash("correct-password")
	require.NoError(t, err)
	u := model.User{Username: "passkey-user", Status: model.UserStatusActive, IsActivated: true, Salt: salt, PasswordHash: hash}
	require.NoError(t, db.Create(&u).Error)
	application, err := service.NewApplicationService(db).CreateApplication(&dto.CreateApplicationRequest{Name: "Passkey test application", ProtocolType: "oidc", ProtocolConfig: map[string]interface{}{"redirectUris": []interface{}{"https://rp.example/callback"}, "grantTypes": []interface{}{"authorization_code", "refresh_token"}, "responseTypes": []interface{}{"code"}, "scopes": []interface{}{"openid", "profile"}, "clientType": "public", "signingAlgorithm": "HS256"}}, u.ID)
	require.NoError(t, err)
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	pc := PasskeyController{}
	tc := TwoFactorController{}
	router.GET("/api/passkey/config", pc.Config)
	router.POST("/api/passkey/login/options", pc.LoginOptions)
	router.POST("/api/passkey/login/verify", pc.LoginVerify)
	router.POST("/api/user/login", UserController{}.ValidateLogin)
	router.GET("/api/user/auth/authorize", AccountAuthController{}.Authorize)
	router.GET("/api/user/auth/callback", AccountAuthController{}.Callback)
	router.GET("/oidc/:client_id/authorize", OIDCController{}.Authorize)
	router.GET("/api/oidc/session-bind", OIDCAuthFlowController{}.BindSession)
	router.POST("/api/oidc/consent", OIDCAuthFlowController{}.Consent)
	router.POST("/webhook/oidc/consent", OIDCAuthFlowController{}.Consent)
	router.GET("/api/identity-connections", func(c httpInterfaces.RouterContextInterface) { BaseResponse{}.Success(c, []any{}) })
	router.POST("/api/auth/logout", UserController{}.Logout)
	router.POST("/api/oidc/auth/logout", OIDCAuthFlowController{}.Logout)
	router.POST("/api/captcha/params", CaptchaController{}.GetCaptchaParams)
	router.GET("/api/2fa/challenges/:id", tc.Get)
	router.POST("/api/2fa/challenges/:id/verify", tc.Verify)
	router.POST("/api/2fa/challenges/:id/cancel", tc.Cancel)
	router.POST("/api/2fa/challenges/:id/passkey/options", pc.SecondOptions)
	router.POST("/api/2fa/challenges/:id/passkey/verify", pc.SecondVerify)
	router.POST("/api/oidc/login", OIDCAuthFlowController{}.Login)
	router.GET("/api/oidc/flow-status", OIDCAuthFlowController{}.GetFlowStatus)
	router.GET("/api/oidc/client-config", OIDCAuthFlowController{}.GetClientConfig)
	router.GET("/api/oidc/auth-info", OIDCAuthFlowController{}.GetAuthInfo)
	protected := router.Group("/api/user")
	protected.Use(middleware.UserAuthMiddleware())
	protected.GET("/profile", UserController{}.GetUserProfile)
	protected.GET("/2fa", tc.Status)
	protected.POST("/2fa/reauthenticate", tc.Reauthenticate)
	protected.POST("/2fa/enable", tc.Mutate)
	protected.POST("/2fa/disable", tc.Mutate)
	protected.POST("/2fa/enroll", tc.Enroll)
	protected.POST("/2fa/methods/:method/remove", tc.Mutate)
	protected.POST("/2fa/recovery-codes/regenerate", tc.Mutate)
	protected.GET("/passkeys", pc.List)
	protected.POST("/passkeys/registration/options", pc.RegistrationOptions)
	protected.POST("/passkeys/registration/verify", pc.RegistrationVerify)
	protected.PATCH("/passkeys/:id", pc.Rename)
	protected.DELETE("/passkeys/:id", pc.Delete)
	protected.GET("/sessions", LoginSessionController{}.ListMine)
	protected.GET("/external-identities", func(c httpInterfaces.RouterContextInterface) { BaseResponse{}.Success(c, []any{}) })
	router.GET("/__test/oidc", func(c httpInterfaces.RouterContextInterface) {
		tr, err := service.NewOIDCSessionService(client).CreateSession(context.Background(), application.UUID, "https://rp.example/callback", "openid profile", "state", "nonce", "code", strings.Repeat("A", 43), "S256", "")
		require.NoError(t, err)
		c.SetCookie(oidcBrowserCookieName(tr.SessionID), tr.BrowserBindingToken, 300, "/", "", false, true)
		BaseResponse{}.Success(c, map[string]any{"sessionId": tr.SessionID})
	})
	done := make(chan struct{}, 1)
	engine.POST("/__test/stop", func(c *gin.Context) {
		select {
		case done <- struct{}{}:
		default:
		}
		c.Status(204)
	})
	engine.GET("/__test/iframe", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(200, `<iframe src="%s/auth/#/account/login" style="width:100%%;height:800px"></iframe>`, cfg.GetStringSlice("passkey.rp_origins", nil)[0])
	})
	authDir := os.Getenv("IAM_AUTH_OUT_DIR")
	if authDir == "" {
		authDir = "../../static/auth"
	}
	engine.Static("/auth/assets", filepath.Join(authDir, "assets"))
	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/auth/") {
			c.File(filepath.Join(authDir, "index.html"))
		} else {
			c.Status(404)
		}
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	origin := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	cfg.Set("passkey.enabled", true)
	cfg.Set("passkey.rp_id", "localhost")
	cfg.Set("passkey.rp_origins", []string{origin})
	cfg.Set("passkey.rp_display_name", "Passkey test IAM")
	fmt.Println("PASSKEY_BROWSER_URL=" + origin)
	select {
	case <-done:
	case <-time.After(5 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
