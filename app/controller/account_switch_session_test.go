package controller

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAdminSessionAfterAccountSwitch(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", strings.Repeat("account-switch-test-", 4))
	db := registerControllerTestDatabase(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Role{}, &model.UserRole{}, &model.LoginSession{}, &model.OIDCToken{}, &model.AuthorizationCode{}))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), 10000))
	users := []model.User{
		{Username: "administrator", Status: model.UserStatusActive, IsActivated: true},
		{Username: "personal", Status: model.UserStatusActive, IsActivated: true},
	}
	require.NoError(t, db.Create(&users).Error)
	roleType := model.RoleTypeSystem
	role := model.Role{Name: "Super admin", Code: "super_admin", Type: &roleType}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&model.UserRole{UserID: users[0].ID, RoleID: role.ID, AssignedBy: users[0].ID, IsActive: true, ApprovalStatus: 1}).Error)
	jwtService, err := service.NewJWTAuthServiceWithError()
	require.NoError(t, err)

	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	// Exercise the production session replacement and cookie-clearing path.
	for _, user := range users {
		profile := &dto.UserResponse{ID: user.ID, Username: user.Username}
		router.POST("/test/login/"+user.Username, func(c httpInterfaces.RouterContextInterface) {
			token, maxAge, sid, err := prepareUserSessionCookie(c, profile, false, "")
			require.NoError(t, err)
			require.NoError(t, finishUserSession(c, token, maxAge, sid, profile.ID))
			c.Status(http.StatusNoContent)
		})
	}
	// Model the verified admin callback using the same role check and signer.
	router.POST("/test/admin-callback", func(c httpInterfaces.RouterContextInterface) {
		raw, err := c.Cookie(middleware.UserCookieName)
		require.NoError(t, err)
		claims, err := middleware.ValidateUserSessionToken(raw)
		require.NoError(t, err)
		isAdmin, err := service.IsSuperAdmin(helper.GetDatabase(), claims.UserID)
		require.NoError(t, err)
		if !isAdmin {
			c.Status(http.StatusForbidden)
			return
		}
		token, err := jwtService.GenerateAdminToken(&dto.UserResponse{ID: claims.UserID, Username: claims.Username}, claims.SessionID)
		require.NoError(t, err)
		c.SetCookie(middleware.AdminCookieName, token, 3600, "/", "", false, true)
		c.Status(http.StatusNoContent)
	})
	admins := router.Group("/api/admin")
	admins.Use(middleware.AdminAuthMiddleware())
	admins.GET("/profile", func(c httpInterfaces.RouterContextInterface) {
		id, ok := middleware.GetCurrentUserID(c)
		require.True(t, ok)
		c.JSON(http.StatusOK, map[string]any{"id": id})
	})
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	origin, err := url.Parse("http://iam.test")
	require.NoError(t, err)
	request := func(method, path, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, origin.String()+path, nil)
		for _, cookie := range jar.Cookies(origin) {
			req.AddCookie(cookie)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		jar.SetCookies(origin, rec.Result().Cookies())
		return rec
	}

	require.Equal(t, http.StatusNoContent, request("POST", "/test/login/administrator", "").Code)
	require.Equal(t, http.StatusNoContent, request("POST", "/test/admin-callback", "").Code)
	require.Equal(t, http.StatusOK, request("GET", "/api/admin/profile", "").Code)
	var previousAdminCookie *http.Cookie
	for _, cookie := range jar.Cookies(origin) {
		if cookie.Name == middleware.AdminCookieName {
			previousAdminCookie = cookie
		}
	}
	require.NotNil(t, previousAdminCookie)

	for _, account := range []string{"personal", "administrator"} {
		require.Equal(t, http.StatusNoContent, request("POST", "/test/login/"+account, "").Code)
		for _, cookie := range jar.Cookies(origin) {
			require.NotEqual(t, middleware.AdminCookieName, cookie.Name, "re-login clears the previous admin session")
		}
		t.Run(account+" without admin cookie", func(t *testing.T) {
			response := request("GET", "/api/admin/profile", "")
			require.Equal(t, http.StatusUnauthorized, response.Code, "missing admin session must trigger reauthentication: %s", response.Body.String())
		})
		// Even if a stale tab sends the old admin cookie, it cannot fall back
		// to the new personal login and turn an expired session into a 403.
		jar.SetCookies(origin, []*http.Cookie{previousAdminCookie})
		t.Run(account+" with revoked admin cookie", func(t *testing.T) {
			response := request("GET", "/api/admin/profile", "")
			require.Equal(t, http.StatusUnauthorized, response.Code, "revoked admin session must trigger reauthentication: %s", response.Body.String())
		})
		jar.SetCookies(origin, []*http.Cookie{{Name: middleware.AdminCookieName, MaxAge: -1, Path: "/"}})
		if account == "personal" {
			require.Equal(t, http.StatusForbidden, request("POST", "/test/admin-callback", "").Code)
		}
	}
	require.Equal(t, http.StatusNoContent, request("POST", "/test/admin-callback", "").Code)
	require.Equal(t, http.StatusOK, request("GET", "/api/admin/profile", "").Code)
	for _, cookie := range jar.Cookies(origin) {
		if cookie.Name == middleware.UserCookieName {
			require.Equal(t, http.StatusForbidden, request("GET", "/api/admin/profile", cookie.Value).Code, "explicit user bearer must not fall back to the admin cookie")
		}
	}
	require.Equal(t, http.StatusUnauthorized, request("GET", "/api/admin/profile", "invalid").Code)
	require.NoError(t, db.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", users[0].ID, role.ID).Update("is_active", false).Error)
	require.Equal(t, http.StatusForbidden, request("GET", "/api/admin/profile", "").Code, "a valid admin cookie must not bypass role revocation")
}
