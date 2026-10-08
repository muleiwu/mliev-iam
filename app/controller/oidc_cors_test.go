package controller

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
	configimpl "cnb.cool/mliev/open/go-web/pkg/server/config/impl"
	envimpl "cnb.cool/mliev/open/go-web/pkg/server/env/impl"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type corsHTTPFixture struct {
	engine  *gin.Engine
	db      *gorm.DB
	user    *model.User
	session *model.LoginSession
	apps    []*model.Application
	secrets []string
}

func newCORSHTTPFixture(t *testing.T, origins []string, public bool) *corsHTTPFixture {
	t.Helper()
	db := registerControllerTestDatabase(t)
	db.Logger = logger.Default.LogMode(logger.Silent)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.LoginSession{}, &model.ApplicationOIDCKey{}, &model.OIDCToken{}, &model.AuthorizationCode{}))
	priority := int(atomic.AddInt64(&controllerTestDBProviderPriority, 1))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Enver](), &envimpl.Env{}, priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Provider](), configimpl.NewConfig(), priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), priority))
	user := &model.User{Username: "cors-http-user", Status: model.UserStatusActive, IsActivated: true}
	require.NoError(t, db.Create(user).Error)
	session, err := service.NewLoginSessionService(db).Create(user.ID, "password", "127.0.0.1", "CORS test", time.Now().Add(time.Hour))
	require.NoError(t, err)
	fixture := &corsHTTPFixture{db: db, user: user, session: session}
	for _, origin := range origins {
		clientType, alg := "confidential", "HS256"
		if public {
			clientType, alg = "public", "RS256"
		}
		result, err := service.NewApplicationService(db).CreateApplication(&dto.CreateApplicationRequest{
			Name: "CORS " + origin, ProtocolType: "oidc", ProtocolConfig: map[string]interface{}{
				"redirectUris": []interface{}{origin + "/callback"}, "grantTypes": []interface{}{"authorization_code", "refresh_token", "client_credentials"},
				"responseTypes": []interface{}{"code"}, "scopes": []interface{}{"openid", "profile"}, "clientType": clientType, "tokenLifetime": 300, "signingAlgorithm": alg,
			},
		}, user.ID)
		require.NoError(t, err)
		app, err := service.NewApplicationService(db).GetApplicationByClientID(result.UUID)
		require.NoError(t, err)
		fixture.apps = append(fixture.apps, app)
		secret, _ := result.ProtocolConfig["clientSecret"].(string)
		fixture.secrets = append(fixture.secrets, secret)
	}
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.Use(middleware.CorsMiddleware())
	router.GET("/oidc/:client_id/.well-known/openid-configuration", OIDCController{}.Discovery)
	router.GET("/oidc/:client_id/jwks", OIDCController{}.JWKS)
	router.POST("/oidc/:client_id/token", OIDCController{}.Token)
	router.GET("/oidc/:client_id/userinfo", OIDCController{}.UserInfo)
	router.POST("/oidc/:client_id/userinfo", OIDCController{}.UserInfo)
	router.POST("/oidc/:client_id/introspect", OIDCController{}.Introspect)
	router.POST("/oidc/:client_id/revoke", OIDCController{}.Revoke)
	fixture.engine = engine
	return fixture
}

func (f *corsHTTPFixture) request(method, path, origin string, form url.Values, bearer, basicID, basicSecret string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if basicID != "" {
		request.SetBasicAuth(basicID, basicSecret)
	}
	response := httptest.NewRecorder()
	f.engine.ServeHTTP(response, request)
	return response
}

func TestOIDCCORSHTTPClientBindingAndTokenOwnership(t *testing.T) {
	f := newCORSHTTPFixture(t, []string{"https://a.example", "https://b.example"}, false)
	a, b := f.apps[0], f.apps[1]
	for _, basic := range []bool{false, true} {
		for _, endpoint := range []string{"token", "introspect", "revoke"} {
			form := url.Values{"grant_type": {"client_credentials"}, "scope": {"profile"}, "token": {"example-token"}}
			id, secret := "", ""
			if basic {
				id, secret = b.UUID, f.secrets[1]
			} else {
				form.Set("client_id", b.UUID)
				form.Set("client_secret", f.secrets[1])
			}
			response := f.request("POST", "/oidc/"+a.UUID+"/"+endpoint, "https://a.example", form, "", id, secret)
			require.Equal(t, 401, response.Code, endpoint)
			require.Contains(t, response.Body.String(), "invalid_client")
			require.Equal(t, "https://a.example", response.Header().Get("Access-Control-Allow-Origin"))
			require.Empty(t, response.Header().Get("Access-Control-Allow-Credentials"))
		}
	}
	code := &model.AuthorizationCode{Code: "belongs-to-b", ClientID: b.UUID, UserID: f.user.ID, LoginSessionID: f.session.ID, RedirectURI: "https://b.example/callback", Scopes: []string{"openid"}, ExpiresAt: time.Now().Add(time.Minute)}
	require.NoError(t, f.db.Create(code).Error)
	response := f.request("POST", "/oidc/"+a.UUID+"/token", "https://a.example", url.Values{"grant_type": {"authorization_code"}, "code": {code.Code}, "redirect_uri": {code.RedirectURI}, "client_id": {b.UUID}, "client_secret": {f.secrets[1]}}, "", "", "")
	require.Equal(t, 401, response.Code)
	var stored model.AuthorizationCode
	require.NoError(t, f.db.First(&stored, code.ID).Error)
	require.False(t, stored.IsUsed, "cross-client requests must not consume authorization codes")
	tokens, err := service.NewOIDCAuthorizationService(f.db).GenerateTokens(b, f.user.ID, []string{"openid", "profile"}, "", "https://iam.example/oidc/"+b.UUID, f.session.ID)
	require.NoError(t, err)
	for _, method := range []string{"GET", "POST"} {
		response = f.request(method, "/oidc/"+a.UUID+"/userinfo", "https://a.example", nil, tokens.AccessToken, "", "")
		require.Equal(t, 401, response.Code)
		require.Contains(t, response.Body.String(), "invalid_token")
		response = f.request(method, "/oidc/"+b.UUID+"/userinfo", "https://b.example", nil, tokens.AccessToken, "", "")
		require.Equal(t, 200, response.Code)
		require.Contains(t, response.Body.String(), f.user.UUID)
	}
	// Cross-client revocation cannot affect B; matching Basic auth is accepted.
	response = f.request("POST", "/oidc/"+a.UUID+"/revoke", "https://a.example", url.Values{"token": {tokens.AccessToken}}, "", b.UUID, f.secrets[1])
	require.Equal(t, 401, response.Code)
	_, err = dao.NewOIDCTokenDAO(f.db).GetValidByAccessToken(tokens.AccessToken)
	require.NoError(t, err)
	response = f.request("POST", "/oidc/"+b.UUID+"/introspect", "https://b.example", url.Values{"token": {tokens.AccessToken}}, "", b.UUID, f.secrets[1])
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"active":true`)
	response = f.request("POST", "/oidc/"+b.UUID+"/revoke", "https://b.example", url.Values{"token": {tokens.AccessToken}}, "", b.UUID, f.secrets[1])
	require.Equal(t, 200, response.Code)
	_, err = dao.NewOIDCTokenDAO(f.db).GetValidByAccessToken(tokens.AccessToken)
	require.Error(t, err)
}

func TestOIDCCORSHTTPMetadataAndServerCalls(t *testing.T) {
	f := newCORSHTTPFixture(t, []string{"https://pagnode.mliev.com"}, false)
	app := f.apps[0]
	for _, endpoint := range []string{".well-known/openid-configuration", "jwks"} {
		response := f.request("GET", "/oidc/"+app.UUID+"/"+endpoint, "https://any.example", nil, "", "", "")
		require.Equal(t, 200, response.Code)
		require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, response.Header().Get("Access-Control-Allow-Credentials"))
	}
	response := f.request("GET", "/oidc/missing/.well-known/openid-configuration", "https://any.example", nil, "", "", "")
	require.Equal(t, 404, response.Code)
	require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
	response = f.request("POST", "/oidc/"+app.UUID+"/token", "", url.Values{"grant_type": {"client_credentials"}, "scope": {"profile"}}, "", app.UUID, f.secrets[0])
	require.Equal(t, 200, response.Code)
	require.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
	var token dto.TokenResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &token))
	require.NotEmpty(t, token.AccessToken)
}

func TestOIDCCORSJWKSExcludesExpiredKeys(t *testing.T) {
	f := newCORSHTTPFixture(t, []string{"https://rp.example"}, true)
	keyService := service.NewApplicationKeyService(f.db)
	old, err := keyService.GetPrimaryKeyForApplication(f.apps[0].ID)
	require.NoError(t, err)
	latest, err := keyService.GenerateKeyForApplication(f.apps[0].ID, "RS256", 2048)
	require.NoError(t, err)
	valid, err := keyService.ListValidKeysByApplication(f.apps[0].ID)
	require.NoError(t, err)
	require.Len(t, valid, 2)
	require.NoError(t, f.db.Model(old).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	response := f.request("GET", "/oidc/"+f.apps[0].UUID+"/jwks", "https://any.example", nil, "", "", "")
	require.Equal(t, 200, response.Code)
	require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
	var keys struct {
		Keys []map[string]interface{} `json:"keys"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &keys))
	require.Len(t, keys.Keys, 1)
	require.Equal(t, latest.KeyID, keys.Keys[0]["kid"])
}
