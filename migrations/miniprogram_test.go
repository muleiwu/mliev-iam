package migrations

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"database/sql"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func assertMiniProgramSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.True(t, db.Migrator().HasTable(&model.WeChatIdentity{}))
	require.True(t, db.Migrator().HasTable(&model.WeChatIdentityConflict{}))
	require.True(t, db.Migrator().HasColumn(&model.LoginSession{}, "native_transaction_hash"))
	row := model.WeChatIdentity{Hash: "test-identity", Kind: "openid", Scope: "wx", Subject: "open", UserID: 1, CreatedAt: time.Now()}
	require.NoError(t, db.Create(&row).Error)
	row.UserID = 2
	require.Error(t, db.Create(&row).Error)
	require.NoError(t, db.Delete(&row).Error)
}
func TestMiniProgramMigrationScrubsSessionKeys(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	assertMiniProgramUpgrade(t, db, "sqlite")
}
func assertMiniProgramUpgrade(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	require.NoError(t, goose.SetDialect(dialectMap[driver]))
	goose.SetBaseFS(FS)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261020000000))
	gdb, err := openGormOnSQLDB(db, driver)
	require.NoError(t, err)
	connection := model.IdentityConnection{Name: "migration mini", Identifier: "migration-mini", Category: model.ConnectionCategorySocial, ProviderType: "wechat_miniprogram", Status: model.ConnectionStatusEnabled, Capabilities: model.StringList{"authentication"}}
	require.NoError(t, gdb.Omit("ShowGlobally").Create(&connection).Error)
	p := model.ExternalPrincipal{ConnectionID: connection.ID, Status: model.PrincipalStatusActive, RawProfile: model.JSONObject{"openid": "retain-openid", "session_key": "erase-secret"}}
	require.NoError(t, gdb.Create(&p).Error)
	require.NoError(t, RunGooseMigrations(db, driver, nil))
	assertMiniProgramSchema(t, gdb)
	var updated model.ExternalPrincipal
	require.NoError(t, gdb.First(&updated, p.ID).Error)
	require.Equal(t, "retain-openid", updated.RawProfile["openid"])
	require.NotContains(t, updated.RawProfile, "session_key")
}
