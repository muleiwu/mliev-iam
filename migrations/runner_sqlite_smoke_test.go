package migrations

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestSqliteSmoke 在内存 SQLite 上跑一遍 goose 迁移，验证 SQL 语法无误、所有关键列齐全。
// 本地 go test ./migrations -run TestSqliteSmoke 即可。
func TestSqliteSmoke(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := RunGooseMigrations(db, "sqlite", nil); err != nil {
		t.Fatalf("goose up 失败: %v", err)
	}

	// 新代码读写依赖的列都存在
	for _, col := range []string{"uuid", "email_verified_at", "phone_verified_at", "failure_count"} {
		var count int
		q := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = '%s'`, col)
		if err := db.QueryRow(q).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Fatalf("users 表缺少列: %s", col)
		}
	}

	// 已 DROP 的遗留列确实不存在
	var legacyCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'is_verified'`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatalf("users.is_verified 列本该被 drop_user_is_verified 迁移删除，实际还在")
	}

	var name string
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='captcha_configs'").Scan(&name); err != nil {
		t.Fatalf("captcha_configs 未创建: %v", err)
	}

	for _, table := range []string{"identity_providers", "user_identity_accounts"} {
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("%s 未创建: %v", table, err)
		}
	}

	var providerCount int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM social_providers
		WHERE identifier IN ('qq', 'wechat', 'wechat-miniprogram', 'github', 'google', 'oidc', 'generic-oauth2')
		  AND status = 0
	`).Scan(&providerCount); err != nil {
		t.Fatal(err)
	}
	if providerCount != 7 {
		t.Fatalf("预置 social provider 模板数量错误，期望 7，实际 %d", providerCount)
	}

	const expectedMaxVersion int64 = 20260617000001
	var maxV int64
	if err := db.QueryRow("SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version").Scan(&maxV); err != nil {
		t.Fatal(err)
	}
	if maxV != expectedMaxVersion {
		t.Fatalf("期望最大版本 %d，实际 %d", expectedMaxVersion, maxV)
	}
}

func TestReconcileKnownSchemaDriftMarksExistingFailureCountMigration(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(FS)
	if _, err := goose.EnsureDBVersion(sqlDB); err != nil {
		t.Fatal(err)
	}

	if err := gormDB.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, failure_count INTEGER NOT NULL DEFAULT 0)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := gormDB.Exec(
		"INSERT INTO "+goose.TableName()+" (version_id, is_applied, tstamp) VALUES (?, ?, ?)",
		userFailureCountPreviousMigrationVersion, true, time.Now(),
	).Error; err != nil {
		t.Fatal(err)
	}

	if err := reconcileKnownMySQLSchemaDrift(sqlDB, gormDB, "sqlite", stdoutLogger{}); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := gormDB.Raw(
		"SELECT COUNT(*) FROM "+goose.TableName()+" WHERE version_id = ? AND is_applied = ?",
		userFailureCountMigrationVersion, true,
	).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("期望补写 failure_count 迁移版本记录，实际 %d", count)
	}
}
