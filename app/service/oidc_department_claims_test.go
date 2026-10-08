package service

import (
	"encoding/json"
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
	"gorm.io/gorm/logger"
)

type departmentClaimFixture struct {
	db                              *gorm.DB
	user                            *model.User
	app                             *model.Application
	session                         *model.LoginSession
	company, platform, sre, ops, hr *model.Department
}

func newDepartmentClaimFixture(t *testing.T) *departmentClaimFixture {
	t.Helper()
	db, user, app, session, _ := policyFixture(t)
	db.Logger = logger.Default.LogMode(logger.Silent)
	create := func(code string, parent *model.Department) *model.Department {
		department := &model.Department{Name: code, Code: code}
		if parent != nil {
			department.ParentID = &parent.ID
		}
		require.NoError(t, db.Create(department).Error)
		return department
	}
	f := &departmentClaimFixture{db: db, user: user, app: app, session: session}
	f.company = create("company", nil)
	f.platform = create("platform", f.company)
	f.sre = create("sre", f.platform)
	f.ops = create("ops", f.platform)
	f.hr = create("hr", nil)
	for _, department := range []*model.Department{f.sre, f.ops} {
		require.NoError(t, db.Create(&model.UserDepartment{UserID: user.ID, DepartmentID: department.ID, IsActive: true}).Error)
	}
	// A manager relationship is not a department membership; Path is not authoritative.
	require.NoError(t, db.Model(f.hr).Update("manager_id", user.ID).Error)
	require.NoError(t, db.Model(f.sre).Update("path", "/incorrect-denormalized-path").Error)
	f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: true, DepartmentIDs: f.allIDs(), Targets: []string{"access_token", "id_token", "userinfo"}})
	return f
}
func (f *departmentClaimFixture) allIDs() []uint {
	return []uint{f.company.ID, f.platform.ID, f.sre.ID, f.ops.ID, f.hr.ID}
}
func (f *departmentClaimFixture) setConfig(t *testing.T, config model.OIDCDepartmentClaimsConfig) {
	t.Helper()
	f.app.ProtocolConfig["departmentClaims"] = config
	require.NoError(t, f.db.Save(f.app).Error)
}
func (f *departmentClaimFixture) issue(t *testing.T) *dto.TokenResponse {
	t.Helper()
	tokens, err := NewOIDCAuthorizationService(f.db).GenerateTokens(f.app, f.user.ID, []string{"openid", "profile"}, "", "https://issuer.example", f.session.ID)
	require.NoError(t, err)
	return tokens
}
func departmentTokenClaims(t *testing.T, raw string) jwt.MapClaims {
	t.Helper()
	token, _, err := jwt.NewParser().ParseUnverified(raw, jwt.MapClaims{})
	require.NoError(t, err)
	return token.Claims.(jwt.MapClaims)
}

func TestOIDCDepartmentMembershipsAncestorsAndWhitelist(t *testing.T) {
	for _, scenario := range []string{"all", "ancestor-only", "direct-only", "empty", "disabled-membership", "left", "deleted-membership", "disabled-direct", "frozen-parent", "disabled-parent", "deleted-parent", "missing-parent"} {
		t.Run(scenario, func(t *testing.T) {
			f := newDepartmentClaimFixture(t)
			expected := []string{"company", "ops", "platform", "sre"}
			membership := f.db.Model(&model.UserDepartment{}).Where("user_id = ? AND department_id = ?", f.user.ID, f.sre.ID)
			switch scenario {
			case "ancestor-only":
				f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: true, DepartmentIDs: []uint{f.platform.ID}})
				expected = []string{"platform"}
			case "direct-only":
				f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: true, DepartmentIDs: []uint{f.sre.ID}})
				expected = []string{"sre"}
			case "empty":
				f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: true})
				expected = []string{}
			case "disabled-membership":
				require.NoError(t, membership.Update("is_active", false).Error)
				expected = []string{"company", "ops", "platform"}
			case "left":
				require.NoError(t, membership.Update("left_at", time.Now()).Error)
				expected = []string{"company", "ops", "platform"}
			case "deleted-membership":
				require.NoError(t, membership.Update("deleted_at", time.Now()).Error)
				expected = []string{"company", "ops", "platform"}
			case "disabled-direct":
				require.NoError(t, f.db.Model(f.sre).Update("status", model.DepartmentStatusDisabled).Error)
				expected = []string{"company", "ops", "platform"}
			case "frozen-parent":
				require.NoError(t, f.db.Model(f.platform).Update("status", model.DepartmentStatusFrozen).Error)
				expected = []string{"ops", "sre"}
			case "disabled-parent":
				require.NoError(t, f.db.Model(f.platform).Update("status", model.DepartmentStatusDisabled).Error)
				expected = []string{"ops", "sre"}
			case "deleted-parent":
				require.NoError(t, f.db.Model(f.platform).Update("deleted_at", time.Now()).Error)
				expected = []string{"ops", "sre"}
			case "missing-parent":
				require.NoError(t, f.db.Model(f.platform).Update("parent_id", 99999).Error)
				expected = []string{"ops", "platform", "sre"}
			}
			grant, err := NewOIDCAuthorizationService(f.db).evaluateOIDCGrant(f.app, f.user.ID, []string{"openid"}, false)
			require.NoError(t, err)
			require.NotNil(t, grant.Departments)
			require.Equal(t, expected, grant.Departments.Values)
			require.NotContains(t, grant.Departments.Values, "hr", "being a manager must not manufacture membership")
		})
	}
}

func TestOIDCDepartmentOutputDestinationsAndLegacyTokens(t *testing.T) {
	f := newDepartmentClaimFixture(t)
	for _, target := range []string{"access_token", "id_token", "userinfo"} {
		f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: true, DepartmentIDs: []uint{f.platform.ID}, Targets: []string{target}})
		issued := f.issue(t)
		for destination, raw := range map[string]string{"access_token": issued.AccessToken, "id_token": issued.IDToken} {
			claims := departmentTokenClaims(t, raw)
			_, present := claims["departments"]
			require.Equal(t, target == destination, present)
			if present {
				require.Equal(t, []interface{}{"platform"}, claims["departments"])
			}
			require.Equal(t, []interface{}{"pagnode-admin"}, claims["roles"], "department output must not overwrite role claims")
		}
		info, err := NewOIDCUserInfoService(f.db).GetUserInfo(issued.AccessToken)
		require.NoError(t, err)
		require.Equal(t, target == "userinfo", info.Departments != nil)
		if info.Departments != nil {
			require.Equal(t, []string{"platform"}, *info.Departments)
		}
		discovery := NewOIDCDiscoveryService("https://issuer.example").GetDiscoveryDocument(f.app)
		require.Equal(t, target != "access_token", containsPolicyString(discovery.ClaimsSupported, "departments"))
	}
	// Empty values serialize as [], while disabled/legacy snapshots omit the field.
	f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: true, Targets: []string{"access_token", "id_token", "userinfo"}})
	empty := f.issue(t)
	require.Equal(t, []interface{}{}, departmentTokenClaims(t, empty.AccessToken)["departments"])
	info, err := NewOIDCUserInfoService(f.db).GetUserInfo(empty.AccessToken)
	require.NoError(t, err)
	require.NotNil(t, info.Departments)
	require.Empty(t, *info.Departments)
	delete(f.app.ProtocolConfig, "departmentClaims")
	require.NoError(t, f.db.Save(f.app).Error)
	legacy := f.issue(t)
	require.NotContains(t, departmentTokenClaims(t, legacy.AccessToken), "departments")
	f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: true, DepartmentIDs: f.allIDs(), Targets: []string{"userinfo"}})
	info, err = NewOIDCUserInfoService(f.db).GetUserInfo(legacy.AccessToken)
	require.NoError(t, err)
	require.Nil(t, info.Departments, "old tokens must not acquire new claims from current configuration")
	f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: false, DepartmentIDs: f.allIDs(), Targets: []string{"access_token", "id_token", "userinfo"}})
	disabled := f.issue(t)
	require.NotContains(t, departmentTokenClaims(t, disabled.AccessToken), "departments")
	require.NotContains(t, NewOIDCDiscoveryService("https://issuer.example").GetDiscoveryDocument(f.app).ClaimsSupported, "departments")
}

func TestOIDCDepartmentSnapshotSurvivesChangesAndRefreshRecomputes(t *testing.T) {
	for _, change := range []string{"rename", "move", "remove", "add"} {
		t.Run(change, func(t *testing.T) {
			f := newDepartmentClaimFixture(t)
			issued := f.issue(t)
			expected := []string{"company", "ops", "platform", "sre"}
			switch change {
			case "rename":
				require.NoError(t, f.db.Model(f.sre).Update("code", "z-sre").Error)
				expected = []string{"company", "ops", "platform", "z-sre"}
			case "move":
				require.NoError(t, f.db.Model(f.platform).Update("parent_id", f.hr.ID).Error)
				expected = []string{"hr", "ops", "platform", "sre"}
			case "remove":
				require.NoError(t, f.db.Where("user_id = ? AND department_id = ?", f.user.ID, f.sre.ID).Delete(&model.UserDepartment{}).Error)
				expected = []string{"company", "ops", "platform"}
			case "add":
				require.NoError(t, f.db.Create(&model.UserDepartment{UserID: f.user.ID, DepartmentID: f.hr.ID, IsActive: true}).Error)
				expected = []string{"company", "hr", "ops", "platform", "sre"}
			}
			info, err := NewOIDCUserInfoService(f.db).GetUserInfo(issued.AccessToken)
			require.NoError(t, err)
			require.Equal(t, []string{"company", "ops", "platform", "sre"}, *info.Departments)
			old, err := dao.NewOIDCTokenDAO(f.db).GetValidByRefreshToken(issued.RefreshToken)
			require.NoError(t, err)
			refreshed, err := NewOIDCAuthorizationService(f.db).RotateLoginToken(old, f.app, "", "https://issuer.example")
			require.NoError(t, err)
			info, err = NewOIDCUserInfoService(f.db).GetUserInfo(refreshed.AccessToken)
			require.NoError(t, err)
			require.Equal(t, expected, *info.Departments)
			require.Equal(t, []string{"pagnode-admin"}, *info.Roles)
		})
	}
}

func TestOIDCDepartmentCycleAndQueryFailureRollBackRefresh(t *testing.T) {
	for _, failure := range []string{"cycle", "membership-query", "ancestor-query"} {
		t.Run(failure, func(t *testing.T) {
			f := newDepartmentClaimFixture(t)
			issued := f.issue(t)
			old, err := dao.NewOIDCTokenDAO(f.db).GetValidByRefreshToken(issued.RefreshToken)
			require.NoError(t, err)
			switch failure {
			case "cycle":
				require.NoError(t, f.db.Model(f.platform).Update("parent_id", f.sre.ID).Error)
			case "membership-query":
				require.NoError(t, f.db.Migrator().DropTable(&model.UserDepartment{}))
			case "ancestor-query":
				require.NoError(t, f.db.Migrator().DropTable(&model.Department{}))
			}
			svc := NewOIDCAuthorizationService(f.db)
			result, err := svc.GenerateTokens(f.app, f.user.ID, []string{"openid"}, "", "https://issuer.example", f.session.ID)
			require.Nil(t, result)
			require.ErrorIs(t, err, constants.ErrServerError)
			result, err = svc.RotateLoginToken(old, f.app, "", "https://issuer.example")
			require.Nil(t, result)
			require.ErrorIs(t, err, constants.ErrServerError)
			_, err = dao.NewOIDCTokenDAO(f.db).GetValidByRefreshToken(issued.RefreshToken)
			require.NoError(t, err)
			info, err := NewOIDCUserInfoService(f.db).GetUserInfo(issued.AccessToken)
			require.NoError(t, err)
			require.Equal(t, []string{"company", "ops", "platform", "sre"}, *info.Departments)
			var count int64
			require.NoError(t, f.db.Model(&model.OIDCToken{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}

func TestOIDCDepartmentMachineTokensNeverReadUserDepartments(t *testing.T) {
	f := newDepartmentClaimFixture(t)
	f.app.ProtocolConfig["clientType"] = "confidential"
	f.app.ProtocolConfig["clientSecret"] = "machine-department-test"
	f.app.ProtocolConfig["grantTypes"] = []interface{}{"client_credentials"}
	handler := protocol.NewOIDCProtocolHandler(f.db)
	require.NoError(t, handler.EncryptSensitiveFields(f.app.ProtocolConfig))
	config, err := model.DecodeOIDCAuthorizationConfig(f.app.ProtocolConfig)
	require.NoError(t, err)
	rule := config.ScopeAuthorization.Rules["profile"]
	rule.AllowClientCredentials = true
	config.ScopeAuthorization.Rules["profile"] = rule
	f.app.ProtocolConfig["scopeAuthorization"] = config.ScopeAuthorization
	require.NoError(t, f.db.Save(f.app).Error)
	require.NoError(t, f.db.Migrator().DropTable(&model.UserDepartment{}))
	issued, err := NewOIDCAuthorizationService(f.db).ExchangeClientCredentials(&dto.TokenRequest{GrantType: "client_credentials", ClientID: f.app.UUID, ClientSecret: "machine-department-test", Scope: "profile"}, "https://issuer.example")
	require.NoError(t, err)
	require.NotContains(t, departmentTokenClaims(t, issued.AccessToken), "departments")
	stored, err := dao.NewOIDCTokenDAO(f.db).GetByAccessToken(issued.AccessToken)
	require.NoError(t, err)
	require.NotContains(t, stored.Attributes, model.OIDCDepartmentSnapshotAttribute)
}

func TestOIDCDepartmentConfigurationValidationAndCompatibility(t *testing.T) {
	f := newDepartmentClaimFixture(t)
	for _, config := range []model.OIDCDepartmentClaimsConfig{
		{Enabled: true, DepartmentIDs: []uint{0}},
		{Enabled: true, DepartmentIDs: []uint{99999}},
		{Enabled: true, DepartmentIDs: []uint{f.sre.ID}, Targets: []string{}},
		{Enabled: true, DepartmentIDs: []uint{f.sre.ID}, Targets: []string{"unknown"}},
	} {
		f.app.ProtocolConfig["departmentClaims"] = config
		require.Error(t, protocol.NewOIDCProtocolHandler(f.db).ValidateConfig(f.app.ProtocolConfig))
	}
	f.setConfig(t, model.OIDCDepartmentClaimsConfig{Enabled: true, DepartmentIDs: []uint{f.sre.ID}})
	require.NoError(t, protocol.NewOIDCProtocolHandler(f.db).ValidateConfig(f.app.ProtocolConfig))
	issued := f.issue(t)
	require.Equal(t, []interface{}{"sre"}, departmentTokenClaims(t, issued.AccessToken)["departments"])
	require.NotContains(t, departmentTokenClaims(t, issued.IDToken), "departments")
	response, err := NewApplicationService(f.db).UpdateApplication(f.app.ID, &dto.UpdateApplicationRequest{ProtocolConfig: &map[string]interface{}{"tokenLifetime": 300}})
	require.NoError(t, err)
	config, err := model.DecodeOIDCAuthorizationConfig(response.ProtocolConfig)
	require.NoError(t, err)
	require.True(t, config.DepartmentClaims.Enabled)
	require.Equal(t, []uint{f.sre.ID}, config.DepartmentClaims.DepartmentIDs)
	// Another application with a distinct output whitelist cannot receive these claims.
	other := &model.Application{UUID: "another-department-app", Name: "Other", ProtocolType: "oidc", IsActive: true, ProtocolConfig: map[string]interface{}{"scopes": []string{"openid"}, "departmentClaims": model.OIDCDepartmentClaimsConfig{Enabled: true, DepartmentIDs: []uint{f.hr.ID}}}}
	require.NoError(t, f.db.Create(other).Error)
	grant, err := NewOIDCAuthorizationService(f.db).evaluateOIDCGrant(other, f.user.ID, []string{"openid"}, false)
	require.NoError(t, err)
	require.Empty(t, grant.Departments.Values)
	encoded, err := json.Marshal(grant.Departments.For("access_token"))
	require.NoError(t, err)
	require.JSONEq(t, "[]", string(encoded))
}

func TestOIDCDepartmentClaimsAcrossAuthorizationAndSSOFlows(t *testing.T) {
	f := newDepartmentClaimFixture(t)
	f.app.ProtocolConfig["clientType"] = "confidential"
	f.app.ProtocolConfig["clientSecret"] = "department-flow-secret"
	f.app.ProtocolConfig["requirePkce"] = false
	f.app.ProtocolConfig["responseTypes"] = []interface{}{"code", "token", "id_token"}
	require.NoError(t, protocol.NewOIDCProtocolHandler(f.db).EncryptSensitiveFields(f.app.ProtocolConfig))
	require.NoError(t, f.db.Save(f.app).Error)
	svc := NewOIDCAuthorizationService(f.db)
	expected := []interface{}{"company", "ops", "platform", "sre"}
	for _, responseType := range []string{"code", "token", "id_token"} {
		request := &dto.AuthorizeRequest{ClientID: f.app.UUID, RedirectURI: "https://rp.example/callback", ResponseType: responseType, Scope: "openid profile", LoginSessionID: f.session.ID}
		response, err := svc.ProcessAuthorize(request, f.user.ID, "https://issuer.example")
		require.NoError(t, err)
		require.Empty(t, response.Error)
		raw := response.Code
		if responseType == "code" {
			tokens, err := svc.ExchangeCodeForToken(&dto.TokenRequest{GrantType: "authorization_code", ClientID: f.app.UUID, ClientSecret: "department-flow-secret", RedirectURI: request.RedirectURI, Code: response.Code}, "https://issuer.example")
			require.NoError(t, err)
			raw = tokens.AccessToken
			require.Equal(t, expected, departmentTokenClaims(t, tokens.IDToken)["departments"])
		}
		require.Equal(t, expected, departmentTokenClaims(t, raw)["departments"], responseType)
	}
	tokens, err := svc.GenerateSSOTokens(f.app, f.user.ID, []string{"openid", "profile"}, "", "https://issuer.example", f.session.ID, "")
	require.NoError(t, err)
	require.Equal(t, expected, departmentTokenClaims(t, tokens.AccessToken)["departments"])
	info, err := NewOIDCUserInfoService(f.db).GetUserInfo(tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, []string{"company", "ops", "platform", "sre"}, *info.Departments)
	// Department claims are independent even when role output is switched off.
	f.app.ProtocolConfig["roleClaims"] = model.OIDCRoleClaimsConfig{Enabled: false}
	require.NoError(t, f.db.Save(f.app).Error)
	tokens = f.issue(t)
	claims := departmentTokenClaims(t, tokens.AccessToken)
	require.Equal(t, expected, claims["departments"])
	require.NotContains(t, claims, "roles")
	require.NotContains(t, claims, "groups")
}
