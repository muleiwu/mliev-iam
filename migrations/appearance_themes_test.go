package migrations

import (
	"database/sql"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestAppearanceThemeMigration(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	assertAppearanceThemeUpgrade(t, db, "sqlite")
}
func assertAppearanceThemeUpgrade(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	require.NoError(t, goose.SetDialect(dialectMap[driver]))
	goose.SetBaseFS(FS)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261015000000))
	gdb, err := openGormOnSQLDB(db, driver)
	require.NoError(t, err)
	before := model.Setting{Key: "appearance_settings_config", Value: `{"mode":"custom","custom":{"preset":"paper"}}`}
	require.NoError(t, gdb.Create(&before).Error)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261020000000))
	row := model.AppearanceTheme{UUID: "11111111-1111-4111-8111-111111111111", IdentityKey: "identity-one", Name: "导入主题", Version: "1.0.0", FormatVersion: 1, AppearanceJSON: `{"preset":"paper","customCss":"body{color:red}"}`}
	require.NoError(t, gdb.Create(&row).Error)
	duplicate := row
	duplicate.ID = 0
	duplicate.UUID = "22222222-2222-4222-8222-222222222222"
	require.Error(t, gdb.Create(&duplicate).Error)
	var stored model.AppearanceTheme
	require.NoError(t, gdb.Where("uuid = ?", row.UUID).First(&stored).Error)
	require.Equal(t, row.AppearanceJSON, stored.AppearanceJSON)
	var setting model.Setting
	require.NoError(t, gdb.First(&setting, before.ID).Error)
	require.Equal(t, before.Value, setting.Value)
	require.NoError(t, goose.DownTo(db, dirByDriver[driver], 20261015000000))
	require.False(t, gdb.Migrator().HasTable(&model.AppearanceTheme{}))
	require.NoError(t, gdb.First(&setting, before.ID).Error)
	require.Equal(t, before.Value, setting.Value)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261020000000))
	require.True(t, gdb.Migrator().HasTable(&model.AppearanceTheme{}))
}
