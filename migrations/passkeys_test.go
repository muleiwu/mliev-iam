package migrations

import (
	"database/sql"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func assertPasskeySchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.True(t, db.Migrator().HasTable(&model.Passkey{}))
	row := model.Passkey{UserID: 1, RPID: "localhost", UserHandle: "test-handle", CredentialIDHash: "unique-passkey", CredentialID: "credential", CredentialData: `{"publicKey":"test"}`, Name: "First", CreatedAt: time.Now().UTC()}
	require.NoError(t, db.Create(&row).Error)
	duplicate := row
	duplicate.ID = 0
	duplicate.UserID = 2
	require.Error(t, db.Create(&duplicate).Error, "credential identifiers must be unique across accounts")
	require.NoError(t, db.Model(&row).Updates(map[string]any{"name": "Renamed", "last_used_at": time.Now().UTC(), "credential_data": `{"signCount":2}`}).Error)
	var stored model.Passkey
	require.NoError(t, db.First(&stored, row.ID).Error)
	require.Equal(t, "Renamed", stored.Name)
	require.NotNil(t, stored.LastUsedAt)
	require.NoError(t, db.Delete(&row).Error)
}
func TestPasskeyUpgradePreservesFactorsAndClearsStaleContactVerification(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	assertPasskeyUpgrade(t, db, "sqlite")
}

func assertPasskeyUpgrade(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	require.NoError(t, goose.SetDialect(dialectMap[driver]))
	goose.SetBaseFS(FS)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261014000000))
	gdb, err := openGormOnSQLDB(db, driver)
	require.NoError(t, err)
	now := time.Now().UTC()
	email := "new@example.test"
	phone := "+8613800138000"
	u := model.User{Username: "upgrade-passkey", Email: &email, Phone: &phone, EmailVerifiedAt: &now, PhoneVerifiedAt: &now, TwoFactorEnabled: true, TwoFactorVersion: 3, Status: model.UserStatusActive, IsActivated: true}
	require.NoError(t, gdb.Create(&u).Error)
	require.NoError(t, gdb.Create(&model.TwoFactorMethod{UserID: u.ID, Method: "email", Receiver: "old@example.test", CreatedAt: now}).Error)
	require.NoError(t, gdb.Create(&model.TwoFactorMethod{UserID: u.ID, Method: "phone", Receiver: phone, CreatedAt: now}).Error)
	require.NoError(t, gdb.Create(&model.TwoFactorRecoveryCode{UserID: u.ID, CodeHash: "keep-recovery"}).Error)
	require.NoError(t, RunGooseMigrations(db, driver, nil))
	var actual model.User
	require.NoError(t, gdb.First(&actual, u.ID).Error)
	require.True(t, actual.TwoFactorEnabled)
	require.Nil(t, actual.EmailVerifiedAt)
	require.NotNil(t, actual.PhoneVerifiedAt)
	require.Equal(t, uint64(4), actual.TwoFactorVersion)
	var factors, recovery int64
	require.NoError(t, gdb.Model(&model.TwoFactorMethod{}).Count(&factors).Error)
	require.Equal(t, int64(2), factors)
	require.NoError(t, gdb.Model(&model.TwoFactorRecoveryCode{}).Count(&recovery).Error)
	require.Equal(t, int64(1), recovery)
	require.Error(t, goose.Down(db, dirByDriver[driver]))
	require.True(t, gdb.Migrator().HasTable(&model.Passkey{}))
}
