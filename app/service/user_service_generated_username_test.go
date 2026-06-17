package service

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newGeneratedUsernameTestService(t *testing.T) (*UserService, *gorm.DB) {
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

func TestGenerateAvailableUsernameRetriesOnCollision(t *testing.T) {
	svc, db := newGeneratedUsernameTestService(t)

	if err := db.Create(&model.User{
		Username:    "u_aaaaaaaaaaaaaaaa",
		Status:      model.UserStatusActive,
		IsActivated: true,
	}).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	calls := 0
	username, err := svc.generateAvailableUsername(func() (string, error) {
		calls++
		if calls == 1 {
			return "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil
		}
		return "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", nil
	})
	if err != nil {
		t.Fatalf("generate username: %v", err)
	}
	if username != "u_bbbbbbbbbbbbbbbb" {
		t.Fatalf("username = %s, want u_bbbbbbbbbbbbbbbb", username)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestGenerateAvailableUsernameReturnsFormattedUsername(t *testing.T) {
	svc, _ := newGeneratedUsernameTestService(t)

	username, err := svc.GenerateAvailableUsername()
	if err != nil {
		t.Fatalf("generate username: %v", err)
	}
	if len(username) != len("u_0000000000000000") || username[:2] != "u_" {
		t.Fatalf("unexpected username format: %s", username)
	}
}
