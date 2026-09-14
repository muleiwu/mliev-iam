package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	internaljwt "cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	"cnb.cool/mliev/open/go-web/pkg/container"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoginSessionHTTPPermissionsAndCookieIsolation(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", strings.Repeat("session-test-key-", 4))
	db := registerControllerTestDatabase(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Role{}, &model.UserRole{}, &model.LoginSession{}, &model.OIDCToken{}, &model.AuthorizationCode{}, &model.Application{}))
	container.Register(container.NewSimpleProviderWithPriority(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop()), 10000))
	users := []model.User{{Username: "personal", Status: model.UserStatusActive, IsActivated: true}, {Username: "administrator", Status: model.UserStatusActive, IsActivated: true}}
	require.NoError(t, db.Create(&users).Error)
	roleType := model.RoleTypeSystem
	role := model.Role{Name: "Super admin", Code: "super_admin", Type: &roleType}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&model.UserRole{UserID: users[1].ID, RoleID: role.ID, AssignedBy: users[1].ID, IsActive: true, ApprovalStatus: 1}).Error)
	sessions := service.NewLoginSessionService(db)
	first, err := sessions.Create(users[0].ID, "password", "192.0.2.1", "", time.Now().Add(time.Hour))
	require.NoError(t, err)
	second, err := sessions.Create(users[1].ID, "password", "192.0.2.2", "", time.Now().Add(time.Hour))
	require.NoError(t, err)
	other, err := sessions.Create(users[0].ID, "password", "192.0.2.3", "", time.Now().Add(time.Hour))
	require.NoError(t, err)
	jwtService, err := service.NewJWTAuthServiceWithError()
	require.NoError(t, err)
	personal, err := jwtService.GenerateUserToken(&dto.UserResponse{ID: users[0].ID, Username: users[0].Username}, first.ID)
	require.NoError(t, err)
	admin, err := jwtService.GenerateAdminToken(&dto.UserResponse{ID: users[1].ID, Username: users[1].Username}, second.ID)
	require.NoError(t, err)
	engine := gin.New()
	router := httpimpl.NewRouter(engine, httpimpl.NewHttpDeps())
	router.POST("/test/admin-logout", func(c httpInterfaces.RouterContextInterface) {
		if err := LogoutBrowserSession(c, true); err != nil {
			(BaseResponse{}).Error(c, 500, err.Error())
			return
		}
		(BaseResponse{}).Success(c, nil)
	})
	self := router.Group("/api/user")
	self.Use(middleware.UserAuthMiddleware())
	self.GET("/sessions", LoginSessionController{}.ListMine)
	self.POST("/sessions/:sid/revoke", LoginSessionController{}.RevokeMine)
	self.POST("/sessions/revoke-others", LoginSessionController{}.RevokeOthers)
	admins := router.Group("/api/admin")
	admins.Use(middleware.AdminAuthMiddleware())
	admins.GET("/sessions", LoginSessionController{}.ListAdmin)
	admins.POST("/sessions/:sid/revoke", LoginSessionController{}.RevokeAdmin)
	admins.POST("/users/:id/sessions/revoke-all", LoginSessionController{}.RevokeAllAdmin)
	request := func(method, path, body, authorization, contentType string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		req.AddCookie(&http.Cookie{Name: middleware.UserCookieName, Value: personal})
		req.AddCookie(&http.Cookie{Name: middleware.AdminCookieName, Value: admin})
		result := httptest.NewRecorder()
		engine.ServeHTTP(result, req)
		return result
	}
	require.Equal(t, 400, request("GET", "/api/user/sessions?status=invalid", "", "", "").Code)
	require.Equal(t, 404, request("POST", "/api/admin/users/999/sessions/revoke-all", "{}", "", "application/json").Code)
	listed := request("GET", "/api/user/sessions?userId=2", "", "", "")
	require.Equal(t, 200, listed.Code)
	var response struct {
		Data service.LoginSessionList `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &response))
	require.Len(t, response.Data.Items, 2)
	for _, session := range response.Data.Items {
		require.Equal(t, users[0].ID, session.UserID)
		require.Equal(t, session.ID == first.ID, session.IsCurrent)
	}
	require.Equal(t, 404, request("POST", "/api/user/sessions/"+second.ID+"/revoke", "{}", "", "application/json").Code)
	require.Equal(t, 400, request("POST", "/api/user/sessions/"+other.ID+"/revoke", "{}", "", "text/plain").Code)
	require.Equal(t, 401, request("GET", "/api/user/sessions", "", "Bearer invalid", "").Code)
	require.Equal(t, 403, request("GET", "/api/admin/sessions", "", "Bearer "+personal, "").Code)
	forged, err := jwtService.GetJWTService().GenerateToken(internaljwt.TokenSubject{UserID: users[0].ID, Username: users[0].Username, Role: "user", SessionID: second.ID}, 1)
	require.NoError(t, err)
	require.Equal(t, 401, request("GET", "/api/user/sessions", "", "Bearer "+forged, "").Code)
	legacy, err := jwtService.GetJWTService().GenerateToken(internaljwt.TokenSubject{UserID: users[0].ID, Username: users[0].Username, Role: "user"}, 1)
	require.NoError(t, err)
	require.Equal(t, 401, request("GET", "/api/user/sessions", "", "Bearer "+legacy, "").Code)
	require.Equal(t, 200, request("POST", "/api/user/sessions/revoke-others", "{}", "", "application/json").Code)
	require.Equal(t, 200, request("GET", "/api/user/sessions", "", "", "").Code)
	ended := request("POST", "/api/user/sessions/"+first.ID+"/revoke", "{}", "", "application/json")
	require.Equal(t, 200, ended.Code)
	require.Contains(t, ended.Body.String(), `"currentRevoked":true`)
	for _, cookie := range ended.Result().Cookies() {
		require.NotEqual(t, middleware.AdminCookieName, cookie.Name)
	}
	require.Equal(t, 401, request("GET", "/api/user/sessions", "", "Bearer "+personal, "").Code)
	require.Equal(t, 200, request("GET", "/api/admin/sessions", "", "Bearer "+admin, "").Code)
	// A stale admin page with no admin cookie cannot log out a different user's
	// still-valid personal session.
	retained, err := sessions.Create(users[0].ID, "password", "192.0.2.4", "", time.Now().Add(time.Hour))
	require.NoError(t, err)
	retainedToken, err := jwtService.GenerateUserToken(&dto.UserResponse{ID: users[0].ID, Username: users[0].Username}, retained.ID)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/test/admin-logout", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: middleware.UserCookieName, Value: retainedToken})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	_, err = dao.NewLoginSessionDAO(db).Valid(retained.ID, users[0].ID)
	require.NoError(t, err)
	for _, cookie := range rec.Result().Cookies() {
		require.NotEqual(t, middleware.UserCookieName, cookie.Name)
	}

}
