package service

import (
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAppearancePersistenceAndInactiveDraft(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	s := NewSettingService(db)
	initial, err := s.GetPublicAppearance()
	if err != nil || initial.Mode != "default" || initial.Appearance.PrimaryColor != "" {
		t.Fatalf("initial = %+v, %v", initial, err)
	}
	config := DefaultAppearanceSettings()
	config.Mode = "custom"
	config.Custom.BrandName = "  示例品牌  "
	config.Custom.PrimaryColor = "#123abc"
	config.Custom.CustomCSS = ".auth-entry-title { font-size: 30px }"
	if err := s.UpdateAppearanceSettings(config); err != nil {
		t.Fatal(err)
	}
	active, _ := NewSettingService(db).GetPublicAppearance()
	if active.Appearance.BrandName != "示例品牌" || active.Appearance.PrimaryColor != "#123abc" {
		t.Fatalf("active = %+v", active)
	}
	config.Mode = "default"
	if err := s.UpdateAppearanceSettings(config); err != nil {
		t.Fatal(err)
	}
	active, _ = s.GetPublicAppearance()
	stored, _ := s.GetAppearanceSettings()
	if active.Appearance.CustomCSS != "" || active.Appearance.BrandName != "" || stored.Custom.CustomCSS == "" {
		t.Fatal("default must hide but retain the custom draft")
	}
	if _, err := s.Update(AppearanceSettingsConfigKey, &dto.UpdateSettingRequest{Value: `{"mode":"invalid"}`}); err == nil {
		t.Fatal("generic update bypassed validation")
	}
	if err := s.SetSimple(AppearanceSettingsConfigKey, `{"mode":"custom","custom":{"logo":"javascript:alert(1)"}}`); err == nil {
		t.Fatal("simple update bypassed validation")
	}
	if err := db.Model(&model.Setting{}).Where("setting_key = ?", AppearanceSettingsConfigKey).Update("value", "broken JSON").Error; err != nil {
		t.Fatal(err)
	}
	active, err = s.GetPublicAppearance()
	if err != nil || active.Mode != "default" {
		t.Fatal("corrupt stored configuration must fall back")
	}
}

func TestAppearanceValidation(t *testing.T) {
	if err := validateAppearanceSettingValue(AppearanceSettingsConfigKey, strings.Repeat(" ", 61*1024)+`{"mode":"default"}`); err == nil {
		t.Fatal("generic settings must reject oversized raw JSON")
	}
	negative, tooLarge := -1, 49
	cases := []struct {
		name   string
		change func(*dto.AppearanceSettings)
	}{
		{"mode", func(c *dto.AppearanceSettings) { c.Mode = "other" }},
		{"name", func(c *dto.AppearanceSettings) { c.Custom.BrandName = strings.Repeat("界", 101) }},
		{"color", func(c *dto.AppearanceSettings) { c.Custom.PrimaryColor = "red;display:none" }},
		{"dark", func(c *dto.AppearanceSettings) { c.Custom.DarkMode = "other" }},
		{"background", func(c *dto.AppearanceSettings) { c.Custom.BackgroundMode = "color" }},
		{"image", func(c *dto.AppearanceSettings) { c.Custom.BackgroundMode = "image" }},
		{"script", func(c *dto.AppearanceSettings) { c.Custom.Logo = "javascript:alert(1)" }},
		{"protocol relative", func(c *dto.AppearanceSettings) { c.Custom.Logo = "//example.com/image.png" }},
		{"backslash", func(c *dto.AppearanceSettings) { c.Custom.Logo = `/\example.com/image.png` }},
		{"credentials", func(c *dto.AppearanceSettings) { c.Custom.Logo = "https://user:password@example.com/image.png" }},
		{"negative radius", func(c *dto.AppearanceSettings) { c.Custom.ControlRadius = &negative }},
		{"large radius", func(c *dto.AppearanceSettings) { c.Custom.CardRadius = &tooLarge }},
		{"css bytes", func(c *dto.AppearanceSettings) { c.Custom.CustomCSS = strings.Repeat("界", 12000) }},
		{"json expansion", func(c *dto.AppearanceSettings) { c.Custom.CustomCSS = strings.Repeat("<", 12000) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultAppearanceSettings()
			tc.change(&c)
			if ValidateAppearanceSettings(&c) == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	for _, raw := range []string{"", "/attachments/test", "https://cdn.example.com/image.png?size=200", "http://localhost:8080/a.png"} {
		c := DefaultAppearanceSettings()
		c.Custom.Logo = raw
		if err := ValidateAppearanceSettings(&c); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}
