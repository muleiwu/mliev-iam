package service

import (
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCodeLoginUserService(t *testing.T) (*UserService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}

	return NewUserService(db), db
}

func createCodeLoginUser(t *testing.T, db *gorm.DB, user *model.User) {
	t.Helper()

	if user.Status == 0 {
		user.Status = model.UserStatusActive
	}
	user.IsActivated = true

	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func TestGetVerifiedUserForCodeLoginSuccess(t *testing.T) {
	userService, db := setupCodeLoginUserService(t)
	now := time.Now()
	email := "verified@example.com"
	phone := "13800138000"

	createCodeLoginUser(t, db, &model.User{
		Username:        "verified-email",
		Email:           &email,
		EmailVerifiedAt: &now,
		PasswordHash:    "unused",
	})
	createCodeLoginUser(t, db, &model.User{
		Username:        "verified-phone",
		Phone:           &phone,
		PhoneVerifiedAt: &now,
		PasswordHash:    "unused",
	})

	emailUser, err := userService.GetVerifiedUserForCodeLogin(VerificationTypeEmail, email)
	if err != nil {
		t.Fatalf("verified email should login: %v", err)
	}
	if emailUser.Username != "verified-email" {
		t.Fatalf("unexpected email user: %s", emailUser.Username)
	}

	phoneUser, err := userService.GetVerifiedUserForCodeLogin(VerificationTypePhone, phone)
	if err != nil {
		t.Fatalf("verified phone should login: %v", err)
	}
	if phoneUser.Username != "verified-phone" {
		t.Fatalf("unexpected phone user: %s", phoneUser.Username)
	}
}

func TestGetVerifiedUserForCodeLoginRejectsMissingUnverifiedAndDisabled(t *testing.T) {
	userService, db := setupCodeLoginUserService(t)
	email := "unverified@example.com"
	phone := "13800138001"
	disabledPhone := "13800138002"
	now := time.Now()

	createCodeLoginUser(t, db, &model.User{
		Username:     "unverified-email",
		Email:        &email,
		PasswordHash: "unused",
	})
	createCodeLoginUser(t, db, &model.User{
		Username:     "unverified-phone",
		Phone:        &phone,
		PasswordHash: "unused",
	})
	createCodeLoginUser(t, db, &model.User{
		Username:        "disabled-phone",
		Phone:           &disabledPhone,
		PhoneVerifiedAt: &now,
		Status:          model.UserStatusDisabled,
		PasswordHash:    "unused",
	})

	cases := []struct {
		name             string
		verificationType VerificationType
		receiver         string
		wantMessage      string
	}{
		{
			name:             "missing email",
			verificationType: VerificationTypeEmail,
			receiver:         "missing@example.com",
			wantMessage:      "邮箱未注册",
		},
		{
			name:             "missing phone",
			verificationType: VerificationTypePhone,
			receiver:         "13800138003",
			wantMessage:      "手机号未注册",
		},
		{
			name:             "unverified email",
			verificationType: VerificationTypeEmail,
			receiver:         email,
			wantMessage:      "邮箱未验证",
		},
		{
			name:             "unverified phone",
			verificationType: VerificationTypePhone,
			receiver:         phone,
			wantMessage:      "手机号未验证",
		},
		{
			name:             "disabled phone",
			verificationType: VerificationTypePhone,
			receiver:         disabledPhone,
			wantMessage:      "用户账户已禁用",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := userService.GetVerifiedUserForCodeLogin(tc.verificationType, tc.receiver)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("expected %q in error, got %q", tc.wantMessage, err.Error())
			}
		})
	}
}
