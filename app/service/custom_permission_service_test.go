package service

import (
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func customFixture(t *testing.T) (*gorm.DB, *model.User, *model.Application, *model.LoginSession, *model.CustomPermission) {
	t.Helper()
	db, user, app, session, _ := policyFixture(t)
	var permission model.CustomPermission
	require.NoError(t, db.First(&permission).Error)
	require.NoError(t, db.Where("permission_id = ?", permission.ID).Delete(&model.CustomPermissionGrant{}).Error)
	return db, user, app, session, &permission
}

func TestCustomPermissionManagement(t *testing.T) {
	db, user, app, _, p := customFixture(t)
	svc := NewCustomPermissionService(db)
	for _, code := range []string{"", "openid", "email", "bad code", "中文"} {
		_, err := svc.SavePermission(0, dto.CustomPermissionRequest{Code: code, Name: "Read"}, user.ID)
		require.ErrorIs(t, err, ErrCustomPermissionInvalid)
	}
	_, err := svc.SavePermission(0, dto.CustomPermissionRequest{Code: p.Code, Name: "Duplicate"}, user.ID)
	require.ErrorIs(t, err, ErrCustomPermissionInvalid)
	_, err = svc.SavePermission(p.ID, dto.CustomPermissionRequest{Code: "changed", Name: p.Name}, user.ID)
	require.ErrorIs(t, err, ErrCustomPermissionInvalid)
	disabled := false
	p, err = svc.SavePermission(p.ID, dto.CustomPermissionRequest{Name: "Renamed", Enabled: &disabled}, user.ID)
	require.NoError(t, err)
	require.False(t, p.Enabled)
	detail, err := svc.GetPermission(p.ID)
	require.NoError(t, err)
	require.Equal(t, app.ID, detail.Applications[0].ID)
	_, err = NewApplicationService(db).UpdateApplication(app.ID, &dto.UpdateApplicationRequest{ProtocolConfig: &map[string]interface{}{"customPermissionIds": []uint{999999}}})
	require.ErrorContains(t, err, "请求参数错误")
	require.ErrorIs(t, svc.DeletePermission(p.ID), ErrCustomPermissionInvalid)
	app.ProtocolConfig["customPermissionIds"] = []uint{}
	require.NoError(t, db.Save(app).Error)
	grant, err := svc.SaveGrant(0, dto.CustomPermissionGrantRequest{PermissionID: p.ID, SubjectType: "user", SubjectID: user.ID}, user.ID)
	require.NoError(t, err)
	require.Nil(t, grant.EndsAt)
	require.ErrorIs(t, svc.DeletePermission(p.ID), ErrCustomPermissionInvalid)
	require.NoError(t, svc.RevokeGrant(grant.ID, user.ID))
	require.NoError(t, svc.DeletePermission(p.ID))
	_, err = svc.SavePermission(0, dto.CustomPermissionRequest{Code: p.Code, Name: "Reused"}, user.ID)
	require.ErrorIs(t, err, ErrCustomPermissionInvalid)
	var revoked model.CustomPermissionGrant
	require.NoError(t, db.Unscoped().First(&revoked, grant.ID).Error)
	require.True(t, revoked.DeletedAt.Valid)
	require.Equal(t, user.ID, revoked.UpdatedBy)
}

func TestCustomGrantValidationAndFilters(t *testing.T) {
	db, user, _, _, p := customFixture(t)
	svc := NewCustomPermissionService(db)
	now := time.Now()
	later := now.Add(time.Hour)
	earlier := now.Add(-time.Hour)
	base := dto.CustomPermissionGrantRequest{PermissionID: p.ID, SubjectType: "user", SubjectID: user.ID}
	for _, scenario := range []string{"missing-end", "reversed", "unknown-user", "unknown-type", "everyone-id", "subdepartments", "invalid-client"} {
		req := base
		switch scenario {
		case "missing-end":
			req.StartsAt = &now
		case "reversed":
			req.StartsAt, req.EndsAt = &later, &now
		case "unknown-user":
			req.SubjectID = 99999
		case "unknown-type":
			req.SubjectType = "unknown"
		case "everyone-id":
			req.SubjectType = "everyone"
		case "subdepartments":
			req.IncludeSubDepartments = true
		case "invalid-client":
			req.SubjectType = "client"
		}
		_, err := svc.SaveGrant(0, req, user.ID)
		require.Error(t, err, scenario)
	}
	for _, pair := range [][2]*time.Time{{nil, nil}, {&now, &later}, {&earlier, &now}, {&later, func() *time.Time { v := later.Add(time.Hour); return &v }()}} {
		req := base
		req.StartsAt, req.EndsAt = pair[0], pair[1]
		_, err := svc.SaveGrant(0, req, user.ID)
		require.NoError(t, err)
	}
	for state, count := range map[string]int{"active": 2, "expired": 1, "pending": 1} {
		rows, err := svc.ListGrants(dto.CustomPermissionQuery{State: state, SubjectType: "user", SubjectID: user.ID})
		require.NoError(t, err)
		require.Equal(t, count, rows.Total)
	}
}

func TestCustomGrantTimeBoundariesAndAlternatives(t *testing.T) {
	db, user, app, _, p := customFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-time.Hour), now.Add(time.Hour)
	grant := model.CustomPermissionGrant{PermissionID: p.ID, SubjectType: "user", SubjectID: user.ID, StartsAt: &start, EndsAt: &end}
	require.NoError(t, db.Create(&grant).Error)
	svc := NewOIDCAuthorizationService(db)
	evaluate := func(at time.Time) (*time.Time, error) {
		return svc.evaluateCustomPermissions(app, user.ID, []string{p.Code}, map[string]model.CustomPermission{p.Code: *p}, false, at)
	}
	_, err := evaluate(start.Add(-time.Nanosecond))
	require.ErrorIs(t, err, constants.ErrAccessDenied)
	until, err := evaluate(start)
	require.NoError(t, err)
	require.True(t, until.Equal(end))
	_, err = evaluate(end)
	require.ErrorIs(t, err, constants.ErrAccessDenied)
	longEnd := end.Add(time.Hour)
	other := grant
	other.ID = 0
	other.SubjectType = "everyone"
	other.SubjectID = 0
	other.EndsAt = &longEnd
	require.NoError(t, db.Create(&other).Error)
	until, err = evaluate(now)
	require.NoError(t, err)
	require.True(t, until.Equal(longEnd))
	permanent := model.CustomPermissionGrant{PermissionID: p.ID, SubjectType: "user", SubjectID: user.ID}
	require.NoError(t, db.Create(&permanent).Error)
	until, err = evaluate(now)
	require.NoError(t, err)
	require.Nil(t, until)
	require.NoError(t, db.Delete(&permanent).Error)
	// A short-lived role membership cannot make a longer scope grant permanent.
	var role model.Role
	require.NoError(t, db.First(&role).Error)
	roleEnd := now.Add(30 * time.Minute)
	require.NoError(t, db.Model(&model.UserRole{}).Where("user_id = ?", user.ID).Update("expires_at", roleEnd).Error)
	require.NoError(t, db.Where("permission_id = ?", p.ID).Delete(&model.CustomPermissionGrant{}).Error)
	require.NoError(t, db.Create(&model.CustomPermissionGrant{PermissionID: p.ID, SubjectType: "role", SubjectID: role.ID}).Error)
	until, err = evaluate(now)
	require.NoError(t, err)
	require.True(t, until.Equal(roleEnd))
}

func TestCustomGlobalBindingsAndLegacyCannotBypass(t *testing.T) {
	db, user, app, _, p := customFixture(t)
	require.NoError(t, db.Create(&model.CustomPermissionGrant{PermissionID: p.ID, SubjectType: "everyone"}).Error)
	svc := NewOIDCAuthorizationService(db)
	app.ProtocolConfig["scopeAuthorization"] = model.OIDCScopeAuthorization{Enabled: false}
	require.NoError(t, db.Save(app).Error)
	_, err := svc.evaluateOIDCGrant(app, user.ID, []string{p.Code}, false)
	require.NoError(t, err)
	// A second application deliberately opts in; the same global grant applies.
	other := *app
	other.ID = 0
	other.UUID = "another-custom-client"
	require.NoError(t, db.Create(&other).Error)
	_, err = svc.evaluateOIDCGrant(&other, user.ID, []string{p.Code}, false)
	require.NoError(t, err)
	other.AccessScope = policyAccess(false)
	require.NoError(t, db.Save(&other).Error)
	_, err = svc.evaluateOIDCGrant(&other, user.ID, []string{p.Code}, false)
	require.ErrorIs(t, err, constants.ErrAccessDenied)
	delete(app.ProtocolConfig, "customPermissionIds") // old code still remains in scopes
	require.NoError(t, db.Save(app).Error)
	_, err = svc.evaluateOIDCGrant(app, user.ID, []string{p.Code}, false)
	require.ErrorIs(t, err, constants.ErrInvalidScope)
	app.ProtocolConfig["customPermissionIds"] = []uint{p.ID}
	require.NoError(t, db.Save(app).Error)
	require.NoError(t, db.Model(p).Update("enabled", false).Error)
	_, err = svc.evaluateOIDCGrant(app, user.ID, []string{p.Code}, false)
	require.ErrorIs(t, err, constants.ErrInvalidScope)
}

func TestCustomTokenExpiryRefreshAndMultipleScopes(t *testing.T) {
	db, user, app, session, p := customFixture(t)
	svc := NewOIDCAuthorizationService(db)
	start, end := time.Now().Add(-time.Hour), time.Now().Add(2*time.Minute).Truncate(time.Second)
	grant, err := NewCustomPermissionService(db).SaveGrant(0, dto.CustomPermissionGrantRequest{PermissionID: p.ID, SubjectType: "user", SubjectID: user.ID, StartsAt: &start, EndsAt: &end}, user.ID)
	require.NoError(t, err)
	second := model.CustomPermission{Code: "files:read", Name: "Files", Enabled: true}
	require.NoError(t, db.Create(&second).Error)
	shortEnd := end.Add(-time.Minute)
	require.NoError(t, db.Create(&model.CustomPermissionGrant{PermissionID: second.ID, SubjectType: "user", SubjectID: user.ID, StartsAt: &start, EndsAt: &shortEnd}).Error)
	app.ProtocolConfig["customPermissionIds"] = []uint{p.ID, second.ID}
	require.NoError(t, db.Save(app).Error)
	issued, err := svc.GenerateTokens(app, user.ID, []string{"openid", p.Code, second.Code}, "", "https://issuer.example", session.ID)
	require.NoError(t, err)
	claims := departmentTokenClaims(t, issued.AccessToken)
	require.Equal(t, float64(shortEnd.Unix()), claims["exp"])
	stored, err := dao.NewOIDCTokenDAO(db).GetByAccessToken(issued.AccessToken)
	require.NoError(t, err)
	require.Equal(t, shortEnd.Unix(), stored.ExpiresAt.Unix())
	require.InDelta(t, time.Until(shortEnd).Seconds(), issued.ExpiresIn, 1)
	_, err = NewOIDCUserInfoService(db).GetUserInfo(issued.AccessToken)
	require.NoError(t, err)
	require.NoError(t, db.Delete(grant).Error)
	_, err = svc.RotateLoginToken(stored, app, "", "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidGrant)
	_, err = dao.NewOIDCTokenDAO(db).GetValidByRefreshToken(issued.RefreshToken)
	require.NoError(t, err)
	_, err = svc.RotateLoginToken(stored, app, "openid", "https://issuer.example")
	require.NoError(t, err)
	// Less than one whole second remaining cannot turn into a default-hour token.
	expired := time.Now().Truncate(time.Second)
	require.ErrorIs(t, svc.grantAccessTokenExpiry(&oidcGrant{App: app, ValidUntil: &expired}), constants.ErrAccessDenied)
}

func TestCustomTimedMachineAndBrowserFlows(t *testing.T) {
	db, user, app, session, p := customFixture(t)
	app.ProtocolConfig["clientType"] = "confidential"
	app.ProtocolConfig["clientSecret"] = "timed-secret"
	app.ProtocolConfig["requirePkce"] = false
	app.ProtocolConfig["grantTypes"] = []interface{}{"authorization_code", "refresh_token", "implicit", "client_credentials"}
	app.ProtocolConfig["responseTypes"] = []interface{}{"code", "token", "id_token"}
	require.NoError(t, protocol.NewOIDCProtocolHandler(db).EncryptSensitiveFields(app.ProtocolConfig))
	require.NoError(t, db.Save(app).Error)
	start, end := time.Now().Add(-time.Hour), time.Now().Add(2*time.Minute).Truncate(time.Second)
	permissions := NewCustomPermissionService(db)
	userGrant, err := permissions.SaveGrant(0, dto.CustomPermissionGrantRequest{PermissionID: p.ID, SubjectType: "user", SubjectID: user.ID, StartsAt: &start, EndsAt: &end}, user.ID)
	require.NoError(t, err)
	svc := NewOIDCAuthorizationService(db)
	machineReq := &dto.TokenRequest{GrantType: "client_credentials", ClientID: app.UUID, ClientSecret: "timed-secret", Scope: p.Code}
	_, err = svc.ExchangeClientCredentials(machineReq, "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidScope, "user grants cannot authorize a machine")
	machineGrant, err := permissions.SaveGrant(0, dto.CustomPermissionGrantRequest{PermissionID: p.ID, SubjectType: "client", SubjectID: app.ID, StartsAt: &start, EndsAt: &end}, user.ID)
	require.NoError(t, err)
	issued, err := svc.ExchangeClientCredentials(machineReq, "https://issuer.example")
	require.NoError(t, err)
	require.Equal(t, float64(end.Unix()), departmentTokenClaims(t, issued.AccessToken)["exp"])
	stored, err := dao.NewOIDCTokenDAO(db).GetByAccessToken(issued.AccessToken)
	require.NoError(t, err)
	require.Equal(t, end.Unix(), stored.ExpiresAt.Unix())
	for _, responseType := range []string{"code", "token", "id_token"} {
		req := &dto.AuthorizeRequest{ClientID: app.UUID, RedirectURI: "https://rp.example/callback", ResponseType: responseType, Scope: "openid " + p.Code, LoginSessionID: session.ID}
		auth, err := svc.ProcessAuthorize(req, user.ID, "https://issuer.example")
		require.NoError(t, err)
		require.Empty(t, auth.Error)
		switch responseType {
		case "code":
			tokens, err := svc.ExchangeCodeForToken(&dto.TokenRequest{GrantType: "authorization_code", ClientID: app.UUID, ClientSecret: "timed-secret", RedirectURI: req.RedirectURI, Code: auth.Code}, "https://issuer.example")
			require.NoError(t, err)
			require.Equal(t, float64(end.Unix()), departmentTokenClaims(t, tokens.AccessToken)["exp"])
		case "token":
			require.Equal(t, float64(end.Unix()), departmentTokenClaims(t, auth.Code)["exp"])
		}
	}
	// Revocation during an authorization-code round trip must prevent exchange.
	auth, err := svc.ProcessAuthorize(&dto.AuthorizeRequest{ClientID: app.UUID, RedirectURI: "https://rp.example/callback", ResponseType: "code", Scope: "openid " + p.Code, LoginSessionID: session.ID}, user.ID, "https://issuer.example")
	require.NoError(t, err)
	require.Empty(t, auth.Error)
	require.NoError(t, permissions.RevokeGrant(userGrant.ID, user.ID))
	_, err = svc.ExchangeCodeForToken(&dto.TokenRequest{GrantType: "authorization_code", ClientID: app.UUID, ClientSecret: "timed-secret", RedirectURI: "https://rp.example/callback", Code: auth.Code}, "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidGrant)
	require.NoError(t, permissions.RevokeGrant(machineGrant.ID, user.ID))
	_, err = svc.ExchangeClientCredentials(machineReq, "https://issuer.example")
	require.ErrorIs(t, err, constants.ErrInvalidScope)
}

func TestCustomPermissionStorageFailureDeniesIssuance(t *testing.T) {
	db, user, app, _, p := customFixture(t)
	require.NoError(t, db.Migrator().DropTable(&model.CustomPermissionGrant{}))
	_, err := NewOIDCAuthorizationService(db).evaluateOIDCGrant(app, user.ID, []string{p.Code}, false)
	require.ErrorIs(t, err, constants.ErrServerError)
}
