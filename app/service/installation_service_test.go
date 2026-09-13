package service

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/google/uuid"
)

func TestInstallationRetryPreservesAdministrator(t *testing.T) {
	db := newMigratedTestDB(t)
	admin := dto.AdminAccountInfo{Username: "install_admin", Password: "initial-password-123", Email: "admin@example.test", RealName: "Installer"}
	id := uuid.NewString()
	if err := initializeInstallationOnce(db, admin, id); err != nil {
		t.Fatal(err)
	}
	var before model.User
	if err := db.Where("username = ?", admin.Username).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	admin.Password, admin.Username = "replacement-password", "replacement-admin"
	if err := initializeInstallationOnce(db, admin, id); err != nil {
		t.Fatal(err)
	}
	var after model.User
	if err := db.First(&after, before.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.PasswordHash != before.PasswordHash || after.Username != before.Username {
		t.Fatal("retry changed administrator credentials")
	}
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count != 1 {
		t.Fatalf("retry duplicated users: %d", count)
	}
	if err := initializeInstallationOnce(db, admin, uuid.NewString()); err == nil {
		t.Fatal("another installation must not take over initialized data")
	}
}

func TestDatabaseValidationSupportsSQLiteWithoutNetworkFields(t *testing.T) {
	if err := validateDatabase(dto.DatabaseConfig{Driver: "sqlite", FilePath: "config/iam.db"}); err != nil {
		t.Fatal(err)
	}
	for _, config := range []dto.DatabaseConfig{{Driver: "sqlite", FilePath: ":memory:"}, {Driver: "mysql"}, {Driver: "unknown"}} {
		if err := validateDatabase(config); err == nil {
			t.Fatalf("accepted invalid database config: %+v", config)
		}
	}
}
