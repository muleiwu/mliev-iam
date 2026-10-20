package admin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
	configimpl "cnb.cool/mliev/open/go-web/pkg/server/config/impl"
	envimpl "cnb.cool/mliev/open/go-web/pkg/server/env/impl"
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

// The DI registry retains providers on Reset and only accepts strictly higher
// priorities. Restore both the value and a low priority so later tests can bind.
type themeTestProvider struct {
	typ      reflect.Type
	mu       sync.Mutex
	value    any
	priority atomic.Int64
}

func (p *themeTestProvider) Type() reflect.Type        { return p.typ }
func (p *themeTestProvider) Priority() int             { return int(p.priority.Load()) }
func (p *themeTestProvider) DependsOn() []reflect.Type { return nil }
func (p *themeTestProvider) Build() any                { p.mu.Lock(); defer p.mu.Unlock(); return p.value }
func bindThemeTestProvider[T any](t *testing.T, value T) {
	old, _ := container.Get[T]()
	p := &themeTestProvider{typ: reflect.TypeFor[T](), value: value}
	p.priority.Store(1 << 30)
	container.Register(p)
	t.Cleanup(func() { p.mu.Lock(); p.value = old; p.mu.Unlock(); p.priority.Store(-1); container.Reset(p.typ) })
}

func themeHTTPFixture(t *testing.T) (*gin.Engine, []string) {
	t.Helper()
	t.Setenv("JWT_SECRET_KEY", strings.Repeat("theme-package-test-", 4))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Role{}, &model.UserRole{}, &model.LoginSession{}, &model.Setting{}, &model.AppearanceTheme{}, &model.Attachment{}))
	cfg := configimpl.NewConfig()
	cfg.Set("http.static_mode", "disk")
	bindThemeTestProvider[*gorm.DB](t, db)
	bindThemeTestProvider[gsr.Enver](t, &envimpl.Env{})
	bindThemeTestProvider[gsr.Logger](t, golog.NewLoggerWithZap(zap.NewNop()))
	bindThemeTestProvider[gsr.Provider](t, cfg)
	users := []model.User{{Username: "theme-member", Status: model.UserStatusActive, IsActivated: true}, {Username: "theme-admin", Status: model.UserStatusActive, IsActivated: true}}
	require.NoError(t, db.Create(&users).Error)
	roleType := model.RoleTypeSystem
	role := model.Role{Code: "super_admin", Name: "Administrator", Type: &roleType}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&model.UserRole{UserID: users[1].ID, RoleID: role.ID, AssignedBy: users[1].ID, IsActive: true, ApprovalStatus: 1}).Error)
	jwt, err := service.NewJWTAuthServiceWithError()
	require.NoError(t, err)
	tokens := []string{}
	for i, user := range users {
		session, err := service.NewLoginSessionService(db).Create(user.ID, "password", "127.0.0.1", "", time.Now().Add(time.Hour))
		require.NoError(t, err)
		info := &dto.UserResponse{ID: user.ID, Username: user.Username}
		var token string
		if i == 0 {
			token, err = jwt.GenerateUserToken(info, session.ID)
		} else {
			token, err = jwt.GenerateAdminToken(info, session.ID)
		}
		require.NoError(t, err)
		tokens = append(tokens, token)
	}
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	group := router.Group("/api/admin")
	group.Use(middleware.AdminAuthMiddleware())
	c := AppearanceThemeController{}
	group.GET("/appearance-themes", c.List)
	group.GET("/appearance-themes/:id", c.Get)
	group.POST("/appearance-themes/import", c.Import)
	group.DELETE("/appearance-themes/:id", c.Delete)
	group.POST("/settings/appearance/export", c.Export)
	group.GET("/settings/appearance/config", SettingController{}.GetAppearanceSettings)
	group.PUT("/settings/appearance/config", SettingController{}.UpdateAppearanceSettings)
	group.GET("/auth/profile", AuthController{}.GetAdminProfile)
	group.GET("/capabilities", func(ctx httpInterfaces.RouterContextInterface) {
		controller.BaseResponse{}.Success(ctx, map[string]any{"edition": "community", "version": "test", "features": map[string]bool{}})
	})
	router.GET("/api/appearance", controller.AppearanceController{}.Get)
	return engine, tokens
}
func TestThemePackageHTTPAuthorizationAndImport(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, name := range []string{"README.md", "preview.mjs", "preview/index.html", "preview/canvas.html"} {
		file := filepath.Join("static/auth/theme-kit", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0755))
		require.NoError(t, os.WriteFile(file, []byte("trusted fixture"), 0644))
	}
	engine, tokens := themeHTTPFixture(t)
	request := func(method, path string, body []byte, contentType, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	for _, route := range []struct{ method, path string }{{"GET", "/api/admin/appearance-themes"}, {"GET", "/api/admin/appearance-themes/unknown"}, {"POST", "/api/admin/appearance-themes/import"}, {"DELETE", "/api/admin/appearance-themes/unknown"}, {"POST", "/api/admin/settings/appearance/export"}} {
		require.Equal(t, 401, request(route.method, route.path, nil, "", "").Code)
		require.Equal(t, 403, request(route.method, route.path, nil, "", tokens[0]).Code)
	}
	body := []byte(`{"name":"HTTP Theme","version":"1.0.0","settings":{"mode":"custom","custom":{"preset":"grove","desktopAlignment":"right","customCss":".auth-entry-title{letter-spacing:1px}"}}}`)
	exported := request("POST", "/api/admin/settings/appearance/export", body, "application/json", tokens[1])
	require.Equal(t, 200, exported.Code, exported.Body.String())
	require.Equal(t, "application/zip", exported.Header().Get("Content-Type"))
	_, err := zip.NewReader(bytes.NewReader(exported.Body.Bytes()), int64(exported.Body.Len()))
	require.NoError(t, err)
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("file", "theme.zip")
	require.NoError(t, err)
	_, err = part.Write(exported.Body.Bytes())
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	imported := request("POST", "/api/admin/appearance-themes/import", upload.Bytes(), writer.FormDataContentType(), tokens[1])
	require.Equal(t, 200, imported.Code, imported.Body.String())
	var result struct {
		Data dto.AppearanceThemeResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(imported.Body.Bytes(), &result))
	require.Equal(t, "grove", result.Data.Appearance.Preset)
	require.Equal(t, 409, request("POST", "/api/admin/appearance-themes/import", upload.Bytes(), writer.FormDataContentType(), tokens[1]).Code)
	active := request("GET", "/api/appearance", nil, "", "")
	require.Contains(t, active.Body.String(), `"mode":"default"`)
	saved, _ := json.Marshal(dto.AppearanceSettings{Mode: "custom", Custom: *result.Data.Appearance})
	require.Equal(t, 200, request("PUT", "/api/admin/settings/appearance/config", saved, "application/json", tokens[1]).Code)
	require.Equal(t, 409, request("DELETE", "/api/admin/appearance-themes/"+result.Data.ID, nil, "", tokens[1]).Code)
	require.Equal(t, 400, request("POST", "/api/admin/settings/appearance/export", []byte(`{"name":"x"}`), "application/json", tokens[1]).Code)
}

func TestAppearanceThemeBrowserFixture(t *testing.T) {
	if os.Getenv("IAM_THEME_BROWSER_TEST") != "1" {
		t.Skip("opt-in local browser fixture")
	}
	t.Chdir("../../..")
	engine, tokens := themeHTTPFixture(t)
	engine.Static("/admin", "static/admin")
	engine.Static("/auth", "static/auth")
	engine.GET("/__test/login", func(c *gin.Context) {
		c.SetCookie(middleware.AdminCookieName, tokens[1], 3600, "/", "", false, true)
		c.Redirect(302, "/admin/#/system-settings/appearance-settings")
	})
	done := make(chan struct{}, 1)
	engine.POST("/__test/stop", func(c *gin.Context) {
		select {
		case done <- struct{}{}:
		default:
		}
		c.Status(204)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:8892")
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(engine)
	server.Listener = listener
	server.Start()
	defer server.Close()
	t.Log("theme browser fixture ready at http://127.0.0.1:8892/__test/login")
	select {
	case <-done:
	case <-time.After(15 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
