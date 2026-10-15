package service

import (
	"context"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
)

func TestAuthenticationContactChangesInvalidateVerifiedAddressesAndChallenges(t *testing.T) {
	for _, source := range []string{"admin", "directory"} {
		t.Run(source, func(t *testing.T) {
			s, u, session := twoFactorFixture(t)
			ctx := context.Background()
			// A management challenge must become unusable even when two-step login is disabled.
			_, tr, err := s.Reauthenticate(ctx, u.ID, session.ID, "", "browser", "user")
			require.NoError(t, err)
			require.NotNil(t, tr)
			switch source {
			case "admin":
				_, err = NewUserService(s.db).UpdateUser(u.ID, &dto.UpdateUserRequest{Email: "new@example.test", Phone: *u.Phone})
				require.NoError(t, err)
			case "directory":
				principal := model.ExternalPrincipal{ConnectionID: 1, UserID: &u.ID, Status: model.PrincipalStatusActive}
				require.NoError(t, s.db.Create(&principal).Error)
				err = NewDirectorySyncService(s.db).applyUserChange(&model.DirectorySyncChange{ChangeType: model.DirectoryChangeUpdate, AfterData: model.JSONObject{"principalId": principal.ID, "userId": u.ID, "email": "new@example.test"}}, &model.DirectorySyncPolicy{})
				require.NoError(t, err)
			}
			var current model.User
			require.NoError(t, s.db.First(&current, u.ID).Error)
			require.Nil(t, current.EmailVerifiedAt)
			require.NotNil(t, current.PhoneVerifiedAt)
			_, err = s.Read(ctx, tr.ID, "browser")
			require.ErrorIs(t, err, ErrTwoFactorExpired)
			_, err = dao.NewLoginSessionDAO(s.db).Valid(session.ID, u.ID)
			require.Error(t, err)
		})
	}
}
func TestAuthenticationAdminMayExplicitlyVerifyNewContact(t *testing.T) {
	s, u, _ := twoFactorFixture(t)
	yes := true
	_, err := NewUserService(s.db).UpdateUser(u.ID, &dto.UpdateUserRequest{Email: "new@example.test", Phone: *u.Phone, EmailVerified: &yes})
	require.NoError(t, err)
	require.NoError(t, s.db.First(u, u.ID).Error)
	require.NotNil(t, u.EmailVerifiedAt)
	require.Equal(t, "new@example.test", *u.Email)
}
func TestAuthenticationUnbindUsesRemainingPasskeyAndPreservesTwoSteps(t *testing.T) {
	s, f, u, session := passkeyFixture(t)
	_, session = addTestPasskey(t, s, f, u, session)
	principal := model.ExternalPrincipal{ConnectionID: 1, UserID: &u.ID, Status: model.PrincipalStatusActive}
	require.NoError(t, s.db.Create(&principal).Error)
	require.NoError(t, s.db.Model(u).Update("password_hash", "").Error)
	// External identity can be removed when Passkey is the remaining primary method.
	proof, err := NewExternalAuthService(s.db, s.redis).UnbindWithSession(u.ID, principal.ID, session.ID, f)
	require.NoError(t, err)
	_, err = NewLoginSessionService(s.db).CreateWithProof(u.ID, "security_change", "", "", time.Now().Add(time.Hour), proof)
	require.NoError(t, err)
	require.NoError(t, s.db.Model(u).Update("two_factor_enabled", true).Error)
	// With just one method left, a self-service change cannot silently lower the requirement.
	require.ErrorContains(t, f.validateLoginPath(u.ID), "两种")
}
