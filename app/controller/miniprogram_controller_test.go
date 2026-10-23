package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/container"
	envimpl "cnb.cool/mliev/open/go-web/pkg/server/env/impl"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

type nativeHTTPTransport func(*http.Request) (*http.Response, error)

func (f nativeHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMiniProgramHTTPUsesBearerTransactionsAndNeverCookies(t *testing.T) {
	db := registerControllerTestDatabase(t)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.CaptchaConfig{}, &model.Setting{}, &model.IdentityConnection{}, &model.IdentityConnectionCredential{}, &model.ExternalPrincipal{}, &model.ExternalIdentifier{}, &model.WeChatIdentity{}, &model.WeChatIdentityConflict{}, &model.Agreement{}, &model.AgreementAcceptance{}, &model.CustomPermission{}))
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { rdb.Close() })
	priority := int(atomic.AddInt64(&controllerTestDBProviderPriority, 1))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[*redis.Client](), rdb, priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Enver](), envimpl.NewEnv(), priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), priority))
	connection := model.IdentityConnection{Name: "WeChat", Identifier: "native-http", Category: model.ConnectionCategorySocial, ProviderType: "wechat_miniprogram", Capabilities: model.StringList{"authentication"}, Status: model.ConnectionStatusEnabled, Config: model.JSONObject{"clientId": "wx-test", "clientSecret": "do-not-return"}}
	require.NoError(t, db.Create(&connection).Error)
	app := model.Application{UUID: "native-http", Name: "Native HTTP", ProtocolType: "oidc", IsActive: true, ProtocolConfig: map[string]any{"clientId": "native-http", "clientType": "public", "scopes": []string{"openid"}, "miniProgram": model.MiniProgramConfig{Enabled: true, ConnectionID: connection.ID, AccountPolicy: "auto_register"}}}
	require.NoError(t, db.Create(&app).Error)
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = nativeHTTPTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "api.weixin.qq.com", r.URL.Host)
		require.Equal(t, "/sns/jscode2session", r.URL.Path)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"openid":"http-open","unionid":"http-union","session_key":"never-return-session-key"}`))}, nil
	})
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.GET("/api/auth/miniprogram/:client_id/config", MiniProgramController{}.Handle)
	for _, action := range []string{"login", "phone", "complete", "bind", "captcha/params"} {
		router.POST("/api/auth/miniprogram/:client_id/"+action, MiniProgramController{}.Handle)
	}
	request := func(method, action, body, authorization string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/auth/miniprogram/native-http/"+action, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authorization)
		req.AddCookie(&http.Cookie{Name: "user_access_token", Value: "not-a-native-transaction"})
		engine.ServeHTTP(rec, req)
		require.Empty(t, rec.Result().Cookies())
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		return rec
	}
	config := request("GET", "config", "", "")
	require.Equal(t, 200, config.Code)
	require.NotContains(t, config.Body.String(), "do-not-return")
	bad := request("POST", "login", `{"openid":"forged"}`, "")
	require.Equal(t, 400, bad.Code)
	login := request("POST", "login", `{"jsCode":"real-code"}`, "")
	require.Equal(t, 200, login.Code)
	var result struct {
		Data struct {
			State string `json:"state"`
			Token string `json:"transactionToken"`
		}
	}
	require.NoError(t, json.Unmarshal(login.Body.Bytes(), &result))
	require.Equal(t, "phone_required", result.Data.State)
	require.Len(t, result.Data.Token, 43)
	require.NotContains(t, login.Body.String(), "session_key")
	require.NotContains(t, login.Body.String(), "http-open")
	noBearer := request("POST", "phone", `{"phoneCode":"code"}`, "")
	require.Equal(t, 401, noBearer.Code)
	wrongStage := request("POST", "complete", `{"approveScopes":true}`, "Bearer "+result.Data.Token)
	require.Equal(t, 401, wrongStage.Code)
	capConfig := model.CaptchaConfig{Name: "HTTP mini captcha", Provider: "tencent_miniprogram", Enabled: true, Params: datatypes.JSONMap{"captchaAppId": "123456", "secretId": "hidden-cloud-id", "secretKey": "hidden-cloud-key", "appSecretKey": "hidden-app-secret"}}
	require.NoError(t, db.Create(&capConfig).Error)
	app.ProtocolConfig["miniProgram"] = model.MiniProgramConfig{Enabled: true, ConnectionID: connection.ID, AccountPolicy: "auto_register", CaptchaConfigID: &capConfig.ID}
	require.NoError(t, db.Save(&app).Error)
	missingProof := request("POST", "captcha/params", `{"fingerprint":"installation"}`, "")
	require.Equal(t, 401, missingProof.Code)
	params := request("POST", "captcha/params", `{"fingerprint":"installation"}`, "Bearer "+result.Data.Token)
	require.Equal(t, 200, params.Code)
	require.Contains(t, params.Body.String(), `"provider":"tencent_miniprogram"`)
	require.Contains(t, params.Body.String(), `"sessionUuid"`)
	require.NotContains(t, params.Body.String(), "hidden-")
	bindWithoutCaptcha := request("POST", "bind", `{"username":"somebody","password":"password"}`, "Bearer "+result.Data.Token)
	require.Equal(t, 400, bindWithoutCaptcha.Code)
	require.Contains(t, bindWithoutCaptcha.Body.String(), "请完成小程序验证码")
	replay := request("POST", "login", `{"jsCode":"real-code"}`, "")
	require.Equal(t, 400, replay.Code)
}
