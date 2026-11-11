package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
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

func TestCaptchaParamsHTTPFreshEncryptionAndFailureBeforeSession(t *testing.T) {
	db := registerControllerTestDatabase(t)
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.CaptchaConfig{}, &model.Setting{}))
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { rdb.Close() })
	priority := int(atomic.AddInt64(&controllerTestDBProviderPriority, 1))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[*redis.Client](), rdb, priority))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), priority))
	cfg := model.CaptchaConfig{Name: "web", Provider: "tencent", Enabled: true, Params: datatypes.JSONMap{"secretId": "private-id", "secretKey": "private-cloud-key", "captchaAppId": "123456", "appSecretKey": strings.Repeat("x", 33)}}
	require.NoError(t, db.Create(&cfg).Error)
	require.NoError(t, db.Create(&model.Setting{Key: service.CaptchaSceneBindingsKey, Value: fmt.Sprintf(`{"login":%d}`, cfg.ID), Category: service.CaptchaCategory}).Error)
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.POST("/api/captcha/params", CaptchaController{}.GetCaptchaParams)
	request := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/captcha/params", strings.NewReader(`{"scene":"login","fingerprint":"test-device"}`))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(rec, req)
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		require.NotContains(t, rec.Body.String(), "private-")
		return rec
	}
	failed := request()
	require.Equal(t, 500, failed.Code, failed.Body.String())
	require.Empty(t, server.Keys(), "parameter failures must not allocate a session")
	cfg.Params["appSecretKey"] = "private-app-key"
	require.NoError(t, db.Save(&cfg).Error)
	var previous dto.CaptchaClientParamsResponse
	for i := 0; i < 2; i++ {
		rec := request()
		require.Equal(t, 200, rec.Code, rec.Body.String())
		var body struct {
			Data dto.CaptchaClientParamsResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, 300, body.Data.ExpiresIn)
		require.Equal(t, "cbc", body.Data.Params["aidEncryptedType"])
		require.NotEmpty(t, body.Data.SessionUuid)
		require.NotEqual(t, previous.SessionUuid, body.Data.SessionUuid)
		require.NotEmpty(t, body.Data.Params["aidEncrypted"])
		require.NotEqual(t, previous.Params["aidEncrypted"], body.Data.Params["aidEncrypted"])
		previous = body.Data
	}
	require.Len(t, server.Keys(), 2)
}
