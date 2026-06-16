package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// TestCreateUserStoresArgon2id 新建用户的密码哈希应为 argon2id。
func TestCreateUserStoresArgon2id(t *testing.T) {
	db := newMigratedTestDB(t)
	svc := NewUserService(db)

	resp, err := svc.CreateUser(&dto.CreateUserRequest{
		Username: "argon_user",
		Password: "p@ssw0rd",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	user, err := dao.NewUserDao(db).GetByID(resp.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !strings.HasPrefix(user.PasswordHash, "$argon2id$") {
		t.Fatalf("expected argon2id hash, got %q", user.PasswordHash)
	}
	if user.Salt != "" {
		t.Fatalf("expected empty salt for argon2id, got %q", user.Salt)
	}
}

// TestLegacySha256TransparentUpgrade 历史 sha256+salt 用户登录成功后透明升级为 argon2id。
func TestLegacySha256TransparentUpgrade(t *testing.T) {
	db := newMigratedTestDB(t)

	const (
		username = "legacy_sha"
		password = "oldpass"
		salt     = "saltsalt"
	)
	h := sha256.New()
	h.Write([]byte(password + salt))
	legacyHash := hex.EncodeToString(h.Sum(nil))

	// 直接落一条历史格式用户（绕过 CreateUser，模拟旧数据）。
	if err := db.Create(&model.User{
		Username:     username,
		Salt:         salt,
		PasswordHash: legacyHash,
		Status:       model.UserStatusActive,
		IsActivated:  true,
	}).Error; err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}

	svc := NewUserService(db)
	resp, err := svc.ValidateUserForLogin(username, password)
	if err != nil {
		t.Fatalf("legacy login should succeed: %v", err)
	}

	// 登录后应已升级为 argon2id。
	user, err := dao.NewUserDao(db).GetByID(resp.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !strings.HasPrefix(user.PasswordHash, "$argon2id$") {
		t.Fatalf("expected upgraded argon2id hash, got %q", user.PasswordHash)
	}
	if user.Salt != "" {
		t.Fatalf("expected salt cleared after upgrade, got %q", user.Salt)
	}

	// 升级后再次登录仍成功（走标准路径）。
	if _, err := svc.ValidateUserForLogin(username, password); err != nil {
		t.Fatalf("login after upgrade should succeed: %v", err)
	}
	// 错误密码失败。
	if _, err := svc.ValidateUserForLogin(username, "wrong"); err == nil {
		t.Fatal("wrong password should fail")
	}
}
