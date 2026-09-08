package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/container"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func sessionFixture(t *testing.T) (*gorm.DB, *model.User, *model.Application, *model.LoginSession) {
	t.Helper()
	container.Register(container.NewSimpleProvider(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop())))
	db := setupOIDCPublicClientTokenDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	user := &model.User{Username: "session-user", Status: model.UserStatusActive, IsActivated: true}
	salt, hash, err := NewUserService(db).GeneratePasswordHash("old-password")
	require.NoError(t, err)
	user.Salt, user.PasswordHash = salt, hash
	require.NoError(t, db.Create(user).Error)
	response, err := NewApplicationService(db).CreateApplication(&dto.CreateApplicationRequest{
		Name: "Session application", ProtocolType: "oidc", ProtocolConfig: map[string]interface{}{
			"redirectUris": []interface{}{"https://rp.example/callback"}, "grantTypes": []interface{}{"authorization_code", "refresh_token"},
			"responseTypes": []interface{}{"code"}, "scopes": []interface{}{"openid", "profile"}, "clientType": "public", "tokenLifetime": 3600, "signingAlgorithm": "HS256",
		},
	}, user.ID)
	require.NoError(t, err)
	app, err := NewApplicationService(db).GetApplicationByClientID(response.ProtocolConfig["clientId"].(string))
	require.NoError(t, err)
	session, err := NewLoginSessionService(db).Create(user.ID, "password", "192.0.2.1", "Mozilla/5.0 Macintosh Chrome/130.0", time.Now().Add(time.Hour))
	require.NoError(t, err)
	return db, user, app, session
}

func TestLoginSessionRotationAndRevocation(t *testing.T) {
	db, user, app, session := sessionFixture(t)
	svc := NewOIDCAuthorizationService(db)
	first, err := svc.GenerateTokens(app, user.ID, []string{"profile"}, "", "https://issuer.example", session.ID)
	require.NoError(t, err)
	old, err := dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(first.RefreshToken)
	require.NoError(t, err)
	code := &model.AuthorizationCode{Code: "pending-code", UserID: user.ID, ClientID: app.UUID, LoginSessionID: session.ID, ExpiresAt: time.Now().Add(time.Minute)}
	require.NoError(t, db.Create(code).Error)
	require.NoError(t, db.Model(old).Update("authorization_code_id", code.ID).Error)
	rotated, err := svc.RotateLoginToken(old, app, "", "https://issuer.example")
	require.NoError(t, err)
	current, err := dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(rotated.RefreshToken)
	require.NoError(t, err)
	require.Equal(t, session.ID, current.LoginSessionID)
	require.Equal(t, &code.ID, current.AuthorizationCodeID)
	_, err = svc.RotateLoginToken(old, app, "", "https://issuer.example")
	require.Error(t, err)
	_, err = dao.NewOIDCTokenDAO(db).GetValidByAccessToken(first.AccessToken)
	require.Error(t, err)
	listed, err := NewLoginSessionService(db).List(LoginSessionQuery{}, session.ID)
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	require.True(t, listed.Items[0].IsCurrent)
	require.Equal(t, "Session application", listed.Items[0].Applications[0].Name)
	require.True(t, listed.Items[0].ExpiresAt.After(session.ExpiresAt))
	payload, err := json.Marshal(listed)
	require.NoError(t, err)
	require.NotContains(t, string(payload), first.RefreshToken)
	require.NotContains(t, string(payload), rotated.AccessToken)
	require.NoError(t, NewLoginSessionService(db).Revoke(user.ID, session.ID, "", "admin_revoked", &user.ID))
	require.NoError(t, NewLoginSessionService(db).Revoke(user.ID, session.ID, "", "logout", &user.ID))
	_, err = dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(rotated.RefreshToken)
	require.Error(t, err)
	_, err = dao.NewOIDCTokenDAO(db).GetValidByAccessToken(rotated.AccessToken)
	require.Error(t, err)
	_, err = svc.GenerateTokens(app, user.ID, []string{"profile"}, "", "https://issuer.example", session.ID)
	require.Error(t, err)
	_, err = svc.ExchangeCodeForToken(&dto.TokenRequest{GrantType: "authorization_code", ClientID: app.UUID, Code: code.Code}, "https://issuer.example")
	require.Error(t, err)
	var ended model.LoginSession
	require.NoError(t, db.First(&ended, "id = ?", session.ID).Error)
	require.Equal(t, "admin_revoked", ended.EndReason)
}

func TestLoginSessionPasswordChangesKeepOnlyCurrent(t *testing.T) {
	db, user, _, current := sessionFixture(t)
	svc := NewLoginSessionService(db)
	other, err := svc.Create(user.ID, "password", "192.0.2.2", "", time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.NoError(t, NewUserService(db).ChangePassword(user.ID, &dto.ChangePasswordRequest{OldPassword: "old-password", NewPassword: "new-password"}, current.ID))
	_, err = dao.NewLoginSessionDAO(db).Valid(current.ID, user.ID)
	require.NoError(t, err)
	_, err = dao.NewLoginSessionDAO(db).Valid(other.ID, user.ID)
	require.Error(t, err)
	var state model.User
	require.NoError(t, db.First(&state, user.ID).Error)
	require.True(t, state.CanLogin())
	require.NoError(t, NewUserService(db).ResetPassword(user.ID, "reset-password"))
	_, err = dao.NewLoginSessionDAO(db).Valid(current.ID, user.ID)
	require.Error(t, err)
	_, err = svc.Create(user.ID, "password", "192.0.2.1", "", time.Now().Add(time.Hour))
	require.NoError(t, err)
}

func TestLoginSessionPasswordAndRevocationRollbackTogether(t *testing.T) {
	db, user, _, session := sessionFixture(t)
	const callback = "test:deny_session_update"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "login_sessions" {
			tx.AddError(errors.New("storage unavailable"))
		}
	}))
	err := NewUserService(db).ResetPassword(user.ID, "replacement-password")
	require.Error(t, err)
	require.NoError(t, db.Callback().Update().Remove(callback))
	var after model.User
	require.NoError(t, db.First(&after, user.ID).Error)
	require.Equal(t, user.PasswordHash, after.PasswordHash)
	_, err = dao.NewLoginSessionDAO(db).Valid(session.ID, user.ID)
	require.NoError(t, err)
}

func TestLoginSessionConcurrentRefreshCannotSurviveRevocation(t *testing.T) {
	db, user, app, session := sessionFixture(t)
	svc := NewOIDCAuthorizationService(db)
	issued, err := svc.GenerateTokens(app, user.ID, []string{"profile"}, "", "https://issuer.example", session.ID)
	require.NoError(t, err)
	old, err := dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(issued.RefreshToken)
	require.NoError(t, err)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var rotated *dto.TokenResponse
	var revokeErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		rotated, _ = svc.RotateLoginToken(old, app, "", "https://issuer.example")
	}()
	go func() {
		defer wg.Done()
		<-start
		revokeErr = NewLoginSessionService(db).Revoke(user.ID, "", "", "admin_revoked", &user.ID)
	}()
	close(start)
	wg.Wait()
	require.NoError(t, revokeErr)
	if rotated != nil {
		_, err := dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(rotated.RefreshToken)
		require.Error(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&model.OIDCToken{}).Where("login_session_id = ? AND is_revoked = ?", session.ID, false).Count(&count).Error)
	require.Zero(t, count)
}

func TestLoginSessionLegacyMachineAndCleanup(t *testing.T) {
	db, user, _, current := sessionFixture(t)
	legacy := &model.OIDCToken{AccessToken: "legacy-access", RefreshToken: "legacy-refresh", ClientID: "client", UserID: user.ID}
	require.NoError(t, db.Create(legacy).Error)
	_, err := dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(legacy.RefreshToken)
	require.Error(t, err)
	machine := &model.OIDCToken{AccessToken: "machine-access", RefreshToken: "machine-unused", ClientID: "client", UserID: 0}
	require.NoError(t, db.Create(machine).Error)
	_, err = dao.NewOIDCTokenDAO(db).GetValidByAccessToken(machine.AccessToken)
	require.NoError(t, err)
	now := time.Now()
	cutoff := now.Add(-90 * 24 * time.Hour)
	old := cutoff.Add(-time.Second)
	for _, record := range []model.LoginSession{
		{ID: "expired-old", UserID: user.ID, ExpiresAt: old},
		{ID: "revoked-old", UserID: user.ID, ExpiresAt: now.Add(time.Hour), EndedAt: &old},
		{ID: "boundary", UserID: user.ID, ExpiresAt: cutoff},
	} {
		require.NoError(t, db.Create(&record).Error)
	}
	require.NoError(t, NewLoginSessionService(db).Cleanup(context.Background(), now))
	var ids []string
	require.NoError(t, db.Model(&model.LoginSession{}).Pluck("id", &ids).Error)
	require.ElementsMatch(t, []string{current.ID, "boundary"}, ids)
	_, err = NewLoginSessionService(db).List(LoginSessionQuery{Status: "history", PageSize: 500}, current.ID)
	require.NoError(t, err)
}
