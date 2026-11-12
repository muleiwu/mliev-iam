package migrations

import (
	"database/sql"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestProfileSessionRotationMigration(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	assertProfileSessionRotationUpgrade(t, db, "sqlite")
}

func assertProfileSessionRotationUpgrade(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	require.NoError(t, goose.SetDialect(dialectMap[driver]))
	goose.SetBaseFS(FS)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261025000000))
	gdb, err := openGormOnSQLDB(db, driver)
	require.NoError(t, err)
	old := model.LoginSession{ID: "existing-session", UserID: 42, LoginMethod: "password", CreatedAt: time.Now(), LastActiveAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, gdb.Omit("RememberMe", "ActivationPending").Create(&old).Error)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261112000000))
	var stored model.LoginSession
	require.NoError(t, gdb.First(&stored, "id = ?", old.ID).Error)
	require.Nil(t, stored.RememberMe)
	require.False(t, stored.ActivationPending)
	require.Nil(t, stored.EndedAt)
	require.NoError(t, gdb.Model(&stored).Updates(map[string]any{"remember_me": true, "activation_pending": true}).Error)
	require.NoError(t, gdb.First(&stored, "id = ?", old.ID).Error)
	require.True(t, *stored.RememberMe)
	require.True(t, stored.ActivationPending)
	require.NoError(t, goose.Down(db, dirByDriver[driver]))
	require.False(t, gdb.Migrator().HasColumn(&model.LoginSession{}, "remember_me"))
	require.False(t, gdb.Migrator().HasColumn(&model.LoginSession{}, "activation_pending"))
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261112000000))
	stored = model.LoginSession{}
	require.NoError(t, gdb.First(&stored, "id = ?", old.ID).Error)
	require.Nil(t, stored.RememberMe)
	require.False(t, stored.ActivationPending)
	require.Equal(t, old.LoginMethod, stored.LoginMethod)
}
