package admin

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
	envimpl "cnb.cool/mliev/open/go-web/pkg/server/env/impl"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCustomPermissionAdminHTTPAuthorizationAndValidation(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", strings.Repeat("custom-permission-test-", 4))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Role{}, &model.UserRole{}, &model.LoginSession{}, &model.Application{}, &model.CustomPermission{}, &model.CustomPermissionGrant{}))
	oldDB, _ := container.Get[*gorm.DB]()
	oldEnv, _ := container.Get[gsr.Enver]()
	oldLogger, _ := container.Get[gsr.Logger]()
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[*gorm.DB](), db, 10000))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Enver](), &envimpl.Env{}, 10000))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), 10000))
	t.Cleanup(func() {
		container.Reset(reflect.TypeFor[*gorm.DB]())
		container.Reset(reflect.TypeFor[gsr.Enver]())
		container.Reset(reflect.TypeFor[gsr.Logger]())
		if oldDB != nil {
			container.Register(container.NewSimpleProvider(reflect.TypeFor[*gorm.DB](), oldDB))
		}
		if oldEnv != nil {
			container.Register(container.NewSimpleProvider(reflect.TypeFor[gsr.Enver](), oldEnv))
		}
		if oldLogger != nil {
			container.Register(container.NewSimpleProvider(reflect.TypeFor[gsr.Logger](), oldLogger))
		}
	})
	users := []model.User{{Username: "member", Status: model.UserStatusActive, IsActivated: true}, {Username: "admin", Status: model.UserStatusActive, IsActivated: true}}
	require.NoError(t, db.Create(&users).Error)
	roleType := model.RoleTypeSystem
	role := model.Role{Code: "super_admin", Name: "Super admin", Type: &roleType}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&model.UserRole{UserID: users[1].ID, RoleID: role.ID, AssignedBy: users[1].ID, IsActive: true, ApprovalStatus: 1}).Error)
	tokenService, err := service.NewJWTAuthServiceWithError()
	require.NoError(t, err)
	tokens := []string{}
	for i, user := range users {
		session, err := service.NewLoginSessionService(db).Create(user.ID, "password", "127.0.0.1", "", time.Now().Add(time.Hour))
		require.NoError(t, err)
		response := &dto.UserResponse{ID: user.ID, Username: user.Username}
		var token string
		if i == 0 {
			token, err = tokenService.GenerateUserToken(response, session.ID)
		} else {
			token, err = tokenService.GenerateAdminToken(response, session.ID)
		}
		require.NoError(t, err)
		tokens = append(tokens, token)
	}
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	group := router.Group("/api/admin")
	group.Use(middleware.AdminAuthMiddleware())
	group.GET("/custom-permissions", CustomPermissionController{}.List)
	group.POST("/custom-permissions", CustomPermissionController{}.Create)
	group.GET("/custom-permission-grants", CustomPermissionController{}.ListGrants)
	group.POST("/custom-permission-grants", CustomPermissionController{}.CreateGrant)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		result := httptest.NewRecorder()
		engine.ServeHTTP(result, req)
		return result
	}
	for _, path := range []string{"/api/admin/custom-permissions", "/api/admin/custom-permission-grants"} {
		require.Equal(t, 401, request("GET", path, "", "").Code)
		require.Equal(t, 403, request("GET", path, "", tokens[0]).Code)
		require.Equal(t, 200, request("GET", path, "", tokens[1]).Code)
		require.Equal(t, 403, request("POST", path, "{}", tokens[0]).Code)
	}
	created := request("POST", "/api/admin/custom-permissions", `{"code":"orders:read","name":"Read orders","enabled":false}`, tokens[1])
	require.Equal(t, 200, created.Code, created.Body.String())
	var response struct {
		Data model.CustomPermission `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &response))
	require.False(t, response.Data.Enabled)
	require.Equal(t, users[1].ID, response.Data.CreatedBy)
	require.Equal(t, 400, request("POST", "/api/admin/custom-permissions", `{"code":"openid","name":"Forbidden"}`, tokens[1]).Code)
	invalid := `{"permissionId":1,"subjectType":"everyone","startsAt":"2026-10-14T00:00:00Z"}`
	require.Equal(t, 400, request("POST", "/api/admin/custom-permission-grants", invalid, tokens[1]).Code)
	require.Equal(t, 400, request("GET", "/api/admin/custom-permission-grants?state=unknown", "", tokens[1]).Code)
}
