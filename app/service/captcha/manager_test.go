package captcha

import (
	"encoding/json"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newCaptchaManagerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/captcha.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.CaptchaConfig{}); err != nil {
		t.Fatalf("migrate captcha tables: %v", err)
	}
	return db
}

func createCaptchaSceneBinding(t *testing.T, db *gorm.DB, configID uint) {
	t.Helper()

	bindings := dto.CaptchaSceneBindings{Login: configID}
	raw, err := json.Marshal(bindings)
	if err != nil {
		t.Fatalf("marshal captcha bindings: %v", err)
	}

	setting := &model.Setting{
		Key:      service.CaptchaSceneBindingsKey,
		Value:    string(raw),
		Category: service.CaptchaCategory,
		Remark:   "验证码场景绑定配置",
	}
	if err := db.Create(setting).Error; err != nil {
		t.Fatalf("create captcha binding setting: %v", err)
	}
}

func TestCaptchaManagerValidateForLoginSceneDefaultsToPass(t *testing.T) {
	db := newCaptchaManagerTestDB(t)

	manager := NewManager(db)
	if err := manager.ValidateForScene("login", nil, "127.0.0.1", nil); err != nil {
		t.Fatalf("expected login captcha to pass when unbound, got %v", err)
	}
}

func TestCaptchaManagerValidateForLoginSceneRequiresTicketWhenEnabled(t *testing.T) {
	db := newCaptchaManagerTestDB(t)
	config := &model.CaptchaConfig{
		Name:     "login captcha",
		Provider: TencentProviderType,
		Enabled:  true,
		Params: datatypes.JSONMap{
			"secretId":     "secret-id",
			"secretKey":    "secret-key",
			"captchaAppId": "123456",
			"appSecretKey": "app-secret-key",
		},
	}
	if err := db.Create(config).Error; err != nil {
		t.Fatalf("create captcha config: %v", err)
	}
	createCaptchaSceneBinding(t, db, config.ID)

	manager := NewManager(db)
	if err := manager.ValidateForScene("login", nil, "127.0.0.1", nil); err == nil || err.Error() != "请完成验证码验证" {
		t.Fatalf("expected missing ticket error, got %v", err)
	}
}

func TestCaptchaManagerValidateForLoginScenePassesWhenBoundConfigDisabled(t *testing.T) {
	db := newCaptchaManagerTestDB(t)
	config := &model.CaptchaConfig{
		Name:     "disabled login captcha",
		Provider: TencentProviderType,
		Enabled:  true,
		Params:   datatypes.JSONMap{},
	}
	if err := db.Create(config).Error; err != nil {
		t.Fatalf("create captcha config: %v", err)
	}
	if err := db.Model(config).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable captcha config: %v", err)
	}
	createCaptchaSceneBinding(t, db, config.ID)

	manager := NewManager(db)
	if err := manager.ValidateForScene("login", nil, "127.0.0.1", nil); err != nil {
		t.Fatalf("expected disabled login captcha to pass, got %v", err)
	}
}
