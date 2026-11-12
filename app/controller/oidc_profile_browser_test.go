package controller

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Explicit opt-in, loopback-only fixture. Codes are placed in isolated Redis;
// no real contacts, production database or message provider are used.
func TestOIDCProfileCompletionBrowser(t *testing.T) {
	if os.Getenv("IAM_PROFILE_BROWSER_TEST") != "1" {
		t.Skip("set IAM_PROFILE_BROWSER_TEST=1 to start the isolated browser fixture")
	}
	f := newAccountAuthFixture(t)
	router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
	router.POST("/api/oidc/register", OIDCAuthFlowController{}.Register)
	router.POST("/api/oidc/complete-profile", OIDCAuthFlowController{}.CompleteProfile)
	router.POST("/webhook/oidc/consent", OIDCAuthFlowController{}.Consent)
	router.GET("/api/appearance", AppearanceController{}.Get)
	router.GET("/api/passkey/config", PasskeyController{}.Config)
	router.POST("/api/oidc/auth/refresh", OIDCAuthFlowController{}.RefreshToken)
	router.GET("/api/oidc/register-settings", OIDCAuthFlowController{}.GetRegisterSettings)
	router.POST("/api/captcha/params", CaptchaController{}.GetCaptchaParams)
	router.GET("/api/identity-connections", func(c httpInterfaces.RouterContextInterface) { BaseResponse{}.Success(c, []any{}) })
	router.POST("/oidc/:client_id/token", OIDCController{}.Token)
	require.NoError(t, f.db.Create(&model.Setting{Key: service.PushSettingsConfigKey, Value: `{"baseUrl":"https://push.invalid","appId":"test","appSecret":"test"}`}).Error)
	require.NoError(t, f.db.Create(&model.Setting{Key: service.RegisterFieldsConfigKey, Value: `{"username":{"visible":true,"required":true,"label":"用户名"},"email":{"visible":false,"required":false},"phone":{"visible":false,"required":false},"realName":{"visible":false,"required":false},"nickname":{"visible":false,"required":false}}`}).Error)
	app := newConsentTestApplication(t, f)
	authDir := os.Getenv("IAM_AUTH_OUT_DIR")
	if authDir == "" {
		authDir = "../../static/auth"
	}
	authDir, err := filepath.Abs(authDir)
	require.NoError(t, err)
	f.engine.Static("/auth", authDir)
	var origin string
	router.GET("/__test/start", func(c httpInterfaces.RouterContextInterface) {
		require.NoError(t, f.db.Exec("DROP TRIGGER IF EXISTS fail_profile_token").Error)
		fields := c.Query("fields")
		if fields == "" {
			fields = "email"
		}
		prompt := c.Query("prompt")
		username := "smoke_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		if prompt != "create" {
			_, err := service.NewUserService(f.db).CreateUser(&dto.CreateUserRequest{Username: username, Password: "Correct-password123", RealName: "Smoke User"})
			require.NoError(t, err)
		}
		sum := sha256.Sum256([]byte(profileVerifier))
		tr, err := service.NewOIDCSessionService(f.redis).CreateSession(context.Background(), app.UUID, origin+"/__test/callback", "openid "+fields, "smoke-state", "nonce", "code", base64.RawURLEncoding.EncodeToString(sum[:]), "S256", prompt)
		require.NoError(t, err)
		setOIDCBrowserBindingCookie(c, tr.SessionID, tr.BrowserBindingToken)
		BaseResponse{}.Success(c, map[string]any{"url": origin + "/auth/#/oidc/authorize?session_id=" + tr.SessionID, "sessionId": tr.SessionID, "username": username, "password": "Correct-password123"})
	})
	router.POST("/__test/code", func(c httpInterfaces.RouterContextInterface) {
		var req struct {
			SessionID string `json:"sessionId"`
			Type      string `json:"type"`
			Receiver  string `json:"receiver"`
		}
		require.NoError(t, c.ShouldBindJSON(&req))
		require.Contains(t, []string{"email", "phone"}, req.Type)
		require.NoError(t, f.redis.Set(context.Background(), "verification:code:"+req.Type+":"+req.Receiver+":"+req.SessionID, "123456", 5*time.Minute).Err())
		BaseResponse{}.Success(c, map[string]any{"success": true})
	})
	router.POST("/__test/fail-issuance", func(c httpInterfaces.RouterContextInterface) {
		require.NoError(t, f.db.Exec("CREATE TRIGGER fail_profile_token BEFORE INSERT ON oidc_tokens BEGIN SELECT RAISE(ABORT, 'browser injected issuance failure'); END").Error)
		c.Status(204)
	})
	router.GET("/__test/callback", func(c httpInterfaces.RouterContextInterface) {
		if c.Query("error") != "" {
			c.String(200, "请从应用重新登录：%s", c.Query("error"))
			return
		}
		require.Equal(t, "smoke-state", c.Query("state"))
		result, err := service.NewOIDCAuthorizationService(f.db).ExchangeCodeForToken(&dto.TokenRequest{GrantType: "authorization_code", ClientID: app.UUID, Code: c.Query("code"), RedirectURI: origin + "/__test/callback", CodeVerifier: profileVerifier}, origin+"/oidc/"+app.UUID)
		require.NoError(t, err)
		require.NotEmpty(t, result.AccessToken)
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<!doctype html><html lang=zh-CN><meta name=viewport content='width=device-width,initial-scale=1'><title>授权完成</title><h1>授权完成</h1><p>已返回原应用，授权码交换成功。</p></html>"))
	})
	done := make(chan struct{}, 1)
	f.engine.POST("/__test/stop", func(c *gin.Context) {
		select {
		case done <- struct{}{}:
		default:
		}
		c.Status(204)
	})
	server := httptest.NewServer(f.engine)
	defer server.Close()
	origin = server.URL
	var appModel model.Application
	require.NoError(t, f.db.First(&appModel, app.ID).Error)
	appModel.ProtocolConfig["redirectUris"] = []string{origin + "/__test/callback"}
	require.NoError(t, f.db.Save(&appModel).Error)
	fmt.Println("PROFILE_BROWSER_URL=" + origin)
	select {
	case <-done:
	case <-time.After(15 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
