package migrations

import (
	"database/sql"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestLoginConnectionVisibilityMigration(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	assertLoginConnectionVisibilityUpgrade(t, db, "sqlite")
}

func assertLoginConnectionVisibilityUpgrade(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	require.NoError(t, goose.SetDialect(dialectMap[driver]))
	goose.SetBaseFS(FS)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261023000000))
	gdb, err := openGormOnSQLDB(db, driver)
	require.NoError(t, err)
	old := model.IdentityConnection{Name: "existing mini", Identifier: "existing-mini", Category: "social", ProviderType: "wechat_miniprogram", Status: "enabled", Capabilities: model.StringList{"authentication"}}
	require.NoError(t, gdb.Omit("ShowGlobally").Create(&old).Error)
	app := model.Application{UUID: "existing-client", Name: "existing app", ProtocolType: "oidc", ProtocolConfig: map[string]interface{}{}, IsActive: true}
	require.NoError(t, gdb.Omit("LoginConnections").Create(&app).Error)
	require.NoError(t, RunGooseMigrations(db, driver, nil))
	require.NoError(t, gdb.First(&old, old.ID).Error)
	require.True(t, old.ShowGlobally, "existing connections retain their display setting")
	require.NoError(t, gdb.First(&app, app.ID).Error)
	require.Equal(t, "inherit", app.LoginConnectionSettings().Mode)
	fresh := model.IdentityConnection{Name: "new mini", Identifier: "new-mini", Category: "social", ProviderType: "wechat_miniprogram", Status: "enabled", Capabilities: model.StringList{"authentication"}}
	require.NoError(t, gdb.Create(&fresh).Error)
	require.NoError(t, gdb.First(&fresh, fresh.ID).Error)
	require.False(t, fresh.ShowGlobally)
	app.LoginConnections = &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{fresh.ID}}
	require.NoError(t, gdb.Save(&app).Error)
	require.NoError(t, RunGooseMigrations(db, driver, nil))
	require.NoError(t, gdb.First(&app, app.ID).Error)
	require.Equal(t, []uint{fresh.ID}, app.LoginConnectionSettings().ConnectionIDs)
	require.NoError(t, gdb.First(&fresh, fresh.ID).Error)
	require.False(t, fresh.ShowGlobally, "repeated startup does not backfill again")
}
