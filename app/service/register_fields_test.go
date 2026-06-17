package service

import (
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRegisterFieldsTestService(t *testing.T) (*SettingService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}

	return NewSettingService(db), db
}

func TestRegisterFieldsSettingsBackfillsUsernameForLegacyConfig(t *testing.T) {
	svc, db := newRegisterFieldsTestService(t)

	if err := db.Create(&model.Setting{
		Key:      RegisterFieldsConfigKey,
		Value:    `{"email":{"visible":true,"required":true,"label":"邮箱地址","placeholder":"请输入邮箱地址"},"phone":{"visible":false,"required":false,"label":"手机号码","placeholder":"请输入手机号码"},"realName":{"visible":true,"required":true,"label":"真实姓名","placeholder":"请输入真实姓名"},"nickname":{"visible":false,"required":false,"label":"昵称","placeholder":"请输入昵称"}}`,
		Category: RegisterCategory,
	}).Error; err != nil {
		t.Fatalf("create legacy setting: %v", err)
	}

	settings, err := svc.GetRegisterFieldsSettings()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if !settings.Username.Visible || !settings.Username.Required {
		t.Fatalf("username default = %+v, want visible and required", settings.Username)
	}
}

func TestValidateRegisterFieldsChecksUsernameRequirement(t *testing.T) {
	svc, _ := newRegisterFieldsTestService(t)

	err := svc.ValidateRegisterFields(&dto.CreateUserRequest{
		Email:    "user@example.com",
		RealName: "测试用户",
	})
	if err == nil || !strings.Contains(err.Error(), "用户名为必填项") {
		t.Fatalf("err = %v, want username required", err)
	}
}

func TestValidateRegisterFieldsAllowsOptionalUsername(t *testing.T) {
	svc, _ := newRegisterFieldsTestService(t)

	if err := svc.UpdateRegisterFieldsSettings(&dto.UpdateRegisterFieldsRequest{
		Username: dto.RegisterFieldConfig{
			Visible:     true,
			Required:    false,
			Label:       "用户名",
			Placeholder: "请输入用户名",
		},
		Email: dto.RegisterFieldConfig{
			Visible:     true,
			Required:    true,
			Label:       "邮箱地址",
			Placeholder: "请输入邮箱地址",
		},
		Phone: dto.RegisterFieldConfig{
			Visible:     false,
			Required:    false,
			Label:       "手机号码",
			Placeholder: "请输入手机号码",
		},
		RealName: dto.RegisterFieldConfig{
			Visible:     true,
			Required:    true,
			Label:       "真实姓名",
			Placeholder: "请输入真实姓名",
		},
		Nickname: dto.RegisterFieldConfig{
			Visible:     false,
			Required:    false,
			Label:       "昵称",
			Placeholder: "请输入昵称",
		},
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	err := svc.ValidateRegisterFields(&dto.CreateUserRequest{
		Email:    "user@example.com",
		RealName: "测试用户",
	})
	if err != nil {
		t.Fatalf("optional username should pass: %v", err)
	}
}
