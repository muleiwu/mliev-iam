package service

import (
	"cnb.cool/mliev/open/go-web/pkg/container"
	"cnb.cool/mliev/open/go-web/pkg/server/env/impl"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"github.com/muleiwu/gsr"
	"math/big"
	"reflect"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func policyAccess(public bool) *model.ApplicationAccessScope {
	return &model.ApplicationAccessScope{IsPublic: &public}
}

func policyFixture(t *testing.T) (*gorm.DB, *model.User, *model.Application, *model.LoginSession, *model.Role) {
	t.Helper()
	previousEnv, _ := container.Get[gsr.Enver]()
	container.Register(container.NewSimpleProvider(reflect.TypeFor[gsr.Enver](), &impl.Env{}))
	t.Cleanup(func() {
		container.Reset(reflect.TypeFor[gsr.Enver]())
		if previousEnv != nil {
			container.Register(container.NewSimpleProvider(reflect.TypeFor[gsr.Enver](), previousEnv))
		}
	})
	db, user, app, session := sessionFixture(t)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.UserRole{}, &model.Department{}, &model.UserDepartment{}))
	role := &model.Role{Name: "Pagnode administrator", Code: "pagnode-admin"}
	require.NoError(t, db.Create(role).Error)
	require.NoError(t, db.Create(&model.UserRole{UserID: user.ID, RoleID: role.ID, IsActive: true, ApprovalStatus: 1, ApplyScope: 1}).Error)
	access := policyAccess(false)
	access.RoleIDs = []uint{role.ID}
	app.ProtocolConfig["scopes"] = []interface{}{"openid", "profile", "pagnode:admin"}
	app.ProtocolConfig["scopeAuthorization"] = model.OIDCScopeAuthorization{Enabled: true, Rules: map[string]model.OIDCScopeRule{
		"openid":        {Description: "验证身份", UserAccess: policyAccess(true)},
		"profile":       {UserAccess: policyAccess(true)},
		"pagnode:admin": {Description: "管理 Pagnode", UserAccess: access},
	}}
	app.ProtocolConfig["roleClaims"] = model.OIDCRoleClaimsConfig{Enabled: true, ClaimName: "roles", RoleIDs: []uint{role.ID}, Targets: []string{"access_token", "id_token", "userinfo"}}
	require.NoError(t, db.Save(app).Error)
	return db, user, app, session, role
}

func TestOIDCPolicySubjectRules(t *testing.T) {
	for _, scenario := range []string{"role", "person", "department", "subdepartment", "unrelated", "expired", "pending", "disabled-role", "disabled-assignment", "department-role", "deleted-role", "application-denied", "missing-rule"} {
		t.Run(scenario, func(t *testing.T) {
			db, user, app, _, role := policyFixture(t)
			config, err := model.DecodeOIDCAuthorizationConfig(app.ProtocolConfig)
			require.NoError(t, err)
			rule := config.ScopeAuthorization.Rules["pagnode:admin"]
			allowed := scenario == "role" || scenario == "person" || scenario == "department" || scenario == "subdepartment"
			switch scenario {
			case "person":
				rule.UserAccess.RoleIDs = nil
				rule.UserAccess.UserIDs = []uint{user.ID}
			case "department", "subdepartment":
				parent := &model.Department{Name: "Parent", Code: "parent", Status: model.DepartmentStatusEnabled}
				require.NoError(t, db.Create(parent).Error)
				require.NoError(t, db.Model(parent).Update("path", "/1/").Error)
				membership := parent.ID
				if scenario == "subdepartment" {
					child := &model.Department{Name: "Child", Code: "child", ParentID: &parent.ID, Status: model.DepartmentStatusEnabled}
					require.NoError(t, db.Create(child).Error)
					require.NoError(t, db.Model(child).Update("path", "/1/2/").Error)
					membership = child.ID
				}
				require.NoError(t, db.Create(&model.UserDepartment{UserID: user.ID, DepartmentID: membership, IsActive: true}).Error)
				rule.UserAccess.RoleIDs = nil
				rule.UserAccess.DepartmentIDs = []uint{parent.ID}
				rule.UserAccess.IncludeSubDepartments = scenario == "subdepartment"
			case "unrelated":
				rule.UserAccess.RoleIDs = nil
			case "expired":
				require.NoError(t, db.Model(&model.UserRole{}).Where("user_id = ?", user.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
			case "pending":
				require.NoError(t, db.Model(&model.UserRole{}).Where("user_id = ?", user.ID).Update("approval_status", 0).Error)
			case "disabled-role":
				require.NoError(t, db.Model(role).Update("status", model.RoleStatusDisabled).Error)
			case "disabled-assignment":
				require.NoError(t, db.Model(&model.UserRole{}).Where("user_id = ?", user.ID).Update("is_active", false).Error)
			case "department-role":
				require.NoError(t, db.Model(&model.UserRole{}).Where("user_id = ?", user.ID).Update("apply_scope", 2).Error)
			case "deleted-role":
				require.NoError(t, db.Delete(role).Error)
			case "application-denied":
				app.AccessScope = policyAccess(false)
			}
			config.ScopeAuthorization.Rules["pagnode:admin"] = rule
			if scenario == "missing-rule" {
				delete(config.ScopeAuthorization.Rules, "pagnode:admin")
			}
			app.ProtocolConfig["scopeAuthorization"] = config.ScopeAuthorization
			require.NoError(t, db.Save(app).Error)
			grant, err := NewOIDCAuthorizationService(db).evaluateOIDCGrant(app, user.ID, []string{"pagnode:admin"}, false)
			if allowed {
				require.NoError(t, err)
				require.Equal(t, []string{"pagnode:admin"}, grant.Scopes)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestOIDCPolicySnapshotsAndRefresh(t *testing.T) {
	db, user, app, session, role := policyFixture(t)
	svc := NewOIDCAuthorizationService(db)
	first, err := svc.GenerateTokens(app, user.ID, []string{"openid", "pagnode:admin", "pagnode:admin"}, "", "https://issuer.example", session.ID)
	require.NoError(t, err)
	require.Equal(t, "openid pagnode:admin", first.Scope)
	parsed, _, err := jwt.NewParser().ParseUnverified(first.AccessToken, jwt.MapClaims{})
	require.NoError(t, err)
	require.Equal(t, "at+jwt", parsed.Header["typ"])
	require.Equal(t, []interface{}{"pagnode-admin"}, parsed.Claims.(jwt.MapClaims)["roles"])
	id, _, err := jwt.NewParser().ParseUnverified(first.IDToken, jwt.MapClaims{})
	require.NoError(t, err)
	require.Equal(t, "JWT", id.Header["typ"])
	require.Equal(t, parsed.Claims.(jwt.MapClaims)["roles"], id.Claims.(jwt.MapClaims)["roles"])
	old, err := dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(first.RefreshToken)
	require.NoError(t, err)
	require.NoError(t, db.Model(role).Update("status", model.RoleStatusDisabled).Error)
	info, err := NewOIDCUserInfoService(db).GetUserInfo(first.AccessToken)
	require.NoError(t, err)
	require.Equal(t, []string{"pagnode-admin"}, *info.Roles)
	_, err = svc.RotateLoginToken(old, app, "", "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidGrant)
	_, err = dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(first.RefreshToken)
	require.NoError(t, err, "failed refresh must roll back consumption")
	_, err = svc.RotateLoginToken(old, app, "openid profile", "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidScope, "refresh must not add scopes")
	narrowed, err := svc.RotateLoginToken(old, app, "openid", "https://issuer.example")
	require.NoError(t, err)
	info, err = NewOIDCUserInfoService(db).GetUserInfo(narrowed.AccessToken)
	require.NoError(t, err)
	require.NotNil(t, info.Roles)
	require.Empty(t, *info.Roles)
	_, err = svc.RotateLoginToken(old, app, "openid", "https://issuer.example")
	require.Error(t, err)
	// Re-enabling a role must not add it to an already issued UserInfo snapshot.
	require.NoError(t, db.Model(role).Update("status", model.RoleStatusEnabled).Error)
	info, err = NewOIDCUserInfoService(db).GetUserInfo(narrowed.AccessToken)
	require.NoError(t, err)
	require.Empty(t, *info.Roles)
}

func TestOIDCPolicyMachineAndJWKS(t *testing.T) {
	db, _, app, _, role := policyFixture(t)
	handler := protocol.NewOIDCProtocolHandler(db)
	app.ProtocolConfig["clientType"] = "confidential"
	app.ProtocolConfig["grantTypes"] = []interface{}{"client_credentials", "authorization_code"}
	app.ProtocolConfig["clientSecret"] = "test-machine-secret"
	require.NoError(t, handler.EncryptSensitiveFields(app.ProtocolConfig))
	config, err := model.DecodeOIDCAuthorizationConfig(app.ProtocolConfig)
	require.NoError(t, err)
	rule := config.ScopeAuthorization.Rules["pagnode:admin"]
	rule.AllowClientCredentials = true
	config.ScopeAuthorization.Rules["pagnode:admin"] = rule
	app.ProtocolConfig["scopeAuthorization"] = config.ScopeAuthorization
	require.NoError(t, db.Save(app).Error)
	keys := NewApplicationKeyService(db)
	key, err := keys.GenerateKeyForApplication(app.ID, "RS256", 2048)
	require.NoError(t, err)
	manager, err := keys.LoadKeyManager(key)
	require.NoError(t, err)
	jwk, err := manager.GetJWK()
	require.NoError(t, err)
	n, err := base64.RawURLEncoding.DecodeString(jwk["n"].(string))
	require.NoError(t, err)
	e, err := base64.RawURLEncoding.DecodeString(jwk["e"].(string))
	require.NoError(t, err)
	public := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	svc := NewOIDCAuthorizationService(db)
	request := &dto.TokenRequest{GrantType: "client_credentials", ClientID: app.UUID, ClientSecret: "test-machine-secret", Scope: "pagnode:admin"}
	issue := func() *dto.TokenResponse {
		result, err := svc.ExchangeClientCredentials(request, "https://issuer.example")
		require.NoError(t, err)
		return result
	}
	result := issue()
	token, err := jwt.Parse(result.AccessToken, func(*jwt.Token) (interface{}, error) { return public, nil }, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer("https://issuer.example"), jwt.WithAudience(app.UUID))
	require.NoError(t, err)
	require.Equal(t, "at+jwt", token.Header["typ"])
	require.Equal(t, key.KeyID, token.Header["kid"])
	require.Empty(t, token.Claims.(jwt.MapClaims)["roles"], "creator roles must not be inherited")
	app.ProtocolConfig["clientRoleIds"] = []uint{role.ID}
	require.NoError(t, db.Save(app).Error)
	result = issue()
	token, err = jwt.Parse(result.AccessToken, func(*jwt.Token) (interface{}, error) { return public, nil }, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer("https://issuer.example"), jwt.WithAudience(app.UUID))
	require.NoError(t, err)
	require.Equal(t, []interface{}{"pagnode-admin"}, token.Claims.(jwt.MapClaims)["roles"])
	require.Equal(t, "pagnode:admin", token.Claims.(jwt.MapClaims)["scope"])
	require.Empty(t, result.RefreshToken)
	require.Empty(t, result.IDToken)
	request.Scope = ""
	_, err = svc.ExchangeClientCredentials(request, "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidScope)
	request.Scope = "profile"
	_, err = svc.ExchangeClientCredentials(request, "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidScope)
	request.Scope = "pagnode:admin"
	app.ProtocolConfig["clientType"] = "public"
	require.NoError(t, db.Save(app).Error)
	_, err = svc.ExchangeClientCredentials(request, "https://issuer.example")
	require.Error(t, err)
	stored, err := dao.NewOIDCTokenDAO(db).GetByAccessToken(result.AccessToken)
	require.NoError(t, err)
	stored.Scopes = []string{"openid"}
	require.NoError(t, db.Save(stored).Error)
	_, err = NewOIDCUserInfoService(db).GetUserInfo(result.AccessToken)
	require.EqualError(t, err, "invalid_token")
}

func TestOIDCPolicyConfigAndIndependentOutputs(t *testing.T) {
	db, user, app, session, role := policyFixture(t)
	// Round-trip through JSON exactly as the management API does.
	encoded, err := json.Marshal(app.ProtocolConfig)
	require.NoError(t, err)
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &raw))
	handler := protocol.NewOIDCProtocolHandler(db)
	require.NoError(t, handler.ValidateConfig(raw))
	raw["clientRoleIds"] = []uint{999999}
	require.Error(t, handler.ValidateConfig(raw))
	delete(raw, "clientRoleIds")
	config, err := model.DecodeOIDCAuthorizationConfig(raw)
	require.NoError(t, err)
	delete(config.ScopeAuthorization.Rules, "profile")
	raw["scopeAuthorization"] = config.ScopeAuthorization
	require.Error(t, handler.ValidateConfig(raw))
	// An older management client must preserve policies it doesn't submit.
	_, err = NewApplicationService(db).UpdateApplication(app.ID, &dto.UpdateApplicationRequest{ProtocolConfig: &map[string]interface{}{"tokenLifetime": 300}})
	require.NoError(t, err)
	refreshed, err := NewApplicationService(db).GetApplication(app.ID)
	require.NoError(t, err)
	require.Contains(t, refreshed.ProtocolConfig, "scopeAuthorization")
	for _, target := range []string{"access_token", "id_token", "userinfo"} {
		app.ProtocolConfig["roleClaims"] = model.OIDCRoleClaimsConfig{Enabled: true, ClaimName: "groups", RoleIDs: []uint{role.ID}, Targets: []string{target}}
		require.NoError(t, db.Save(app).Error)
		response, err := NewOIDCAuthorizationService(db).GenerateTokens(app, user.ID, []string{"openid"}, "", "https://issuer.example", session.ID)
		require.NoError(t, err)
		for location, rawToken := range map[string]string{"access_token": response.AccessToken, "id_token": response.IDToken} {
			parsed, _, err := jwt.NewParser().ParseUnverified(rawToken, jwt.MapClaims{})
			require.NoError(t, err)
			_, present := parsed.Claims.(jwt.MapClaims)["groups"]
			require.Equal(t, target == location, present)
			require.NotContains(t, parsed.Claims.(jwt.MapClaims), "roles")
		}
		info, err := NewOIDCUserInfoService(db).GetUserInfo(response.AccessToken)
		require.NoError(t, err)
		require.Equal(t, target == "userinfo", info.Groups != nil)
		discovery := NewOIDCDiscoveryService("https://issuer.example").GetDiscoveryDocument(app)
		require.Contains(t, discovery.ScopesSupported, "pagnode:admin")
		require.Equal(t, target != "access_token", containsPolicyString(discovery.ClaimsSupported, "groups"))
	}
}

func containsPolicyString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestOIDCPolicyAuthorizationAndRecheckAtIssuance(t *testing.T) {
	db, user, app, session, role := policyFixture(t)
	app.ProtocolConfig["clientType"] = "confidential"
	app.ProtocolConfig["requirePkce"] = false
	app.ProtocolConfig["clientSecret"] = "policy-secret"
	require.NoError(t, protocol.NewOIDCProtocolHandler(db).EncryptSensitiveFields(app.ProtocolConfig))
	app.ProtocolConfig["responseTypes"] = []interface{}{"code", "token", "id_token"}
	require.NoError(t, db.Save(app).Error)
	svc := NewOIDCAuthorizationService(db)
	req := &dto.AuthorizeRequest{ClientID: app.UUID, RedirectURI: "https://rp.example/callback", ResponseType: "code", Scope: "openid pagnode:admin", LoginSessionID: session.ID}
	auth, err := svc.ProcessAuthorize(req, user.ID, "https://issuer.example")
	require.NoError(t, err)
	require.Empty(t, auth.Error)
	require.NotEmpty(t, auth.Code)
	require.NoError(t, db.Model(role).Update("status", model.RoleStatusDisabled).Error)
	_, err = svc.ExchangeCodeForToken(&dto.TokenRequest{GrantType: "authorization_code", ClientID: app.UUID, ClientSecret: "policy-secret", RedirectURI: req.RedirectURI, Code: auth.Code}, "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidGrant)
	for _, responseType := range []string{"code", "token", "id_token"} {
		req.ResponseType = responseType
		reply, err := svc.ProcessAuthorize(req, user.ID, "https://issuer.example")
		require.NoError(t, err)
		require.Equal(t, "access_denied", reply.Error)
	}
	_, err = svc.GenerateTokens(app, user.ID, []string{"pagnode:admin"}, "", "https://issuer.example", session.ID)
	require.ErrorIs(t, err, constants.ErrAccessDenied)
	_, err = svc.GenerateSSOTokens(app, user.ID, []string{"pagnode:admin"}, "", "https://issuer.example", session.ID, "")
	require.ErrorIs(t, err, constants.ErrAccessDenied)
	req.Scope = "openid  pagnode:admin"
	reply, err := svc.ProcessAuthorize(req, user.ID, "https://issuer.example")
	require.NoError(t, err)
	require.Equal(t, "invalid_scope", reply.Error)
}

func TestOIDCPolicyPersonGrantDoesNotManufactureRoles(t *testing.T) {
	db, user, app, _, _ := policyFixture(t)
	require.NoError(t, db.Where("user_id = ?", user.ID).Delete(&model.UserRole{}).Error)
	config, err := model.DecodeOIDCAuthorizationConfig(app.ProtocolConfig)
	require.NoError(t, err)
	rule := config.ScopeAuthorization.Rules["pagnode:admin"]
	rule.UserAccess.UserIDs = []uint{user.ID}
	config.ScopeAuthorization.Rules["pagnode:admin"] = rule
	app.ProtocolConfig["scopeAuthorization"] = config.ScopeAuthorization
	require.NoError(t, db.Save(app).Error)
	grant, err := NewOIDCAuthorizationService(db).evaluateOIDCGrant(app, user.ID, []string{"pagnode:admin"}, false)
	require.NoError(t, err)
	require.NotNil(t, grant.Roles)
	require.Empty(t, grant.Roles.Values)
}

func TestOIDCPolicyScopeConfigurationRejectsMalformedValues(t *testing.T) {
	db, _, app, _, _ := policyFixture(t)
	handler := protocol.NewOIDCProtocolHandler(db)
	for _, scope := range []string{"", "admin write", "a\tb", `a"b`, `a\b`, "管理"} {
		encoded, err := json.Marshal(app.ProtocolConfig)
		require.NoError(t, err)
		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &raw))
		raw["scopes"] = []interface{}{"openid", scope}
		require.Error(t, handler.ValidateConfig(raw), "invalid scope %q", scope)
	}
	for _, scopes := range []string{"pagnode:admin", "read:files", "openid profile", "openid openid"} {
		_, err := model.ParseOIDCScopes(scopes)
		require.NoError(t, err)
	}
}

func TestOIDCPolicyRoleWhitelistAndLegacyDefaults(t *testing.T) {
	db, user, app, _, role := policyFixture(t)
	other := &model.Role{Name: "Other application", Code: "other-admin"}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Create(&model.UserRole{UserID: user.ID, RoleID: other.ID, IsActive: true, ApprovalStatus: 1, ApplyScope: 1}).Error)
	svc := NewOIDCAuthorizationService(db)
	grant, err := svc.evaluateOIDCGrant(app, user.ID, []string{"pagnode:admin"}, false)
	require.NoError(t, err)
	require.Equal(t, []string{role.Code}, grant.Roles.Values)
	app.ProtocolConfig["roleClaims"] = model.OIDCRoleClaimsConfig{Enabled: true, ClaimName: "roles", RoleIDs: []uint{}, Targets: []string{"access_token"}}
	require.NoError(t, db.Save(app).Error)
	grant, err = svc.evaluateOIDCGrant(app, user.ID, []string{"pagnode:admin"}, false)
	require.NoError(t, err)
	require.Empty(t, grant.Roles.Values)
	delete(app.ProtocolConfig, "roleClaims")
	delete(app.ProtocolConfig, "scopeAuthorization")
	require.NoError(t, db.Save(app).Error)
	require.NoError(t, db.Where("user_id = ?", user.ID).Delete(&model.UserRole{}).Error)
	grant, err = svc.evaluateOIDCGrant(app, user.ID, []string{"pagnode:admin"}, false)
	require.NoError(t, err)
	require.Nil(t, grant.Roles, "legacy applications retain their allowlist behavior without exposing roles")
}
