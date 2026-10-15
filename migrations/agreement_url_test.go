package migrations

import (
	"database/sql"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestAgreementURLMigration(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	assertAgreementURLUpgrade(t, db, "sqlite")
}

// Exercise the actual SQL upgrade, downgrade and re-upgrade on every dialect.
func assertAgreementURLUpgrade(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	require.NoError(t, goose.SetDialect(dialectMap[driver]))
	goose.SetBaseFS(FS)
	require.NoError(t, goose.UpTo(db, dirByDriver[driver], 20261014010000))
	gdb, err := openGormOnSQLDB(db, driver)
	require.NoError(t, err)
	require.NoError(t, gdb.Exec(`INSERT INTO agreements (scope, application_id, code, title, content_md, enabled, sort, created_at, updated_at)
		VALUES ('application', 42, 'existing-policy', 'Existing policy', '# Existing content', TRUE, 7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`).Error)
	require.NoError(t, goose.Up(db, dirByDriver[driver]))
	var agreement model.Agreement
	require.NoError(t, gdb.Where("code = ?", "existing-policy").First(&agreement).Error)
	require.Equal(t, "markdown", agreement.ContentType)
	require.Empty(t, agreement.ContentURL)
	require.Equal(t, "# Existing content", agreement.ContentMD)
	require.True(t, agreement.Enabled)
	require.Equal(t, 7, agreement.Sort)

	agreement.ContentType = "url"
	agreement.ContentURL = "https://example.com/policy"
	require.NoError(t, gdb.Save(&agreement).Error)
	var stored model.Agreement
	require.NoError(t, gdb.First(&stored, agreement.ID).Error)
	require.Equal(t, agreement.ContentURL, stored.ContentURL)
	require.Equal(t, agreement.ContentMD, stored.ContentMD)

	require.NoError(t, goose.Down(db, dirByDriver[driver]))
	require.False(t, gdb.Migrator().HasColumn(&model.Agreement{}, "content_type"))
	require.False(t, gdb.Migrator().HasColumn(&model.Agreement{}, "content_url"))
	var content string
	require.NoError(t, gdb.Table("agreements").Select("content_md").Where("id = ?", agreement.ID).Scan(&content).Error)
	require.Equal(t, "# Existing content", content)
	require.NoError(t, goose.Up(db, dirByDriver[driver]))
	require.NoError(t, gdb.First(&stored, agreement.ID).Error)
	require.Equal(t, "markdown", stored.ContentType)
	require.Empty(t, stored.ContentURL)
	require.Equal(t, "# Existing content", stored.ContentMD)
}
