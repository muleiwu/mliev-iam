package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDiscoverOIDCOptionalURL(t *testing.T) {
	bindThemeTestProvider[*redis.Client](t, nil)
	bindThemeTestProvider[gsr.Logger](t, golog.NewLoggerWithZap(zap.NewNop()))
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.POST("/discover", IdentityConnectionController{}.DiscoverOIDC)
	var issuer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(protocol.OIDCDiscoveryDoc{Issuer: issuer, AuthorizationEndpoint: issuer + r.URL.Path + "/authorize", TokenEndpoint: issuer + "/token", JwksURI: issuer + "/keys"})
	}))
	defer upstream.Close()
	issuer = upstream.URL
	for _, path := range []string{"", "/api/auth/.well-known/openid-configuration"} {
		request := map[string]string{"issuer": issuer}
		if path != "" {
			request["discoveryUrl"] = issuer + path
		}
		payload, err := json.Marshal(request)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/discover", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		var response struct {
			Code int                      `json:"code"`
			Data dto.OIDCDiscoverResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Zero(t, response.Code, recorder.Body.String())
		require.Equal(t, issuer, response.Data.Issuer)
		if path == "" {
			path = "/.well-known/openid-configuration"
		}
		require.Equal(t, issuer+path+"/authorize", response.Data.AuthorizationEndpoint)
	}
	// A discovery URL never substitutes for an expected, explicitly supplied issuer.
	req := httptest.NewRequest(http.MethodPost, "/discover", bytes.NewBufferString(`{"discoveryUrl":"https://idp.test/metadata"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	var response struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotZero(t, response.Code)
}
