package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/alicebob/miniredis/v2"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func miniFixture(t *testing.T) (*MiniProgramService, *model.Application, *model.IdentityConnection, *model.User, *miniredis.Miniredis) {
	t.Helper()
	t.Setenv(credentialcrypto.EnvironmentKey, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	db, user, app, _, _ := policyFixture(t)
	require.NoError(t, db.AutoMigrate(&model.IdentityConnection{}, &model.IdentityConnectionCredential{}, &model.ExternalPrincipal{}, &model.ExternalIdentifier{}, &model.ExternalPrincipalConflict{}, &model.WeChatIdentity{}, &model.WeChatIdentityConflict{}, &model.Setting{}, &model.Agreement{}, &model.AgreementAcceptance{}, &model.TwoFactorMethod{}, &model.TwoFactorRecoveryCode{}, &model.TwoFactorEvent{}, &model.Passkey{}))
	c := &model.IdentityConnection{Name: "Mini A", Identifier: "mini-a", Category: model.ConnectionCategorySocial, ProviderType: "wechat_miniprogram", Status: model.ConnectionStatusEnabled, Capabilities: model.StringList{"authentication"}, Config: model.JSONObject{"openPlatformId": "platform-1", "clientId": "wx-a", "clientSecret": "test-secret"}}
	require.NoError(t, db.Create(c).Error)
	app.ProtocolConfig["miniProgram"] = model.MiniProgramConfig{Enabled: true, ConnectionID: c.ID, AccountPolicy: "auto_register"}
	app.ProtocolConfig["clientType"] = "public"
	app.ProtocolConfig["grantTypes"] = []string{"refresh_token"}
	app.ProtocolConfig["scopes"] = []string{"openid", "profile", "phone"}
	app.ProtocolConfig["scopeAuthorization"] = model.OIDCScopeAuthorization{}
	app.ProtocolConfig["roleClaims"] = model.OIDCRoleClaimsConfig{}
	app.ProtocolConfig["customPermissionIds"] = []uint{}
	app.AccessScope = policyAccess(true)
	require.NoError(t, db.Save(app).Error)
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { rdb.Close() })
	svc := NewMiniProgramService(db, rdb, nil)
	svc.exchange = func(_ context.Context, c *model.IdentityConnection, code string) (*protocol.ExternalProfile, error) {
		return &protocol.ExternalProfile{Identifiers: []protocol.ExternalProfileIdentifier{{Namespace: model.IdentifierNamespaceOpenID, Value: fmt.Sprintf("open-%d-%s", c.ID, code)}, {Namespace: model.IdentifierNamespaceUnionID, Value: "union-" + code}}}, nil
	}
	svc.phone = func(_ context.Context, _, _, _, _ string) (string, error) { return "+8613800138000", nil }
	return svc, app, c, user, server
}
func miniStart(t *testing.T, s *MiniProgramService, app *model.Application, code string) *dto.MiniProgramResponse {
	t.Helper()
	r, e := s.Login(context.Background(), app.UUID, dto.MiniProgramLoginRequest{JsCode: code}, "https://iam.test/oidc/"+app.UUID)
	require.NoError(t, e)
	require.Nil(t, r.Tokens)
	return r
}
func miniComplete(t *testing.T, s *MiniProgramService, app *model.Application, r *dto.MiniProgramResponse) *dto.MiniProgramResponse {
	t.Helper()
	out, err := s.Complete(context.Background(), app.UUID, r.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true, Profile: dto.MiniProgramProfile{Email: "new@example.test", RealName: "New user"}}, "192.0.2.1", "mini-program")
	require.NoError(t, err)
	return out
}
func miniClaims(t *testing.T, raw string) jwt.MapClaims {
	t.Helper()
	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(raw, claims)
	require.NoError(t, err)
	return claims
}

func TestMiniProgramRegistrationAndScopedTokens(t *testing.T) {
	s, app, _, _, _ := miniFixture(t)
	ctx := context.Background()
	start := miniStart(t, s, app, "new")
	require.Equal(t, "phone_required", start.State)
	_, err := s.Complete(ctx, app.UUID, start.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true}, "", "")
	require.Error(t, err)
	phone, err := s.Phone(ctx, app.UUID, start.TransactionToken, "phone-code")
	require.NoError(t, err)
	require.Equal(t, "profile_required", phone.State)
	_, err = s.Complete(ctx, app.UUID, start.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true}, "", "")
	require.ErrorContains(t, err, "邮箱")
	var count int64
	s.db.Model(&model.WeChatIdentity{}).Count(&count)
	require.Zero(t, count)
	done := miniComplete(t, s, app, phone)
	require.Equal(t, "authenticated", done.State)
	require.True(t, done.IsNewUser)
	require.Empty(t, done.TransactionToken)
	claims := miniClaims(t, done.Tokens.AccessToken)
	require.Contains(t, fmt.Sprint(claims["aud"]), app.UUID)
	require.Equal(t, "https://iam.test/oidc/"+app.UUID, claims["iss"])
	var created model.User
	require.NoError(t, s.db.Where("phone = ?", "+8613800138000").First(&created).Error)
	require.Empty(t, created.PasswordHash)
	require.NotNil(t, created.PhoneVerifiedAt)
	require.Nil(t, created.EmailVerifiedAt)
	_, err = s.Complete(ctx, app.UUID, start.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true}, "", "")
	require.ErrorIs(t, err, ErrMiniTransaction)
	auth := NewOIDCAuthorizationService(s.db)
	refreshed, err := auth.RefreshToken(&dto.TokenRequest{ClientID: app.UUID, GrantType: "refresh_token", RefreshToken: done.Tokens.RefreshToken}, "https://iam.test/oidc/"+app.UUID)
	require.NoError(t, err)
	_, err = auth.RefreshToken(&dto.TokenRequest{ClientID: app.UUID, GrantType: "refresh_token", RefreshToken: done.Tokens.RefreshToken}, "https://iam.test/oidc/"+app.UUID)
	require.Error(t, err)
	stored, err := dao.NewOIDCTokenDAO(s.db).GetByAccessToken(refreshed.AccessToken)
	require.NoError(t, err)
	require.NoError(t, NewLoginSessionService(s.db).Revoke(created.ID, stored.LoginSessionID, "", "logout", nil))
	_, err = auth.RefreshToken(&dto.TokenRequest{ClientID: app.UUID, GrantType: "refresh_token", RefreshToken: refreshed.RefreshToken}, "https://iam.test/oidc/"+app.UUID)
	require.Error(t, err)
}

func TestMiniProgramCrossAppUnionAndIsolation(t *testing.T) {
	s, app, c, user, _ := miniFixture(t)
	require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", fmt.Sprintf("open-%d-shared", c.ID), "union-shared", user.ID))
	first := miniComplete(t, s, app, miniStart(t, s, app, "shared"))
	other := *c
	other.ID = 0
	other.Identifier = "mini-b"
	other.Config = model.JSONObject{"clientId": "wx-b", "clientSecret": "secret-b", "openPlatformId": "platform-1"}
	require.NoError(t, s.db.Create(&other).Error)
	secondApp := *app
	secondApp.ID = 0
	secondApp.UUID = "mini-client-b"
	secondApp.ProtocolConfig = map[string]any{}
	for key, value := range app.ProtocolConfig {
		secondApp.ProtocolConfig[key] = value
	}
	secondApp.ProtocolConfig["clientId"] = secondApp.UUID
	secondApp.ProtocolConfig["miniProgram"] = model.MiniProgramConfig{Enabled: true, ConnectionID: other.ID, AccountPolicy: "auto_register"}
	require.NoError(t, s.db.Create(&secondApp).Error)
	// Application keys are per client, even though the user is shared.
	var key model.ApplicationOIDCKey
	require.NoError(t, s.db.Where("application_id = ?", app.ID).First(&key).Error)
	key.ID = 0
	key.ApplicationID = secondApp.ID
	key.KeyID = "mini-b-key"
	require.NoError(t, s.db.Create(&key).Error)
	start := miniStart(t, s, &secondApp, "shared")
	require.Equal(t, "consent_required", start.State)
	second := miniComplete(t, s, &secondApp, start)
	require.Equal(t, miniClaims(t, first.Tokens.IDToken)["sub"], miniClaims(t, second.Tokens.IDToken)["sub"])
	require.NotEqual(t, miniClaims(t, first.Tokens.AccessToken)["aud"], miniClaims(t, second.Tokens.AccessToken)["aud"])
	{

		_, err := NewOIDCUserInfoService(s.db).GetUserInfo(first.Tokens.AccessToken, secondApp.UUID)
		require.Error(t, err, "userinfo must reject an access token for another application")
		err = NewOIDCAuthorizationService(s.db).RevokeToken(&dto.RevocationRequest{ClientID: secondApp.UUID, Token: first.Tokens.RefreshToken, TokenTypeHint: "refresh_token"})
		require.Error(t, err, "another application must not revoke the owner's refresh token")
		_, err = dao.NewOIDCTokenDAO(s.db).GetValidByAccessToken(first.Tokens.AccessToken)
		require.NoError(t, err, "failed cross-client revocation must leave the owner's token valid")

	}

	_, err := NewOIDCAuthorizationService(s.db).RefreshToken(&dto.TokenRequest{ClientID: app.UUID, GrantType: "refresh_token", RefreshToken: second.Tokens.RefreshToken}, "https://iam.test/oidc/"+app.UUID)
	require.Error(t, err)
	third := other
	third.ID = 0
	third.Identifier = "mini-c"
	third.Config = model.JSONObject{"clientId": "wx-c", "clientSecret": "secret-c", "openPlatformId": "platform-2"}
	require.NoError(t, s.db.Create(&third).Error)
	resolved, err := resolveMiniIdentity(s.db, &third, "wx-c", "new-open", "union-shared")
	require.NoError(t, err)
	require.Zero(t, resolved)
}

func TestMiniProgramPhoneLinkWaitsForSecondFactor(t *testing.T) {
	s, app, _, user, _ := miniFixture(t)
	ctx := context.Background()
	now := time.Now()
	phone := "+8613800138000"
	salt, hash, err := NewUserService(s.db).GeneratePasswordHash("correct-password")
	require.NoError(t, err)
	user.Phone = &phone
	user.PhoneVerifiedAt = &now
	user.PasswordHash = hash
	user.Salt = salt
	user.TwoFactorEnabled = true
	require.NoError(t, s.db.Save(user).Error)
	start := miniStart(t, s, app, "existing-phone")
	r, err := s.Phone(ctx, app.UUID, start.TransactionToken, "phone")
	require.NoError(t, err)
	require.Equal(t, "consent_required", r.State)
	pending := miniComplete(t, s, app, r)
	require.Equal(t, "two_factor_required", pending.State)
	require.Nil(t, pending.Tokens)
	for _, m := range pending.TwoFactorChallenge.Methods {
		require.NotEqual(t, "phone", m.Method)
		require.NotEqual(t, "passkey", m.Method)
	}
	var count int64
	s.db.Model(&model.WeChatIdentity{}).Count(&count)
	require.Zero(t, count)
	_, err = s.Factor(ctx, app.UUID, start.TransactionToken, dto.MiniProgramFactorRequest{Method: "phone", Code: "123456"}, false, "", "")
	require.Error(t, err)
	done, err := s.Factor(ctx, app.UUID, start.TransactionToken, dto.MiniProgramFactorRequest{Method: "password", Code: "correct-password"}, false, "", "")
	require.NoError(t, err)
	require.NotNil(t, done.Tokens)
	require.Equal(t, user.UUID, miniClaims(t, done.Tokens.IDToken)["sub"])
}

func TestMiniProgramTransactionGuards(t *testing.T) {
	for _, scenario := range []string{"expired", "cross-client", "disabled-app", "disabled-connection", "policy-changed", "scope-denied", "replayed-code", "unverified-phone", "missing-consent"} {
		t.Run(scenario, func(t *testing.T) {
			s, app, c, user, server := miniFixture(t)
			ctx := context.Background()
			r := miniStart(t, s, app, "guard")
			client := app.UUID
			switch scenario {
			case "expired":
				server.FastForward(16 * time.Minute)
			case "cross-client":
				client = "other-client"
			case "disabled-app":
				require.NoError(t, s.db.Model(app).Update("is_active", false).Error)
			case "disabled-connection":
				require.NoError(t, s.db.Model(c).Update("status", "disabled").Error)
			case "policy-changed":
				app.ProtocolConfig["miniProgram"] = model.MiniProgramConfig{Enabled: true, ConnectionID: c.ID, AccountPolicy: "bind_existing"}
				require.NoError(t, s.db.Save(app).Error)
			case "scope-denied":
				_, err := s.Login(ctx, app.UUID, dto.MiniProgramLoginRequest{JsCode: "scope", Scope: "unknown"}, "issuer")
				require.Error(t, err)
				return
			case "replayed-code":
				_, err := s.Login(ctx, app.UUID, dto.MiniProgramLoginRequest{JsCode: "guard"}, "issuer")
				require.Error(t, err)
				return
			case "unverified-phone":
				phone := "+8613800138000"
				user.Phone = &phone
				require.NoError(t, s.db.Save(user).Error)
				result, err := s.Phone(ctx, client, r.TransactionToken, "phone")
				require.NoError(t, err)
				require.Equal(t, "bind_required", result.State)
				return
			case "missing-consent":
				phone, err := s.Phone(ctx, client, r.TransactionToken, "phone")
				require.NoError(t, err)
				_, err = s.Complete(ctx, client, phone.TransactionToken, dto.MiniProgramCompleteRequest{}, "", "")
				require.Error(t, err)
				return
			}
			_, err := s.Phone(ctx, client, r.TransactionToken, "phone")
			require.Error(t, err)
		})
	}
}

func TestMiniProgramLegacyConflictAndImmutableNamespaces(t *testing.T) {
	s, _, c, user, _ := miniFixture(t)
	otherUser := model.User{Username: "other", Status: model.UserStatusActive, IsActivated: true}
	require.NoError(t, s.db.Create(&otherUser).Error)
	require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", "open-a", "union-common", user.ID))
	other := *c
	other.ID = 0
	other.Identifier = "legacy"
	other.Config = model.JSONObject{"clientId": "wx-b", "clientSecret": "b", "openPlatformId": "platform-1"}
	require.NoError(t, s.db.Create(&other).Error)
	// Existing conflicting rows predate the global index; do not silently pick it.
	principal := model.ExternalPrincipal{ConnectionID: other.ID, UserID: &otherUser.ID, Status: model.PrincipalStatusActive}
	require.NoError(t, s.db.Create(&principal).Error)
	require.NoError(t, s.db.Create(&model.ExternalIdentifier{ConnectionID: other.ID, PrincipalID: principal.ID, Namespace: model.IdentifierNamespaceUnionID, Value: "union-common"}).Error)
	_, err := resolveMiniIdentity(s.db, c, "wx-a", "open-a", "union-common")
	require.ErrorContains(t, err, "不同账号")
	_, err = NewIdentityConnectionService(s.db).Update(c.ID, &dto.UpdateIdentityConnectionRequest{Config: map[string]any{"openPlatformId": "other-platform"}})
	require.ErrorContains(t, err, "不能修改")
	err = NewIdentityConnectionService(s.db).SaveCredentials(c.ID, model.CredentialPurposeAuthentication, map[string]any{"clientId": "other-app"})
	require.ErrorContains(t, err, "不能修改")
}

func TestMiniProgramBindExistingDoesNotRegisterOrAttachEarly(t *testing.T) {
	s, app, c, user, _ := miniFixture(t)
	ctx := context.Background()
	app.ProtocolConfig["miniProgram"] = model.MiniProgramConfig{Enabled: true, ConnectionID: c.ID, AccountPolicy: "bind_existing"}
	require.NoError(t, s.db.Save(app).Error)
	start := miniStart(t, s, app, "bind")
	require.Equal(t, "bind_required", start.State)
	_, err := s.Phone(ctx, app.UUID, start.TransactionToken, "phone")
	require.Error(t, err)
	_, err = s.Bind(ctx, app.UUID, start.TransactionToken, user.Username, "bad", "127.0.0.1")
	require.Error(t, err)
	bound, err := s.Bind(ctx, app.UUID, start.TransactionToken, user.Username, "old-password", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "consent_required", bound.State)
	var count int64
	require.NoError(t, s.db.Model(&model.WeChatIdentity{}).Count(&count).Error)
	require.Zero(t, count)
	done := miniComplete(t, s, app, bound)
	require.False(t, done.IsNewUser)
	require.Equal(t, user.UUID, miniClaims(t, done.Tokens.IDToken)["sub"])
}

func TestMiniProgramDeniedRegistrationRollsBack(t *testing.T) {
	s, app, _, _, _ := miniFixture(t)
	ctx := context.Background()
	start := miniStart(t, s, app, "denied")
	_, err := s.Phone(ctx, app.UUID, start.TransactionToken, "phone")
	require.NoError(t, err)
	app.AccessScope = policyAccess(false)
	require.NoError(t, s.db.Save(app).Error)
	_, err = s.Complete(ctx, app.UUID, start.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true, Profile: dto.MiniProgramProfile{Email: "denied@example.test", RealName: "Denied"}}, "", "")
	require.Error(t, err)
	var count int64
	require.NoError(t, s.db.Model(&model.User{}).Where("phone = ?", "+8613800138000").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, s.db.Model(&model.WeChatIdentity{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, s.db.Model(&model.LoginSession{}).Where("native_transaction_hash IS NOT NULL").Count(&count).Error)
	require.Zero(t, count)
}

func TestMiniProgramConcurrentCompletionOnlyIssuesOnce(t *testing.T) {
	s, app, c, user, _ := miniFixture(t)
	ctx := context.Background()
	require.NoError(t, attachMiniIdentity(s.db, c, "wx-a", fmt.Sprintf("open-%d-race", c.ID), "union-race", user.ID))
	start := miniStart(t, s, app, "race")
	results := make(chan error, 2)
	ready := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-ready
			_, err := s.Complete(ctx, app.UUID, start.TransactionToken, dto.MiniProgramCompleteRequest{ApproveScopes: true}, "", "")
			results <- err
		}()
	}
	close(ready)
	success := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			success++
		}
	}
	require.Equal(t, 1, success)
	var count int64
	require.NoError(t, s.db.Model(&model.LoginSession{}).Where("native_transaction_hash = ?", miniHash(start.TransactionToken)).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestMiniProgramApplicationConfigAllowsNativeOnlyAndPreservesBrowserValidation(t *testing.T) {
	s, _, connection, user, _ := miniFixture(t)
	config := map[string]any{"clientType": "public", "redirectUris": []any{}, "responseTypes": []any{}, "grantTypes": []any{"refresh_token"}, "scopes": []any{"openid"}, "miniProgram": model.MiniProgramConfig{Enabled: true, ConnectionID: connection.ID}, "signingAlgorithm": "HS256"}
	created, err := NewApplicationService(s.db).CreateApplication(&dto.CreateApplicationRequest{Name: "Native-only", ProtocolType: "oidc", ProtocolConfig: config}, user.ID)
	require.NoError(t, err)
	require.NotContains(t, created.ProtocolConfig, "clientSecret")
	config["grantTypes"] = []any{"authorization_code", "refresh_token"}
	require.ErrorContains(t, protocol.NewOIDCProtocolHandler(s.db).ValidateConfig(config), "重定向")
	config["redirectUris"] = []any{"https://client.test/callback"}
	config["responseTypes"] = []any{"code"}
	require.NoError(t, protocol.NewOIDCProtocolHandler(s.db).ValidateConfig(config))
	config["clientType"] = "confidential"
	require.ErrorContains(t, protocol.NewOIDCProtocolHandler(s.db).ValidateConfig(config), "公开客户端")
}
