package service

import (
	"errors"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testApplicationIconUUID = "018f47df-7f5b-7f88-a7bd-1d3d9a69ea12"
const replacementApplicationIconUUID = "018f47df-7f5b-7f88-a7bd-1d3d9a69ea13"

func setupApplicationIconTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Application{},
		&model.ApplicationOIDCKey{},
		&model.Attachment{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func createApplicationIconAttachment(t *testing.T, db *gorm.DB, attachmentUUID, bizType string) {
	t.Helper()
	attachment := &model.Attachment{
		UUID:           attachmentUUID,
		FileName:       "application-icon.png",
		FileSize:       1024,
		MimeType:       "image/png",
		StorageType:    model.AttachmentStorageTypeS3,
		StorageKey:     "iam/application-icons/application-icon.png",
		BizType:        bizType,
		UploaderUserID: 1,
	}
	if err := db.Create(attachment).Error; err != nil {
		t.Fatalf("create attachment: %v", err)
	}
}

func validOIDCApplicationRequest(icon string) *dto.CreateApplicationRequest {
	return &dto.CreateApplicationRequest{
		Name:         "图标测试应用",
		Icon:         icon,
		ProtocolType: string(model.ProtocolTypeOIDC),
		ProtocolConfig: map[string]interface{}{
			"redirectUris":  []interface{}{"https://app.example.com/callback"},
			"grantTypes":    []interface{}{"authorization_code", "refresh_token"},
			"responseTypes": []interface{}{"code"},
			"scopes":        []interface{}{"openid", "profile"},
			"clientType":    "public",
			"requirePkce":   true,
		},
	}
}

func TestApplicationIconCreateReadAndClear(t *testing.T) {
	db := setupApplicationIconTestDB(t)
	createApplicationIconAttachment(t, db, testApplicationIconUUID, model.AttachmentBizTypeApplicationIcon)
	createApplicationIconAttachment(t, db, replacementApplicationIconUUID, model.AttachmentBizTypeApplicationIcon)
	service := NewApplicationService(db)
	iconURL := "/attachments/" + testApplicationIconUUID

	created, err := service.CreateApplication(validOIDCApplicationRequest(iconURL), 1)
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	if created.Icon != iconURL {
		t.Fatalf("created icon = %q, want %q", created.Icon, iconURL)
	}

	loaded, err := service.GetApplication(created.ID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if loaded.Icon != iconURL {
		t.Fatalf("loaded icon = %q, want %q", loaded.Icon, iconURL)
	}

	replacementIcon := "/attachments/" + replacementApplicationIconUUID
	updated, err := service.UpdateApplication(created.ID, &dto.UpdateApplicationRequest{Icon: &replacementIcon})
	if err != nil {
		t.Fatalf("replace application icon: %v", err)
	}
	if updated.Icon != replacementIcon {
		t.Fatalf("replaced icon = %q, want %q", updated.Icon, replacementIcon)
	}

	emptyIcon := ""
	updated, err = service.UpdateApplication(created.ID, &dto.UpdateApplicationRequest{Icon: &emptyIcon})
	if err != nil {
		t.Fatalf("clear application icon: %v", err)
	}
	if updated.Icon != "" {
		t.Fatalf("updated icon = %q, want empty", updated.Icon)
	}
}

func TestApplicationIconRejectsInvalidReferences(t *testing.T) {
	db := setupApplicationIconTestDB(t)
	createApplicationIconAttachment(t, db, testApplicationIconUUID, model.AttachmentBizTypeAvatar)
	service := NewApplicationService(db)

	tests := []struct {
		name string
		icon string
	}{
		{name: "external URL", icon: "https://example.com/icon.png"},
		{name: "invalid UUID", icon: "/attachments/not-a-uuid"},
		{name: "missing attachment", icon: "/attachments/018f47df-7f5b-7f88-a7bd-1d3d9a69ea14"},
		{name: "wrong business type", icon: "/attachments/" + testApplicationIconUUID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.CreateApplication(validOIDCApplicationRequest(test.icon), 1)
			if !errors.Is(err, ErrInvalidApplicationIcon) {
				t.Fatalf("error = %v, want ErrInvalidApplicationIcon", err)
			}
		})
	}
}
