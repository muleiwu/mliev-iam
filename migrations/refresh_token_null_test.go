package migrations

import (
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestOIDCRefreshTokenNullMigration(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(FS)
	if err := goose.UpTo(db, "sqlite", 20260907000000); err != nil {
		t.Fatalf("创建升级前 schema 失败: %v", err)
	}
	insert := func(access, refresh string) int64 {
		t.Helper()
		result, err := db.Exec(`INSERT INTO oidc_tokens
			(access_token, access_token_hash, client_id, user_id, refresh_token)
			VALUES (?, ?, 'machine-client', 0, ?)`, access, access, refresh)
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	legacyID := insert("legacy-empty", "")
	insert("existing-user", "real-refresh")
	deletedID := insert("deleted-high-id", "later-deleted")
	if _, err := db.Exec("DELETE FROM oidc_tokens WHERE id = ?", deletedID); err != nil {
		t.Fatal(err)
	}

	if err := RunGooseMigrations(db, "sqlite", nil); err != nil {
		t.Fatalf("升级失败: %v", err)
	}
	var legacy sql.NullString
	if err := db.QueryRow("SELECT refresh_token FROM oidc_tokens WHERE id = ?", legacyID).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Valid {
		t.Fatalf("历史空字符串应改为 NULL，实际为 %q", legacy.String)
	}
	var real string
	if err := db.QueryRow("SELECT refresh_token FROM oidc_tokens WHERE access_token = 'existing-user'").Scan(&real); err != nil || real != "real-refresh" {
		t.Fatalf("已有真实刷新令牌发生变化: value=%q err=%v", real, err)
	}
	for _, access := range []string{"machine-a", "machine-b"} {
		result, err := db.Exec(`INSERT INTO oidc_tokens
			(access_token, access_token_hash, client_id, user_id, refresh_token)
			VALUES (?, ?, 'machine-client', 0, NULL)`, access, access)
		if err != nil {
			t.Fatalf("多个 NULL 应可共存: %v", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if id <= deletedID {
			t.Fatalf("SQLite 自增序列回退: deleted=%d next=%d", deletedID, id)
		}
	}
	if _, err := db.Exec(`INSERT INTO oidc_tokens
		(access_token, access_token_hash, client_id, user_id, refresh_token)
		VALUES ('duplicate-refresh', 'duplicate-refresh', 'machine-client', 0, 'real-refresh')`); err == nil {
		t.Fatal("重复非空刷新令牌应被唯一索引拒绝")
	}
	if err := goose.Down(db, "sqlite"); err == nil {
		t.Fatal("不可逆迁移的 Down 应明确拒绝执行")
	}
	version, err := goose.GetDBVersion(db)
	if err != nil || version != 20260922000001 {
		t.Fatalf("拒绝 Down 后迁移版本应保持不变: version=%d err=%v", version, err)
	}
}
