package controller

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"github.com/stretchr/testify/require"
)

func TestFirstPartyCallbackPreservesRememberedUserCookie(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", strings.Repeat("first-party-test-key", 4))
	db := registerControllerTestDatabase(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.LoginSession{}))
	user := model.User{Username: "remembered", Status: model.UserStatusActive, IsActivated: true}
	require.NoError(t, db.Create(&user).Error)
	session, err := service.NewLoginSessionService(db).Create(user.ID, "password", "127.0.0.1", "", time.Now().Add(time.Hour))
	require.NoError(t, err)
	jwtService, err := service.NewJWTAuthServiceWithError()
	require.NoError(t, err)
	profile := &dto.UserResponse{ID: user.ID, Username: user.Username}
	raw, err := jwtService.GenerateUserToken(profile, session.ID)
	require.NoError(t, err)
	current := newTestRouterContext(nil)
	current.req.AddCookie(&http.Cookie{Name: middleware.UserCookieName, Value: raw})
	require.NoError(t, EstablishExistingUserSession(current, profile, session.ID))
	require.Empty(t, current.setCookie, "callback must not reset the original cookie's persistence or JWT")
	missing := newTestRouterContext(nil)
	require.NoError(t, EstablishExistingUserSession(missing, profile, session.ID))
	require.Len(t, missing.setCookie, 1)
	claims, err := middleware.ValidateUserSessionToken(missing.setCookie[0].Value)
	require.NoError(t, err)
	require.Equal(t, session.ID, claims.SessionID)
	require.NoError(t, db.Model(&model.LoginSession{}).Where("id = ?", session.ID).Update("ended_at", time.Now()).Error)
	require.Error(t, EstablishExistingUserSession(current, profile, session.ID), "revoked sessions cannot reuse the cookie")
}
