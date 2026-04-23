package migrations

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/glebarez/go-sqlite"
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
	for _, col := range []string{"uuid", "email_verified_at", "phone_verified_at"} {
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

	const expectedMaxVersion int64 = 20260424040609
	var maxV int64
	if err := db.QueryRow("SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version").Scan(&maxV); err != nil {
		t.Fatal(err)
	}
	if maxV != expectedMaxVersion {
		t.Fatalf("期望最大版本 %d，实际 %d", expectedMaxVersion, maxV)
	}
}
