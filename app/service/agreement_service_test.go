package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
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

func agreementTestString(value string) *string { return &value }

func newAgreementTestApplication(t *testing.T, db *gorm.DB) *model.Application {
	t.Helper()
	app := &model.Application{
		UUID: "agreement-client", Name: "协议测试应用", IsActive: true,
		ProtocolType:   string(model.ProtocolTypeOIDC),
		ProtocolConfig: map[string]interface{}{"clientId": "agreement-client"},
	}
	require.NoError(t, db.Create(app).Error)
	return app
}

func TestAgreementServiceURLModePreservesSources(t *testing.T) {
	db := newAgreementServiceTestDB(t)
	app := newAgreementTestApplication(t, db)
	svc := NewAgreementService(db)
	req := &dto.AgreementUpsertRequest{
		Code: "external-policy", Title: "外部协议", ContentMD: "# 留存正文", Enabled: true,
		ContentType: agreementTestString("url"), ContentURL: agreementTestString("  https://example.com/policy?version=2#terms  "),
	}
	created, err := svc.CreateApplicationAgreement(app.ID, req)
	require.NoError(t, err)
	const target = "https://example.com/policy?version=2#terms"
	require.Equal(t, target, created.ContentURL)
	require.Equal(t, "url", created.ContentType)
	require.Equal(t, req.ContentMD, created.ContentMD)

	links, err := svc.BuildClientAgreementLinks(app)
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, target, links[0].URL)
	require.Equal(t, "application:external-policy", links[0].Key)
	public, err := svc.GetPublicApplicationAgreement(app.UUID, req.Code)
	require.NoError(t, err)
	require.Equal(t, target, public.ContentURL)
	require.Empty(t, public.ContentMD, "inactive Markdown must not be public")
	require.Equal(t, app.Name, public.AppName)

	// Old clients omit the new fields; updates must retain the active URL source.
	req.ContentType, req.ContentURL = nil, nil
	updated, err := svc.UpdateApplicationAgreement(app.ID, created.ID, req)
	require.NoError(t, err)
	require.Equal(t, "url", updated.ContentType)
	require.Equal(t, target, updated.ContentURL)

	req.ContentType = agreementTestString("markdown")
	updated, err = svc.UpdateApplicationAgreement(app.ID, created.ID, req)
	require.NoError(t, err)
	require.Equal(t, target, updated.ContentURL)
	require.Equal(t, req.ContentMD, updated.ContentMD)
	public, err = svc.GetPublicApplicationAgreement(app.UUID, req.Code)
	require.NoError(t, err)
	require.Equal(t, "markdown", public.ContentType)
	require.Equal(t, req.ContentMD, public.ContentMD)
	require.Empty(t, public.ContentURL, "inactive URL must not be public")
	links, err = svc.BuildClientAgreementLinks(app)
	require.NoError(t, err)
	require.Equal(t, "/auth/#/agreements/application?client_id=agreement-client&code=external-policy", links[0].URL)

	req.ContentType = agreementTestString("url")
	_, err = svc.UpdateApplicationAgreement(app.ID, created.ID, req)
	require.NoError(t, err)
	listed, err := svc.ListApplicationAgreements(app.ID, true)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "url", listed[0].ContentType)
	require.Equal(t, target, listed[0].ContentURL)
	require.Equal(t, req.ContentMD, listed[0].ContentMD)

	required, err := svc.ListRequiredRegistrationAgreements(&app.ID)
	require.NoError(t, err)
	require.Error(t, svc.EnsureAgreementsAccepted(required, nil))
	require.NoError(t, svc.EnsureAgreementsAccepted(required, []string{links[0].Key}))
	require.NoError(t, svc.RecordAcceptances(7, &app.ID, required))
	var acceptance model.AgreementAcceptance
	require.NoError(t, db.First(&acceptance).Error)
	hash := sha256.Sum256([]byte("url\n" + target))
	require.Equal(t, hex.EncodeToString(hash[:]), acceptance.ContentSHA256)

	req.Enabled = false
	_, err = svc.UpdateApplicationAgreement(app.ID, created.ID, req)
	require.NoError(t, err)
	_, err = svc.GetPublicApplicationAgreement(app.UUID, req.Code)
	require.ErrorIs(t, err, ErrAgreementDisabled)
	links, err = svc.BuildClientAgreementLinks(app)
	require.NoError(t, err)
	require.Empty(t, links)
	req.Enabled = true
	_, err = svc.UpdateApplicationAgreement(app.ID, created.ID, req)
	require.NoError(t, err)
	require.NoError(t, db.Model(app).Update("is_active", false).Error)
	_, err = svc.GetPublicApplicationAgreement(app.UUID, req.Code)
	require.ErrorIs(t, err, ErrApplicationDisabled)
}

func TestAgreementServiceURLValidation(t *testing.T) {
	db := newAgreementServiceTestDB(t)
	app := newAgreementTestApplication(t, db)
	svc := NewAgreementService(db)
	req := &dto.AgreementUpsertRequest{
		Code: "test-policy", Title: "协议", ContentMD: "# Terms", Enabled: true,
	}
	created, err := svc.CreateApplicationAgreement(app.ID, req)
	require.NoError(t, err)
	require.Equal(t, "markdown", created.ContentType)
	require.Empty(t, created.ContentURL)

	req.ContentType = agreementTestString("url")
	for _, invalid := range []string{"", "  ", "/terms", "//example.com/terms", "javascript:alert(1)", "data:text/html,test", "file:///terms", "ftp://example.com", "https://", "https://exa mple.com", "https://example.com:65536/policy", "https://example.com/line\nbreak", "https://example.com\\@evil.test", "https://example.com/" + strings.Repeat("x", 2048)} {
		t.Run(invalid, func(t *testing.T) {
			req.ContentURL = &invalid
			_, err := svc.UpdateApplicationAgreement(app.ID, created.ID, req)
			require.Error(t, err)
			req.Code = "invalid-create"
			_, err = svc.CreateApplicationAgreement(app.ID, req)
			require.Error(t, err)
			req.Code = "test-policy"
		})
	}
	var persisted model.Agreement
	require.NoError(t, db.First(&persisted, created.ID).Error)
	require.Equal(t, "markdown", persisted.ContentType)
	require.Empty(t, persisted.ContentURL)
	for _, valid := range []string{"http://localhost:3000/terms", "https://example.com/terms?q=a%20b#section", "HTTPS://EXAMPLE.COM/policy", "https://例子.测试/协议"} {
		req.ContentURL = &valid
		_, err := svc.UpdateApplicationAgreement(app.ID, created.ID, req)
		require.NoError(t, err)
	}
	req.ContentType = agreementTestString("html")
	_, err = svc.UpdateApplicationAgreement(app.ID, created.ID, req)
	require.Error(t, err)
	req.ContentType = agreementTestString("url")
	_, err = svc.UpsertGlobalAgreement("terms", req)
	require.Error(t, err, "global agreements must remain Markdown-only")
}

func TestAgreementServiceMarkdownAcceptanceHashUnchanged(t *testing.T) {
	db := newAgreementServiceTestDB(t)
	svc := NewAgreementService(db)
	agreement := &model.Agreement{Scope: "global", Code: "terms", Title: "协议", ContentMD: "# Terms", ContentURL: "https://example.com/unused"}
	require.NoError(t, svc.RecordAcceptances(1, nil, []*model.Agreement{agreement}))
	var acceptance model.AgreementAcceptance
	require.NoError(t, db.First(&acceptance).Error)
	hash := sha256.Sum256([]byte(agreement.ContentMD))
	require.Equal(t, hex.EncodeToString(hash[:]), acceptance.ContentSHA256)
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
