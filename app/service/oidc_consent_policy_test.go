package service

import (
	"encoding/json"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
)

func TestOIDCSessionAutoApprovesConsent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.Application, *OIDCSession)
		want   bool
	}{
		{"interactive login", func(_ *model.Application, _ *OIDCSession) {}, true},
		{"SSO or legacy session", func(_ *model.Application, s *OIDCSession) { s.HasLoginInteraction = false }, false},
		{"explicit consent", func(_ *model.Application, s *OIDCSession) { s.Prompt = "consent" }, false},
		{"login with explicit consent", func(_ *model.Application, s *OIDCSession) { s.Prompt = "  login  consent  " }, false},
		{"forced login", func(_ *model.Application, s *OIDCSession) { s.Prompt = "login" }, true},
		{"registration", func(_ *model.Application, s *OIDCSession) { s.Prompt = "create" }, true},
		{"pending second factor", func(_ *model.Application, s *OIDCSession) { s.PendingTwoFactorID = "challenge" }, false},
		{"unauthenticated", func(_ *model.Application, s *OIDCSession) { s.IsAuthenticated = false }, false},
		{"missing user", func(_ *model.Application, s *OIDCSession) { s.UserID = nil }, false},
		{"terminal failure", func(_ *model.Application, s *OIDCSession) { s.Error = &dto.AuthorizationFailure{Code: "access_denied"} }, false},
		{"protected third party still confirms SSO", func(a *model.Application, s *OIDCSession) { a.IsProtected = true; s.HasLoginInteraction = false }, false},
		{"admin retains existing policy", func(a *model.Application, s *OIDCSession) {
			a.UUID = AdminConsoleApplicationUUID
			a.IsProtected = true
			s.HasLoginInteraction = false
			s.Prompt = "consent"
		}, true},
		{"account retains existing policy", func(a *model.Application, s *OIDCSession) {
			a.UUID = AccountCenterApplicationUUID
			a.IsProtected = true
			s.HasLoginInteraction = false
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uid := uint(1)
			app := &model.Application{UUID: "ordinary-client"}
			session := &OIDCSession{IsAuthenticated: true, UserID: &uid, HasLoginInteraction: true}
			tc.change(app, session)
			require.Equal(t, tc.want, OIDCSessionAutoApprovesConsent(app, session))
		})
	}
	var legacy OIDCSession
	require.NoError(t, json.Unmarshal([]byte(`{"is_authenticated":true,"user_id":1}`), &legacy))
	require.False(t, OIDCSessionAutoApprovesConsent(&model.Application{UUID: "ordinary-client"}, &legacy))
	require.False(t, OIDCSessionAutoApprovesConsent(nil, &legacy))
	require.False(t, OIDCSessionAutoApprovesConsent(&model.Application{}, nil))
}
