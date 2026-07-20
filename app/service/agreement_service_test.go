package service

import (
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAgreementServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Application{}, &model.Agreement{}, &model.AgreementAcceptance{}); err != nil {
		t.Fatalf("migrate agreement tables: %v", err)
	}
	return db
}

func TestAgreementServiceRegistrationAcceptance(t *testing.T) {
	db := newAgreementServiceTestDB(t)
	app := &model.Application{
		UUID:           "client-a",
		Name:           "测试应用",
		ProtocolType:   string(model.ProtocolTypeOIDC),
		ProtocolConfig: map[string]interface{}{"clientId": "client-a"},
		IsActive:       true,
	}
	if err := db.Create(app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}

	svc := NewAgreementService(db)
	if _, err := svc.UpsertGlobalAgreement(model.AgreementCodeTerms, &dto.AgreementUpsertRequest{
		Title:     "服务协议",
		ContentMD: "# Terms",
		Enabled:   true,
		Sort:      10,
	}); err != nil {
		t.Fatalf("upsert global terms: %v", err)
	}
	if _, err := svc.UpsertGlobalAgreement(model.AgreementCodePrivacy, &dto.AgreementUpsertRequest{
		Title:     "隐私协议",
		ContentMD: "# Privacy",
		Enabled:   false,
		Sort:      20,
	}); err != nil {
		t.Fatalf("upsert global privacy: %v", err)
	}
	if _, err := svc.CreateApplicationAgreement(app.ID, &dto.AgreementUpsertRequest{
		Code:      "refund-policy",
		Title:     "退款协议",
		ContentMD: "# Refund",
		Enabled:   true,
		Sort:      1,
	}); err != nil {
		t.Fatalf("create app agreement: %v", err)
	}

	required, err := svc.ListRequiredRegistrationAgreements(&app.ID)
	if err != nil {
		t.Fatalf("list required: %v", err)
	}
	if len(required) != 2 {
		t.Fatalf("required len = %d, want 2", len(required))
	}
	if err := svc.EnsureAgreementsAccepted(required, []string{"global:terms"}); err == nil || !strings.Contains(err.Error(), "退款协议") {
		t.Fatalf("expected missing app agreement error, got %v", err)
	}

	accepted, err := svc.ValidateRegistrationAcceptance(&app.ID, []string{"global:terms", "application:refund-policy"})
	if err != nil {
		t.Fatalf("validate accepted: %v", err)
	}
	if err := svc.RecordAcceptances(7, &app.ID, accepted); err != nil {
		t.Fatalf("record acceptances: %v", err)
	}

	var records []model.AgreementAcceptance
	if err := db.Order("id ASC").Find(&records).Error; err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("acceptance len = %d, want 2", len(records))
	}
	for _, record := range records {
		if record.UserID != 7 || record.ApplicationID != app.ID || len(record.ContentSHA256) != 64 {
			t.Fatalf("unexpected acceptance record: %+v", record)
		}
	}
}

func TestAgreementServicePublicAgreementRequiresEnabled(t *testing.T) {
	db := newAgreementServiceTestDB(t)
	svc := NewAgreementService(db)
	if _, err := svc.UpsertGlobalAgreement(model.AgreementCodeTerms, &dto.AgreementUpsertRequest{
		Title:     "服务协议",
		ContentMD: "# Terms",
		Enabled:   false,
		Sort:      10,
	}); err != nil {
		t.Fatalf("upsert terms: %v", err)
	}

	if _, err := svc.GetPublicGlobalAgreement(model.AgreementCodeTerms); err != ErrAgreementDisabled {
		t.Fatalf("err = %v, want ErrAgreementDisabled", err)
	}
}

func TestAgreementServicePublicGlobalAgreementHasNoApplicationBranding(t *testing.T) {
	db := newAgreementServiceTestDB(t)
	svc := NewAgreementService(db)
	if _, err := svc.UpsertGlobalAgreement(model.AgreementCodeTerms, &dto.AgreementUpsertRequest{
		Title:     "服务协议",
		ContentMD: "# Terms",
		Enabled:   true,
		Sort:      10,
	}); err != nil {
		t.Fatalf("upsert terms: %v", err)
	}

	response, err := svc.GetPublicGlobalAgreement(model.AgreementCodeTerms)
	if err != nil {
		t.Fatalf("get public global agreement: %v", err)
	}
	if response.AppName != "" || response.AppIcon != "" {
		t.Fatalf("global agreement must not contain app branding: %+v", response)
	}
}

func TestAgreementServicePublicApplicationAgreementIncludesBranding(t *testing.T) {
	db := newAgreementServiceTestDB(t)
	app := &model.Application{
		UUID:           "client-branding",
		Name:           "品牌测试应用",
		Icon:           "/attachments/018f47df-7f5b-7f88-a7bd-1d3d9a69ea12",
		ProtocolType:   string(model.ProtocolTypeOIDC),
		ProtocolConfig: map[string]interface{}{"clientId": "client-branding"},
		IsActive:       true,
	}
	if err := db.Create(app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}

	svc := NewAgreementService(db)
	if _, err := svc.CreateApplicationAgreement(app.ID, &dto.AgreementUpsertRequest{
		Code:      "branding-policy",
		Title:     "品牌协议",
		ContentMD: "# Branding",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("create app agreement: %v", err)
	}

	response, err := svc.GetPublicApplicationAgreement(app.UUID, "branding-policy")
	if err != nil {
		t.Fatalf("get public app agreement: %v", err)
	}
	if response.AppName != app.Name || response.AppIcon != app.Icon {
		t.Fatalf("unexpected app branding: %+v", response)
	}

	if err := db.Model(app).Update("icon", "").Error; err != nil {
		t.Fatalf("clear app icon: %v", err)
	}
	response, err = svc.GetPublicApplicationAgreement(app.UUID, "branding-policy")
	if err != nil {
		t.Fatalf("get public app agreement without icon: %v", err)
	}
	if response.AppIcon != "" {
		t.Fatalf("app icon = %q, want empty", response.AppIcon)
	}
}
