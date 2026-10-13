package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/container"
	configimpl "cnb.cool/mliev/open/go-web/pkg/server/config/impl"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var corsTestPriority atomic.Int64

func corsFixture(t *testing.T) (*gin.Engine, *gorm.DB, *model.Application, *int) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.Application{}))
	app := &model.Application{UUID: "cors-client", Name: "CORS client", ProtocolType: "oidc", IsActive: true, ProtocolConfig: map[string]interface{}{"redirectUris": []string{"https://pagnode.mliev.com/callback"}}}
	require.NoError(t, db.Create(app).Error)
	priority := int(corsTestPriority.Add(1)) + 10000
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[*gorm.DB](), db, priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Provider](), configimpl.NewConfig(), priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), priority))
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.Use(func(c httpInterfaces.RouterContextInterface) { c.SetHeader("Vary", "Accept-Encoding"); c.Next() }, CorsMiddleware())
	called := 0
	handler := func(c httpInterfaces.RouterContextInterface) {
		called++
		c.JSON(200, map[string]string{"result": "ok"})
	}
	router.GET("/oidc/:client_id/.well-known/openid-configuration", handler)
	router.GET("/oidc/:client_id/jwks", handler)
	router.POST("/oidc/:client_id/token", func(c httpInterfaces.RouterContextInterface) {
		called++
		c.JSON(401, map[string]string{"error": "invalid_client"})
	})
	router.GET("/oidc/:client_id/userinfo", handler)
	router.POST("/oidc/:client_id/userinfo", handler)
	router.POST("/oidc/:client_id/revoke", handler)
	router.POST("/oidc/:client_id/introspect", handler)
	router.GET("/oidc/:client_id/authorize", handler)
	router.GET("/api/admin/profile", handler)
	router.GET("/api/oidc/client-config", handler)
	router.GET("/install", handler)
	return engine, db, app, &called
}

func corsRequest(engine http.Handler, method, path, origin, requestedMethod, headers string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if requestedMethod != "" {
		request.Header.Set("Access-Control-Request-Method", requestedMethod)
	}
	if headers != "" {
		request.Header.Set("Access-Control-Request-Headers", headers)
	}
	result := httptest.NewRecorder()
	engine.ServeHTTP(result, request)
	return result
}

func TestOIDCCORSPublicMetadata(t *testing.T) {
	engine, _, _, called := corsFixture(t)
	for _, path := range []string{"/oidc/cors-client/.well-known/openid-configuration", "/oidc/cors-client/jwks"} {
		response := corsRequest(engine, "GET", path, "https://pagnode.mliev.com", "", "")
		require.Equal(t, 200, response.Code)
		require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, response.Header().Get("Access-Control-Allow-Credentials"))
		before := *called
		response = corsRequest(engine, "OPTIONS", path, "https://another.example", "GET", "content-type, x-trace-id")
		require.Equal(t, 204, response.Code)
		require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "300", response.Header().Get("Access-Control-Max-Age"))
		require.Equal(t, before, *called, "preflight must not execute the endpoint")
	}
}

func TestOIDCCORSApplicationPreflightAndErrors(t *testing.T) {
	engine, _, _, called := corsFixture(t)
	response := corsRequest(engine, "OPTIONS", "/oidc/cors-client/token", "https://pagnode.mliev.com", "POST", "content-type, AUTHORIZATION")
	require.Equal(t, 204, response.Code)
	require.Equal(t, 0, *called)
	require.Equal(t, "https://pagnode.mliev.com", response.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, response.Header().Get("Access-Control-Allow-Credentials"))
	for _, value := range []string{"Origin", "Accept-Encoding", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
		require.Contains(t, response.Header().Get("Vary"), value)
	}
	response = corsRequest(engine, "POST", "/oidc/cors-client/token", "https://pagnode.mliev.com", "", "")
	require.Equal(t, 401, response.Code, "OAuth errors should remain visible to allowed browsers")
	require.Equal(t, "https://pagnode.mliev.com", response.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, response.Body.String(), "invalid_client")
}

func TestOIDCCORSIsolationAndLiveCallbackChanges(t *testing.T) {
	engine, db, app, called := corsFixture(t)
	for _, origin := range []string{"https://pagnode.mliev.com.evil.test", "https://pagnode.mliev.com:444", "http://pagnode.mliev.com", "null", "https://pagnode.mliev.com/", "http://localhost:5173"} {
		before := *called
		preflight := corsRequest(engine, "OPTIONS", "/oidc/cors-client/token", origin, "POST", "authorization")
		require.Equal(t, 403, preflight.Code, origin)
		require.Empty(t, preflight.Header().Get("Access-Control-Allow-Origin"), origin)
		require.Equal(t, before, *called)
		actual := corsRequest(engine, "POST", "/oidc/cors-client/token", origin, "", "")
		require.Equal(t, 401, actual.Code)
		require.Empty(t, actual.Header().Get("Access-Control-Allow-Origin"))
		require.Contains(t, actual.Header().Get("Vary"), "Origin")
	}
	app.ProtocolConfig["redirectUris"] = []string{"https://other.example/callback"}
	require.NoError(t, db.Save(app).Error)
	require.Equal(t, 403, corsRequest(engine, "OPTIONS", "/oidc/cors-client/token", "https://pagnode.mliev.com", "POST", "").Code)
	require.Equal(t, 204, corsRequest(engine, "OPTIONS", "/oidc/cors-client/token", "https://other.example", "POST", "").Code)
	require.NoError(t, db.Model(app).Update("is_active", false).Error)
	require.Equal(t, 403, corsRequest(engine, "OPTIONS", "/oidc/cors-client/token", "https://other.example", "POST", "").Code)
	require.Equal(t, 401, corsRequest(engine, "POST", "/oidc/cors-client/token", "", "", "").Code)
	require.Equal(t, 403, corsRequest(engine, "OPTIONS", "/oidc/missing/token", "https://other.example", "POST", "").Code)
}

func TestOIDCCORSMethodsHeadersAndOtherRoutes(t *testing.T) {
	engine, db, _, _ := corsFixture(t)
	for _, tc := range []struct{ path, method, headers string }{{"/oidc/cors-client/token", "GET", ""}, {"/oidc/cors-client/jwks", "POST", ""}, {"/oidc/cors-client/userinfo", "DELETE", ""}, {"/oidc/cors-client/token", "POST", "x-unapproved"}} {
		require.Equal(t, 403, corsRequest(engine, "OPTIONS", tc.path, "https://pagnode.mliev.com", tc.method, tc.headers).Code)
	}
	for _, endpoint := range []string{"userinfo", "revoke", "introspect"} {
		require.Equal(t, 204, corsRequest(engine, "OPTIONS", "/oidc/cors-client/"+endpoint, "https://pagnode.mliev.com", "POST", "authorization").Code)
	}
	for _, path := range []string{"/api/admin/profile", "/api/oidc/client-config", "/oidc/cors-client/authorize", "/oidc/cors-client/token/extra", "/other/.well-known/openid-configuration"} {
		require.Empty(t, corsRequest(engine, "GET", path, "https://pagnode.mliev.com", "", "").Header().Get("Access-Control-Allow-Origin"), path)
	}
	local := corsRequest(engine, "GET", "/api/admin/profile", "http://localhost:5173", "", "")
	require.Equal(t, "http://localhost:5173", local.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", local.Header().Get("Access-Control-Allow-Credentials"))
	require.Contains(t, local.Header().Get("Vary"), "Accept-Encoding")
	require.Empty(t, corsRequest(engine, "GET", "/install", "http://localhost:5173", "", "").Header().Get("Access-Control-Allow-Origin"))
	require.NoError(t, db.Migrator().DropTable(&model.Application{}))
	require.Equal(t, 403, corsRequest(engine, "OPTIONS", "/oidc/cors-client/token", "http://localhost:5173", "POST", "").Code)
	// Public metadata remains CORS-readable even if its handler returns an error.
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.GET("/oidc/error/jwks", func(c httpInterfaces.RouterContextInterface) { c.JSON(500, map[string]string{"error": "server_error"}) })
	response := corsRequest(engine, "GET", "/oidc/error/jwks", "https://any.example", "", "")
	require.Equal(t, 500, response.Code)
	require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
	require.Len(t, response.Header().Values("Access-Control-Allow-Origin"), 1)
	require.False(t, strings.Contains(response.Header().Get("Access-Control-Allow-Origin"), ","))
}

func TestOIDCCORSOriginNormalizationAndUnsafeRedirects(t *testing.T) {
	engine, db, app, _ := corsFixture(t)
	for _, tc := range []struct {
		redirect, origin string
		allowed          bool
	}{
		{"HTTPS://PAGNODE.MLIEV.COM:443/callback?app=1", "https://pagnode.mliev.com", true},
		{"http://localhost:80/callback", "http://localhost", true},
		{"http://127.0.0.1:5100/callback", "http://127.0.0.1:5101", false},
		{"http://[0:0:0:0:0:0:0:1]:5100/callback", "http://[::1]:5100", true},
		{"https://例子.测试/callback", "https://xn--fsqu00a.xn--0zwm56d", true},
		{"/callback", "https://pagnode.mliev.com", false},
		{"myapp://pagnode.mliev.com/callback", "https://pagnode.mliev.com", false},
		{"https://user@pagnode.mliev.com/callback", "https://pagnode.mliev.com", false},
		{"https://pagnode.mliev.com/callback#fragment", "https://pagnode.mliev.com", false},
		{"https://pagnode.mliev.com:99999/callback", "https://pagnode.mliev.com", false},
	} {
		app.ProtocolConfig["redirectUris"] = []string{tc.redirect}
		require.NoError(t, db.Save(app).Error)
		response := corsRequest(engine, "OPTIONS", "/oidc/cors-client/token", tc.origin, "POST", "")
		if tc.allowed {
			require.Equal(t, 204, response.Code, tc.redirect)
			require.Equal(t, tc.origin, response.Header().Get("Access-Control-Allow-Origin"))
		} else {
			require.Equal(t, 403, response.Code, tc.redirect)
			require.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
		}
	}
	for _, path := range []string{"/oidc//jwks", "/oidc/./jwks", "/oidc/../jwks", "/oidc/cors-client/jwks/extra", "/prefix/oidc/cors-client/jwks"} {
		request := httptest.NewRequest("GET", path, nil)
		_, methods, _ := oidcCORSEndpoint(request)
		require.Nil(t, methods, path)
	}
	for _, origin := range []string{"null", "https://trusted.example?", "https://trusted.example#", "https://trusted.example/", "https://trusted.example:bad", "https://user@trusted.example", "https://*.example", "https://[::1%25en0]", "https://[not-an-ipv6-address]", " https://trusted.example"} {
		_, ok := oidcURLOrigin(origin, true)
		require.False(t, ok, origin)
	}
	for _, path := range []string{"/oidc%2fcors-client/jwks", "/oidc%2fcors-client/token"} {
		response := corsRequest(engine, "OPTIONS", path, "http://localhost:5173", "POST", "")
		require.Equal(t, 403, response.Code)
		require.Empty(t, response.Header().Get("Access-Control-Allow-Origin"), "encoded separators must not bypass app isolation via global CORS")
	}
	header := http.Header{"Vary": []string{"Accept-Encoding, origin", "ORIGIN, Accept-Language"}}
	appendCORSVary(header, "Origin", "Access-Control-Request-Headers")
	require.Equal(t, "Accept-Encoding, origin, Accept-Language, Access-Control-Request-Headers", header.Get("Vary"))
	header.Set("Vary", "*")
	appendCORSVary(header, "Origin")
	require.Equal(t, "*", header.Get("Vary"))
}
