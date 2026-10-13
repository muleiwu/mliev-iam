package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	setupkit "cnb.cool/mliev/open/setup-kit"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// IAM_SETUP_MYSQL_FIXTURE points to a JSON ConnectionValues object for a
// disposable MySQL or MariaDB database. Only a temporary table is created.
func TestSetupMySQLCompatibleJSON(t *testing.T) {
	path := os.Getenv("IAM_SETUP_MYSQL_FIXTURE")
	if path == "" {
		t.Skip("set IAM_SETUP_MYSQL_FIXTURE to test a real MySQL or MariaDB connection")
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var connection setupkit.ConnectionValues
	require.NoError(t, json.Unmarshal(data, &connection))
	require.Contains(t, []string{"mysql", "mariadb"}, connection.Driver)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	err = withSetupDatabase(ctx, connection, func(db *gorm.DB, sqlDB *sql.DB) error {
		// Keep the temporary table and JSON insert on the same SQL connection.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		if err := db.Exec("CREATE TEMPORARY TABLE setup_json_probe (value JSON NOT NULL)").Error; err != nil {
			return err
		}
		defer db.Exec("DROP TEMPORARY TABLE setup_json_probe")
		payload := datatypes.JSON(`{"database":"compatible"}`)
		if err := db.Table("setup_json_probe").Create(map[string]any{"value": payload}).Error; err != nil {
			return err
		}
		var stored string
		if err := db.Raw("SELECT value FROM setup_json_probe").Scan(&stored).Error; err != nil {
			return err
		}
		require.JSONEq(t, string(payload), stored)
		version := db.Dialector.(*mysql.Dialector).ServerVersion
		require.NotEmpty(t, version)
		if connection.Driver == "mariadb" {
			require.True(t, strings.Contains(version, "MariaDB"), "fixture must use a real MariaDB server")
		}
		return nil
	})
	require.NoError(t, err)
}
