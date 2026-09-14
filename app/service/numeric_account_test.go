package service

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"strings"
	"testing"
)

func TestNumericAccountDoesNotFallBackToPhone(t *testing.T) {
	db := newMigratedTestDB(t)
	s := NewUserService(db)
	numeric, err := s.CreateUser(&dto.CreateUserRequest{Username: "13800138000", Password: "NumericAccount!123"})
	if err != nil {
		t.Fatal(err)
	}
	phone, err := s.CreateUser(&dto.CreateUserRequest{Username: "phone_user", Phone: "+8613800138000", Email: "phone@example.test", Password: "PhoneAccount!456"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		account, password string
		id                uint
	}{
		{"13800138000", "NumericAccount!123", numeric.ID},
		{"+8613800138000", "PhoneAccount!456", phone.ID},
		{"phone@example.test", "PhoneAccount!456", phone.ID},
		{"phone_user", "PhoneAccount!456", phone.ID},
	} {
		got, err := s.ValidateUserForLogin(v.account, v.password)
		if err != nil || got.ID != v.id {
			t.Fatalf("login %s: %v, %v", v.account, got, err)
		}
	}
	for _, v := range []struct{ account, password string }{
		{"13800138000", "PhoneAccount!456"},
		{"+8613800138000", "NumericAccount!123"},
	} {
		if _, err := s.ValidateUserForLogin(v.account, v.password); err == nil {
			t.Fatal("cross-method password accepted")
		}
	}
}

func TestExplicitUnactivatedAccountAndLegacyFactory(t *testing.T) {
	db := newMigratedTestDB(t)
	s := NewUserService(db)
	u, err := s.CreateUnactivatedUserWithUsername("000100", "导入账户")
	if err != nil {
		t.Fatal(err)
	}
	var stored model.User
	if err = db.First(&stored, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Username != "000100" || stored.IsActivated || stored.PasswordHash != "" || stored.CanLogin() {
		t.Fatal("incorrect provisioning state")
	}
	if _, err = s.ValidateUserForLogin("000100", "anything"); err == nil {
		t.Fatal("unactivated account logged in")
	}
	if _, err = s.CreateUnactivatedUserWithUsername("000100", "重复账户"); err == nil {
		t.Fatal("duplicate account created")
	}
	if _, err = s.CreateUnactivatedUserWithUsername("a@b", "错误账户"); err == nil {
		t.Fatal("invalid account created")
	}
	generated, err := s.CreateUnactivatedUser("兼容账户")
	if err != nil || !strings.HasPrefix(generated.Username, "u_") {
		t.Fatalf("existing factory changed: %v", err)
	}
}
