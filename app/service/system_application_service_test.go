package service

import (
	"errors"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureAccountCenterApplicationCreatesAndRepairs(t *testing.T) {
	db := newSystemApplicationTestDB(t)
	app, err := EnsureAccountCenterApplication(db)
	if err != nil {
		t.Fatal(err)
	}
	wantID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://cnb.cool/mliev/iam/mliev-iam/account-center")).String()
	if app.UUID != wantID || !app.IsActive || !app.IsProtected || !app.IsVerified || app.AccessScope != nil || ApplicationAllowsRegistration(app) {
		t.Fatalf("unexpected account application: %+v", app)
	}
	handler := protocol.NewOIDCProtocolHandler(db)
	issuer := "https://iam.example/oidc/" + app.UUID
	if err := handler.ValidateRedirectURI(app, "https://iam.example"+AccountCenterCallbackPath, issuer); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"https://evil.example" + AccountCenterCallbackPath, "https://iam.example" + AdminConsoleCallbackPath, "https://iam.example" + AccountCenterCallbackPath + "?extra=1"} {
		if handler.ValidateRedirectURI(app, target, issuer) == nil {
			t.Fatalf("accepted callback %s", target)
		}
	}
	app.Name = "我的个人中心"
	app.IsActive, app.IsProtected = false, false
	app.AccessScope = &model.ApplicationAccessScope{RoleIDs: []uint{7}}
	app.ProtocolConfig = map[string]interface{}{"broken": true}
	if err := db.Save(app).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(app).Error; err != nil {
		t.Fatal(err)
	}
	repaired, err := EnsureAccountCenterApplication(db)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.ID != app.ID || repaired.Name != app.Name || !repaired.IsActive || !repaired.IsProtected || repaired.AccessScope != nil || repaired.DeletedAt.Valid || repaired.ProtocolConfig["requirePkce"] != true {
		t.Fatalf("account application was not repaired: %+v", repaired)
	}
	var keys int64
	db.Model(&model.ApplicationOIDCKey{}).Where("application_id = ? AND is_primary = ?", app.ID, true).Count(&keys)
	if keys != 1 {
		t.Fatalf("expected one signing key, got %d", keys)
	}
}

func TestApplicationAutoApprovesOnlyCoreProtectedClients(t *testing.T) {
	for _, id := range []string{AdminConsoleApplicationUUID, AccountCenterApplicationUUID, "other-client"} {
		for _, protected := range []bool{true, false} {
			app := &model.Application{UUID: id, IsProtected: protected}
			want := protected && id != "other-client"
			if ApplicationAutoApprovesConsent(app) != want {
				t.Fatalf("unexpected auto-approval for %s protected=%v", id, protected)
			}
		}
	}
	if ApplicationAutoApprovesConsent(nil) {
		t.Fatal("missing client cannot auto-approve")
	}
}

func newSystemApplicationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&model.Application{}, &model.ApplicationOIDCKey{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func TestEnsureAdminConsoleApplicationCreatesAndRepairsIdempotently(t *testing.T) {
	db := newSystemApplicationTestDB(t)
	app, err := EnsureAdminConsoleApplication(db, 7)
	if err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	if app.UUID != AdminConsoleApplicationUUID || !app.IsProtected || !app.IsActive || !app.IsVerified {
		t.Fatalf("unexpected built-in application: %+v", app)
	}
	if app.AccessScope == nil || len(app.AccessScope.RoleIDs) != 1 || app.AccessScope.RoleIDs[0] != 7 {
		t.Fatalf("unexpected access scope: %+v", app.AccessScope)
	}
	var keyCount int64
	if err := db.Model(&model.ApplicationOIDCKey{}).
		Where("application_id = ? AND is_primary = ?", app.ID, true).
		Count(&keyCount).Error; err != nil || keyCount != 1 {
		t.Fatalf("primary key count=%d err=%v", keyCount, err)
	}

	app.Name = "自定义后台名称"
	app.Description = "自定义描述"
	app.Icon = "/attachments/00000000-0000-0000-0000-000000000001"
	app.ProtocolType = "cas"
	app.ProtocolConfig = map[string]interface{}{"broken": true}
	app.IsActive = false
	app.IsVerified = false
	app.IsProtected = false
	app.AccessScope = nil
	if err := db.Save(app).Error; err != nil {
		t.Fatalf("corrupt application: %v", err)
	}
	if err := db.Unscoped().Where("application_id = ?", app.ID).Delete(&model.ApplicationOIDCKey{}).Error; err != nil {
		t.Fatalf("remove key: %v", err)
	}

	repaired, err := EnsureAdminConsoleApplication(db, 9)
	if err != nil {
		t.Fatalf("repair ensure: %v", err)
	}
	if repaired.ID != app.ID || repaired.Name != "自定义后台名称" || repaired.Description != "自定义描述" || repaired.Icon != app.Icon {
		t.Fatalf("display fields were not preserved: %+v", repaired)
	}
	if repaired.ProtocolType != "oidc" || !repaired.IsActive || !repaired.IsVerified || !repaired.IsProtected {
		t.Fatalf("critical fields were not repaired: %+v", repaired)
	}
	if repaired.AccessScope == nil || len(repaired.AccessScope.RoleIDs) != 1 || repaired.AccessScope.RoleIDs[0] != 9 {
		t.Fatalf("access scope was not repaired: %+v", repaired.AccessScope)
	}
	var appCount int64
	if err := db.Model(&model.Application{}).Where("uuid = ?", AdminConsoleApplicationUUID).Count(&appCount).Error; err != nil || appCount != 1 {
		t.Fatalf("application count=%d err=%v", appCount, err)
	}
	keyCount = 0
	if err := db.Model(&model.ApplicationOIDCKey{}).
		Where("application_id = ? AND is_primary = ?", repaired.ID, true).
		Count(&keyCount).Error; err != nil || keyCount != 1 {
		t.Fatalf("repaired primary key count=%d err=%v", keyCount, err)
	}
}

func TestApplicationAllowsRegistration(t *testing.T) {
	regular := &model.Application{UUID: "client-a", IsProtected: false}
	if !ApplicationAllowsRegistration(regular) {
		t.Fatal("regular applications should allow registration")
	}

	admin := &model.Application{UUID: AdminConsoleApplicationUUID, IsProtected: true}
	if ApplicationAllowsRegistration(admin) {
		t.Fatal("the built-in admin console must not allow registration")
	}

	if ApplicationAllowsRegistration(nil) {
		t.Fatal("a missing application must not allow registration")
	}
}

func TestProtectedApplicationAllowsCosmeticsAndRejectsCriticalChanges(t *testing.T) {
	db := newSystemApplicationTestDB(t)
	app, err := EnsureAdminConsoleApplication(db, 7)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	applicationService := NewApplicationService(db)

	name := "新的后台名称"
	if _, err := applicationService.UpdateApplication(app.ID, &dto.UpdateApplicationRequest{Name: &name}); err != nil {
		t.Fatalf("cosmetic update should succeed: %v", err)
	}
	config := map[string]interface{}{"redirectUris": []interface{}{"https://evil.example/callback"}}
	if _, err := applicationService.UpdateApplication(app.ID, &dto.UpdateApplicationRequest{ProtocolConfig: &config}); !errors.Is(err, ErrApplicationProtected) {
		t.Fatalf("protocol update error=%v, want protected", err)
	}
	disabled := false
	if _, err := applicationService.UpdateApplication(app.ID, &dto.UpdateApplicationRequest{IsActive: &disabled}); !errors.Is(err, ErrApplicationProtected) {
		t.Fatalf("disable error=%v, want protected", err)
	}
	if err := applicationService.UpdateAccessScope(app.ID, &dto.ApplicationAccessScopeRequest{}); !errors.Is(err, ErrApplicationProtected) {
		t.Fatalf("scope update error=%v, want protected", err)
	}
	public := true
	if err := applicationService.UpdateAccessScope(app.ID, &dto.ApplicationAccessScopeRequest{IsPublic: &public}); !errors.Is(err, ErrApplicationProtected) {
		t.Fatalf("public access update error=%v, want protected", err)
	}
	if err := applicationService.ValidateBatchOperation("disable", []uint{app.ID}); !errors.Is(err, ErrApplicationProtected) {
		t.Fatalf("batch validation error=%v, want protected", err)
	}
	if err := applicationService.DeleteApplication(app.ID); !errors.Is(err, ErrApplicationProtected) {
		t.Fatalf("delete error=%v, want protected", err)
	}
}
