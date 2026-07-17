package service

import (
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

func TestGetVerifiedUserForPasswordResetSuccess(t *testing.T) {
	userService, db := setupCodeLoginUserService(t)
	now := time.Now()
	email := "reset-verified@example.com"
	phone := "13800138010"

	createCodeLoginUser(t, db, &model.User{
		Username:        "reset-email",
		Email:           &email,
		EmailVerifiedAt: &now,
		PasswordHash:    "unused",
	})
	createCodeLoginUser(t, db, &model.User{
		Username:        "reset-phone",
		Phone:           &phone,
		PhoneVerifiedAt: &now,
		PasswordHash:    "unused",
	})

	emailUser, err := userService.GetVerifiedUserForPasswordReset(VerificationTypeEmail, email)
	if err != nil {
		t.Fatalf("verified email should pass: %v", err)
	}
	if emailUser.Username != "reset-email" {
		t.Fatalf("unexpected email user: %s", emailUser.Username)
	}

	phoneUser, err := userService.GetVerifiedUserForPasswordReset(VerificationTypePhone, phone)
	if err != nil {
		t.Fatalf("verified phone should pass: %v", err)
	}
	if phoneUser.Username != "reset-phone" {
		t.Fatalf("unexpected phone user: %s", phoneUser.Username)
	}
}

func TestGetVerifiedUserForPasswordResetRejectsMissingUnverifiedAndDisabled(t *testing.T) {
	userService, db := setupCodeLoginUserService(t)
	email := "reset-unverified@example.com"
	phone := "13800138011"
	disabledPhone := "13800138012"
	now := time.Now()

	createCodeLoginUser(t, db, &model.User{
		Username:     "reset-unverified-email",
		Email:        &email,
		PasswordHash: "unused",
	})
	createCodeLoginUser(t, db, &model.User{
		Username:     "reset-unverified-phone",
		Phone:        &phone,
		PasswordHash: "unused",
	})
	createCodeLoginUser(t, db, &model.User{
		Username:        "reset-disabled-phone",
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
			receiver:         "reset-missing@example.com",
			wantMessage:      "该邮箱未绑定任何账号",
		},
		{
			name:             "missing phone",
			verificationType: VerificationTypePhone,
			receiver:         "13800138013",
			wantMessage:      "该手机号未绑定任何账号",
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
			_, err := userService.GetVerifiedUserForPasswordReset(tc.verificationType, tc.receiver)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("expected %q in error, got %q", tc.wantMessage, err.Error())
			}
		})
	}
}
