package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/stretchr/testify/require"
)

func securityMiniProfile(openID, unionID string) *protocol.ExternalProfile {
	return &protocol.ExternalProfile{Identifiers: []protocol.ExternalProfileIdentifier{
		{Namespace: model.IdentifierNamespaceLoginSubject, Value: openID, Primary: true},
		{Namespace: model.IdentifierNamespaceOpenID, Value: openID},
		{Namespace: model.IdentifierNamespaceUnionID, Value: unionID},
	}}
}

func TestMiniProgramUnlinkInvalidatesPendingCrossConnectionLogin(t *testing.T) {
	for _, mfa := range []bool{false, true} {
		t.Run(fmt.Sprintf("two-factor-%t", mfa), func(t *testing.T) {
			s, app, first, user, _ := miniFixture(t)
			ctx := context.Background()
			if mfa {
				user.TwoFactorEnabled = true
				email := "mfa-unlink@example.test"
				now := time.Now()
				user.Email, user.EmailVerifiedAt = &email, &now
				s.factors.send = func(context.Context, string, string, string, time.Duration) error { return nil }
				require.NoError(t, s.db.Save(user).Error)
			}
			require.NoError(t, attachMiniIdentity(s.db, first, "wx-a", "original-open", "union-pending", user.ID))
			second := *first
			second.ID = 0
			second.Identifier = "pending-second"
			second.Config = model.JSONObject{"clientId": "wx-second", "clientSecret": "test-secret", "openPlatformId": "platform-1"}
			require.NoError(t, s.db.Create(&second).Error)
			app.ProtocolConfig["miniProgram"] = model.MiniProgramConfig{Enabled: true, ConnectionID: second.ID, AccountPolicy: "bind_existing"}
			require.NoError(t, s.db.Save(app).Error)
			pending := miniStart(t, s, app, "pending")
			require.Equal(t, "consent_required", pending.State)
			var binding model.ExternalPrincipal
			require.NoError(t, s.db.Where("connection_id = ? AND user_id = ?", first.ID, user.ID).First(&binding).Error)
			_, err := NewExternalAuthService(s.db, s.redis).UnbindWithSession(user.ID, binding.ID, "", s.factors)
			require.NoError(t, err)
			result, err := s.Complete(ctx, app.UUID, pending.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true}, "192.0.2.1", "review")
			require.ErrorIs(t, err, ErrMiniTransaction)
			require.Nil(t, result)
			var count int64
			require.NoError(t, s.db.Model(&model.ExternalPrincipal{}).Where("connection_id = ?", second.ID).Count(&count).Error)
			require.Zero(t, count)
			require.NoError(t, s.db.Model(&model.LoginSession{}).Where("native_transaction_hash = ?", miniHash(pending.TransactionToken)).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestMiniProgramLegacyLoginRejectsGlobalConflictBeforeAddingIdentifiers(t *testing.T) {
	s, _, first, user, _ := miniFixture(t)
	require.NoError(t, attachMiniIdentity(s.db, first, "wx-a", "first-open", "shared-union", user.ID))
	second := *first
	second.ID = 0
	second.Identifier = "conflicting-second"
	second.Config = model.JSONObject{"clientId": "wx-second", "clientSecret": "test-secret", "openPlatformId": "platform-1"}
	require.NoError(t, s.db.Create(&second).Error)
	other := model.User{Username: "conflicting-user", Status: model.UserStatusActive, IsActivated: true}
	require.NoError(t, s.db.Create(&other).Error)
	principal := model.ExternalPrincipal{ConnectionID: second.ID, UserID: &other.ID, Status: model.PrincipalStatusActive}
	require.NoError(t, s.db.Create(&principal).Error)
	// Before WeChat first supplies a UnionID, this legacy binding has only openid.
	for _, namespace := range []model.IdentifierNamespace{model.IdentifierNamespaceLoginSubject, model.IdentifierNamespaceOpenID} {
		require.NoError(t, s.db.Create(&model.ExternalIdentifier{ConnectionID: second.ID, PrincipalID: principal.ID, Namespace: namespace, Value: "second-open", IsPrimary: namespace == model.IdentifierNamespaceLoginSubject}).Error)
	}
	result, err := NewExternalAuthService(s.db, s.redis).completeAuthentication(&second, securityMiniProfile("second-open", "shared-union"), "")
	require.ErrorContains(t, err, "不同账号")
	require.Nil(t, result)
	var count int64
	require.NoError(t, s.db.Model(&model.ExternalIdentifier{}).Where("principal_id = ? AND namespace = ?", principal.ID, model.IdentifierNamespaceUnionID).Count(&count).Error)
	require.Zero(t, count, "a failed login must not persist the conflicting UnionID")
}

func TestMiniProgramAuthenticationVersionChangeInvalidatesPendingLogin(t *testing.T) {
	s, app, c, user, _ := miniFixture(t)
	require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", fmt.Sprintf("open-%d-version", c.ID), "union-version", user.ID))
	pending := miniStart(t, s, app, "version")
	require.NoError(t, InvalidateAuthenticationConfiguration(s.db, user.ID, "review_security_reset"))
	result, err := s.Complete(context.Background(), app.UUID, pending.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true}, "", "")
	require.ErrorIs(t, err, ErrMiniTransaction)
	require.Nil(t, result)
}

func TestMiniProgramExplicitBindingStillRequiresSecondFactor(t *testing.T) {
	s, app, c, user, _ := miniFixture(t)
	ctx := context.Background()
	email := "factor-bound@example.test"
	now := time.Now()
	user.Email = &email
	user.EmailVerifiedAt = &now
	user.TwoFactorEnabled = true
	require.NoError(t, s.db.Save(user).Error)
	app.ProtocolConfig["miniProgram"] = model.MiniProgramConfig{Enabled: true, ConnectionID: c.ID, AccountPolicy: "bind_existing"}
	require.NoError(t, s.db.Save(app).Error)
	var code string
	s.factors.send = func(_ context.Context, method, receiver, generated string, _ time.Duration) error {
		require.Equal(t, "email", method)
		require.Equal(t, email, receiver)
		code = generated
		return nil
	}
	begin := func(jsCode string) *dto.MiniProgramResponse {
		login := miniStart(t, s, app, jsCode)
		bound, err := s.Bind(ctx, app.UUID, login.TransactionToken, user.Username, "old-password", "192.0.2.1")
		require.NoError(t, err)
		pending := miniComplete(t, s, app, bound)
		require.Equal(t, "two_factor_required", pending.State)
		require.Nil(t, pending.Tokens)
		return pending
	}
	first := begin("first-factor")
	second := begin("second-factor")
	var count int64
	require.NoError(t, s.db.Model(&model.WeChatIdentity{}).Count(&count).Error)
	require.Zero(t, count)
	_, err := s.Factor(ctx, app.UUID, first.TransactionToken, dto.MiniProgramFactorRequest{Method: "password", Code: "old-password"}, false, "", "")
	require.Error(t, err, "password used as the primary factor cannot be used again")
	_, err = s.Factor(ctx, app.UUID, first.TransactionToken, dto.MiniProgramFactorRequest{Method: "email"}, true, "", "")
	require.NoError(t, err)
	require.NotEmpty(t, code)
	_, err = s.Factor(ctx, app.UUID, second.TransactionToken, dto.MiniProgramFactorRequest{Method: "email", Code: code}, false, "", "")
	require.Error(t, err, "a code issued for another transaction cannot complete this login")
	_, err = s.Factor(ctx, "other-client", first.TransactionToken, dto.MiniProgramFactorRequest{Method: "email", Code: code}, false, "", "")
	require.ErrorIs(t, err, ErrMiniTransaction)
	done, err := s.Factor(ctx, app.UUID, first.TransactionToken, dto.MiniProgramFactorRequest{Method: "email", Code: code}, false, "", "")
	require.NoError(t, err)
	require.NotNil(t, done.Tokens)
	_, err = s.Factor(ctx, app.UUID, first.TransactionToken, dto.MiniProgramFactorRequest{Method: "email", Code: code}, false, "", "")
	require.ErrorIs(t, err, ErrMiniTransaction)
}

func TestMiniProgramNullCredentialCannotChangeBoundAppID(t *testing.T) {
	s, _, c, user, _ := miniFixture(t)
	require.NoError(t, s.db.AutoMigrate(&model.IdentityConnectionHealthCheck{}))
	connections := NewIdentityConnectionService(s.db)
	require.NoError(t, connections.SaveCredentials(c.ID, model.CredentialPurposeAuthentication, map[string]any{"clientId": "wx-a", "clientSecret": "test-secret"}))
	require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", "null-open", "null-union", user.ID))
	// The config fallback does not change the effective ID while credentials win.
	_, err := connections.Update(c.ID, &dto.UpdateIdentityConnectionRequest{Config: map[string]any{"clientId": "wx-fallback"}})
	require.NoError(t, err)
	for _, value := range []any{nil, 42, false, []string{"wx-other"}} {
		require.Error(t, connections.SaveCredentials(c.ID, model.CredentialPurposeAuthentication, map[string]any{"clientId": value}))
		require.NoError(t, s.db.First(c, c.ID).Error)
		actual, _, err := miniCredential(s.db, c)
		require.NoError(t, err)
		require.Equal(t, "wx-a", actual)
	}
}

func TestMiniProgramMissingBindingCannotBecomeImplicitRebind(t *testing.T) {
	s, app, c, user, _ := miniFixture(t)
	require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", fmt.Sprintf("open-%d-removed", c.ID), "union-removed", user.ID))
	pending := miniStart(t, s, app, "removed")
	// Even if a maintenance path removes the mapping without bumping the user
	// security version, authentication by a formerly linked WeChat ID must fail.
	require.NoError(t, s.db.Where("user_id = ?", user.ID).Delete(&model.WeChatIdentity{}).Error)
	var principal model.ExternalPrincipal
	require.NoError(t, s.db.Where("connection_id = ? AND user_id = ?", c.ID, user.ID).First(&principal).Error)
	require.NoError(t, s.db.Where("principal_id = ?", principal.ID).Delete(&model.ExternalIdentifier{}).Error)
	require.NoError(t, s.db.Delete(&principal).Error)
	result, err := s.Complete(context.Background(), app.UUID, pending.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true}, "", "")
	require.ErrorIs(t, err, ErrMiniTransaction)
	require.Nil(t, result)
}

func TestMiniProgramQRCodeRejectsGlobalConflictDuringScan(t *testing.T) {
	s, _, first, user, _ := miniFixture(t)
	ctx := context.Background()
	require.NoError(t, attachMiniIdentity(s.db, first, "wx-a", "first-open", "scan-union", user.ID))
	second := *first
	second.ID = 0
	second.Identifier = "scan-second"
	second.Config = model.JSONObject{"clientId": "wx-second", "clientSecret": "test-secret", "openPlatformId": "platform-1"}
	require.NoError(t, s.db.Create(&second).Error)
	other := model.User{Username: "scan-other", Status: model.UserStatusActive, IsActivated: true}
	require.NoError(t, s.db.Create(&other).Error)
	principal := model.ExternalPrincipal{ConnectionID: second.ID, UserID: &other.ID, Status: model.PrincipalStatusActive}
	require.NoError(t, s.db.Create(&principal).Error)
	require.NoError(t, s.db.Create(&model.ExternalIdentifier{ConnectionID: second.ID, PrincipalID: principal.ID, Namespace: model.IdentifierNamespaceLoginSubject, Value: "scan-open", IsPrimary: true}).Error)
	qr := NewSocialQRCodeService(s.db, s.redis)
	session := &qrSession{ScanHandle: "conflicting-scan", State: QRStatePending, ConnectionID: second.ID, ConnectionIdentifier: second.Identifier}
	require.NoError(t, qr.saveSession(ctx, session))
	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = miniPhoneRoundTrip(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "api.weixin.qq.com", req.URL.Host)
		require.Equal(t, "/sns/jscode2session", req.URL.Path)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"openid":"scan-open","unionid":"scan-union","session_key":"do-not-store"}`))}, nil
	})
	result, err := qr.Scan(ctx, &dto.MPQRCodeScanRequest{ScanHandle: session.ScanHandle, JsCode: "scan-code"})
	require.ErrorContains(t, err, "不同账号")
	require.Nil(t, result)
	stored, err := qr.loadSession(ctx, session.ScanHandle)
	require.NoError(t, err)
	require.Equal(t, QRStatePending, stored.State)
	require.Zero(t, stored.UserID)
	var aliases int64
	require.NoError(t, s.db.Model(&model.ExternalIdentifier{}).Where("principal_id = ? AND namespace = ?", principal.ID, model.IdentifierNamespaceUnionID).Count(&aliases).Error)
	require.Zero(t, aliases)
}

func TestMiniProgramQRCodeRechecksBindingBeforeConfirmAndPolling(t *testing.T) {
	for _, scenario := range []string{"valid", "unlinked", "conflict", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, c, user, _ := miniFixture(t)
			ctx := context.Background()
			require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", "qr-open", "qr-union", user.ID))
			qr := NewSocialQRCodeService(s.db, s.redis)
			ticket := &qrSession{ScanHandle: "verified-scan", State: QRStateScanned, ConnectionID: c.ID, ConnectionIdentifier: c.Identifier, UserID: user.ID, OpenID: "qr-open", UnionID: "qr-union", ConfirmTokenHash: qrTokenHash("confirm-secret"), PollTokenHash: qrTokenHash("poll-secret")}
			require.NoError(t, qr.saveSession(ctx, ticket))
			switch scenario {
			case "unlinked":
				var principal model.ExternalPrincipal
				require.NoError(t, s.db.Where("connection_id = ? AND user_id = ?", c.ID, user.ID).First(&principal).Error)
				require.NoError(t, NewExternalAuthService(s.db, s.redis).Unbind(user.ID, principal.ID))
			case "disabled":
				require.NoError(t, s.db.Model(c).Update("status", model.ConnectionStatusDisabled).Error)
			case "conflict":
				second := *c
				second.ID = 0
				second.Identifier = "qr-conflict"
				second.Config = model.JSONObject{"clientId": "wx-other", "clientSecret": "secret", "openPlatformId": "platform-1"}
				require.NoError(t, s.db.Create(&second).Error)
				other := model.User{Username: "qr-other", Status: model.UserStatusActive, IsActivated: true}
				require.NoError(t, s.db.Create(&other).Error)
				principal := model.ExternalPrincipal{ConnectionID: second.ID, UserID: &other.ID, Status: model.PrincipalStatusActive}
				require.NoError(t, s.db.Create(&principal).Error)
				require.NoError(t, s.db.Create(&model.ExternalIdentifier{ConnectionID: second.ID, PrincipalID: principal.ID, Namespace: model.IdentifierNamespaceUnionID, Value: "qr-union"}).Error)
			}
			err := qr.Confirm(ctx, &dto.MPQRCodeConfirmRequest{ScanHandle: ticket.ScanHandle, ConfirmToken: "confirm-secret", Action: "confirm"})
			if scenario == "valid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			// A QR could have been confirmed just before a binding changed. Polling
			// must independently recheck instead of trusting the cached user ID.
			ticket.State = QRStateConfirmed
			require.NoError(t, qr.saveSession(ctx, ticket))
			result, err := qr.GetStatus(ctx, ticket.ScanHandle, "poll-secret")
			if scenario == "valid" {
				require.NoError(t, err)
				require.NotNil(t, result.User)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
		})
	}
}

func TestMiniProgramLegacyLearnsUnionIDOnlyAfterRequiredFactors(t *testing.T) {
	for _, mfa := range []bool{false, true} {
		t.Run(fmt.Sprintf("two-factor-%t", mfa), func(t *testing.T) {
			s, _, c, user, _ := miniFixture(t)
			ctx := context.Background()
			require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", "learn-open", "", user.ID))
			if mfa {
				user.TwoFactorEnabled = true
				require.NoError(t, s.db.Save(user).Error)
			}
			auth := NewExternalAuthService(s.db, s.redis)
			response, err := auth.completeAuthentication(c, securityMiniProfile("learn-open", "learn-union"), "")
			require.NoError(t, err)
			require.Equal(t, user.ID, response.User.ID)
			countUnion := func() int64 {
				var count int64
				require.NoError(t, s.db.Model(&model.ExternalIdentifier{}).Where("connection_id = ? AND namespace = ?", c.ID, model.IdentifierNamespaceUnionID).Count(&count).Error)
				return count
			}
			if mfa {
				require.NotEmpty(t, response.PendingContinuationID)
				require.Zero(t, countUnion())
				_, err = auth.BindWithExistingUser(ctx, response.PendingContinuationID, response.PendingContinuationCSRF, user.Username, "old-password")
				require.ErrorIs(t, err, ErrMiniTransaction, "an MFA handoff cannot be used as a new binding request")
				challenge, err := s.factors.BeginLogin(ctx, user.ID, "external", "external", "", false, "browser-binding", response.PendingContinuationID, response.PendingContinuationCSRF)
				require.NoError(t, err)
				_, err = s.factors.Verify(ctx, challenge, "password", "incorrect", "192.0.2.1")
				require.Error(t, err)
				require.Zero(t, countUnion())
				proof, err := s.factors.Verify(ctx, challenge, "password", "old-password", "192.0.2.1")
				require.NoError(t, err)
				require.NoError(t, auth.CompleteTwoFactorBinding(ctx, response.PendingContinuationID, response.PendingContinuationCSRF, proof))
			}
			require.EqualValues(t, 1, countUnion())
			second := *c
			second.ID = 0
			second.Identifier = "learn-second"
			second.Config = model.JSONObject{"clientId": "wx-second", "clientSecret": "secret", "openPlatformId": "platform-1"}
			require.NoError(t, s.db.Create(&second).Error)
			matched, err := resolveMiniIdentity(s.db, &second, "wx-second", "second-open", "learn-union")
			require.NoError(t, err)
			require.Equal(t, user.ID, matched)
		})
	}
}

func TestMiniProgramLegacyMFAHandoffCannotSurviveAuthenticationReset(t *testing.T) {
	s, _, c, user, _ := miniFixture(t)
	ctx := context.Background()
	require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", "old-proof-open", "", user.ID))
	user.TwoFactorEnabled = true
	require.NoError(t, s.db.Save(user).Error)
	auth := NewExternalAuthService(s.db, s.redis)
	response, err := auth.completeAuthentication(c, securityMiniProfile("old-proof-open", "new-proof-union"), "")
	require.NoError(t, err)
	require.NoError(t, InvalidateAuthenticationConfiguration(s.db, user.ID, "review_reset"))
	// Even if a challenge starts after the reset, it cannot authenticate the
	// external identity captured by the older continuation.
	challenge, err := s.factors.BeginLogin(ctx, user.ID, "external", "external", "", false, "browser-binding", response.PendingContinuationID, response.PendingContinuationCSRF)
	require.NoError(t, err)
	proof, err := s.factors.Verify(ctx, challenge, "password", "old-password", "192.0.2.1")
	require.NoError(t, err)
	require.ErrorIs(t, auth.CompleteTwoFactorBinding(ctx, response.PendingContinuationID, response.PendingContinuationCSRF, proof), ErrTwoFactorExpired)
	var count int64
	require.NoError(t, s.db.Model(&model.ExternalIdentifier{}).Where("connection_id = ? AND namespace = ?", c.ID, model.IdentifierNamespaceUnionID).Count(&count).Error)
	require.Zero(t, count)
}
