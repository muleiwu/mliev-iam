package migrations

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	_ "github.com/glebarez/go-sqlite"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSqliteSmoke(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := RunGooseMigrations(db, "sqlite", nil); err != nil {
		t.Fatalf("goose up 失败: %v", err)
	}

	for _, col := range []string{"uuid", "email_verified_at", "phone_verified_at", "failure_count"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = ?`, col).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Fatalf("users 表缺少列: %s", col)
		}
	}

	var legacyCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'is_verified'`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatal("users.is_verified 列本该被删除")
	}

	for _, table := range []string{
		"captcha_configs", "identity_connections", "identity_connection_credentials",
		"external_principals", "external_identifiers", "attachments",
		"directory_sync_policies", "external_org_units", "external_memberships",
		"directory_sync_runs", "directory_sync_changes",
		"sync_jobs", "sync_schedules", "external_event_inbox",
	} {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("%s 未创建: %v", table, err)
		}
	}
	for _, table := range []string{"social_providers", "identity_providers", "user_social_accounts", "user_identity_accounts"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("旧表 %s 应在硬切后删除", table)
		}
	}

	var providerCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM identity_connections
		WHERE identifier IN ('qq', 'wechat', 'wechat-miniprogram', 'github', 'google', 'oidc', 'generic-oauth2')
		  AND category = 'social' AND status = 'disabled'
	`).Scan(&providerCount); err != nil {
		t.Fatal(err)
	}
	if providerCount != 7 {
		t.Fatalf("预置连接模板数量错误，期望 7，实际 %d", providerCount)
	}

	var applicationIconColumnCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('applications') WHERE name = 'icon'`).Scan(&applicationIconColumnCount); err != nil {
		t.Fatal(err)
	}
	if applicationIconColumnCount != 1 {
		t.Fatal("applications 表缺少 icon 列")
	}
	var applicationProtectedColumnCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('applications') WHERE name = 'is_protected'`).Scan(&applicationProtectedColumnCount); err != nil {
		t.Fatal(err)
	}
	if applicationProtectedColumnCount != 1 {
		t.Fatal("applications 表缺少 is_protected 列")
	}

	var maxV int64
	if err := db.QueryRow("SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version").Scan(&maxV); err != nil {
		t.Fatal(err)
	}
	const remediationSchemaVersion int64 = 20260827000001
	if maxV != remediationSchemaVersion {
		t.Fatalf("期望最大版本 %d，实际 %d", remediationSchemaVersion, maxV)
	}
	for _, table := range []string{"external_principal_conflicts", "identity_connection_health_checks", "directory_sync_snapshot_objects", "directory_operation_leases"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("修复表 %s 未创建", table)
		}
	}

	if _, err := db.Exec(`INSERT INTO users (uuid, username, email, phone, status, is_activated) VALUES ('active-1', 'active-user', 'active@example.test', '10001', 1, 1)`); err != nil {
		t.Fatalf("写入首个活跃用户失败: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (uuid, username, email, phone, status, is_activated) VALUES ('active-2', 'active-user', 'active@example.test', '10001', 1, 1)`); err == nil {
		t.Fatal("活跃 username/email/phone 重复本应由数据库唯一约束拒绝")
	}
	if _, err := db.Exec(`UPDATE users SET deleted_at=CURRENT_TIMESTAMP WHERE uuid='active-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (uuid, username, email, phone, status, is_activated) VALUES ('active-3', 'active-user', 'active@example.test', '10001', 1, 1)`); err != nil {
		t.Fatalf("软删除后应允许复用活跃唯一字段: %v", err)
	}
}

func TestIdentityConnectionHardCutMigratesBindingsAndEncryptsCredentials(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db := openPreCutoverSQLite(t)
	defer db.Close()

	if _, err := db.Exec(`UPDATE social_providers SET client_id='github-client', client_secret='do-not-leak', status=1 WHERE identifier='github'`); err != nil {
		t.Fatal(err)
	}
	var githubID uint
	if err := db.QueryRow(`SELECT id FROM social_providers WHERE identifier='github'`).Scan(&githubID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_social_accounts
		(user_id, provider_id, open_id, union_id, user_info, access_token, refresh_token, login_count, created_at, updated_at)
		VALUES (42, ?, 'github-subject', 'github-union', '{"name":"Octo User","email":"octo@example.com"}', 'legacy-access-token', 'legacy-refresh-token', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, githubID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO identity_providers
		(name, identifier, type, client_id, client_secret, redirect_url, status, attributes, created_at, updated_at)
		VALUES ('飞书', 'company-feishu', 'feishu', 'feishu-app', 'feishu-secret', 'https://example.test/callback', 1, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	var feishuID uint
	if err := db.QueryRow(`SELECT id FROM identity_providers WHERE identifier='company-feishu'`).Scan(&feishuID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_identity_accounts
		(user_id, provider_id, external_user_id, union_id, tenant_id, user_info, access_token, refresh_token, login_count, created_at, updated_at)
		VALUES (43, ?, 'feishu-subject', 'feishu-union', 'tenant-1', '{"name":"Feishu User"}', 'enterprise-access-token', 'enterprise-refresh-token', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, feishuID); err != nil {
		t.Fatal(err)
	}

	if err := RunGooseMigrations(db, "sqlite", nil); err != nil {
		t.Fatalf("硬切迁移失败: %v", err)
	}

	var connectionCount, principalCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM identity_connections`).Scan(&connectionCount); err != nil {
		t.Fatal(err)
	}
	if connectionCount != 8 {
		t.Fatalf("期望迁移 8 个连接，实际 %d", connectionCount)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM external_principals`).Scan(&principalCount); err != nil {
		t.Fatal(err)
	}
	if principalCount != 2 {
		t.Fatalf("期望迁移 2 个账号，实际 %d", principalCount)
	}

	var connectionID uint
	var ciphertext, nonce, algorithm string
	var keyVersion int
	if err := db.QueryRow(`SELECT c.id, cr.ciphertext, cr.nonce, cr.algorithm, cr.key_version
		FROM identity_connections c JOIN identity_connection_credentials cr ON cr.connection_id=c.id
		WHERE c.identifier='github' AND cr.purpose='authentication'`).Scan(&connectionID, &ciphertext, &nonce, &algorithm, &keyVersion); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "do-not-leak") || strings.Contains(ciphertext, "github-client") {
		t.Fatal("凭据以明文出现在密文字段")
	}
	cipher, err := credentialcrypto.FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := cipher.Decrypt(credentialcrypto.Envelope{
		Ciphertext: ciphertext, Nonce: nonce, Algorithm: algorithm, KeyVersion: keyVersion,
	}, credentialcrypto.AAD(connectionID, "authentication", keyVersion))
	if err != nil {
		t.Fatal(err)
	}
	var credential map[string]string
	if err := json.Unmarshal(plaintext, &credential); err != nil {
		t.Fatal(err)
	}
	if credential["clientSecret"] != "do-not-leak" {
		t.Fatal("加密凭据解密后内容不匹配")
	}

	for _, forbiddenColumn := range []string{"access_token", "refresh_token"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('external_principals') WHERE name=?`, forbiddenColumn).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("新账号表不应包含 %s", forbiddenColumn)
		}
	}
}

func TestIdentityConnectionCutoverStopsOnDuplicateIdentifier(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db := openPreCutoverSQLite(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO identity_providers
		(name, identifier, type, client_id, client_secret, redirect_url, status, created_at, updated_at)
		VALUES ('重复 GitHub', 'github', 'feishu', 'client', 'secret', 'https://example.test/callback', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	err := RunGooseMigrations(db, "sqlite", nil)
	if err == nil || !strings.Contains(err.Error(), "重复 identifier") {
		t.Fatalf("期望重复 identifier 阻止启动，实际错误: %v", err)
	}
	for _, table := range []string{"social_providers", "identity_providers", "user_social_accounts", "user_identity_accounts"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("迁移失败后旧表 %s 未保留", table)
		}
	}
}

func TestMigrationStopsBeforeSchemaChangesWithoutMasterKey(t *testing.T) {
	t.Setenv(credentialcrypto.EnvironmentKey, "")
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	err = RunGooseMigrations(db, "sqlite", nil)
	if err == nil || !strings.Contains(err.Error(), "凭据主密钥") {
		t.Fatalf("期望缺少主密钥阻止迁移，实际: %v", err)
	}
	var count int
	if queryErr := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&count); queryErr != nil {
		t.Fatal(queryErr)
	}
	if count != 0 {
		t.Fatal("主密钥检查失败后不应发生任何 schema 变更")
	}
}

func openPreCutoverSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(FS)
	if err := goose.UpTo(db, "sqlite", 20260720120000); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func setIdentityCredentialsTestKey(t *testing.T) {
	t.Helper()
	t.Setenv(credentialcrypto.EnvironmentKey, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
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
	if err := gormDB.Table(goose.TableName()).Create(&gooseVersionRecord{
		VersionID: userFailureCountPreviousMigrationVersion,
		IsApplied: true,
		Timestamp: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := reconcileKnownMySQLSchemaDrift(sqlDB, gormDB, "sqlite", stdoutLogger{}); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := gormDB.Table(goose.TableName()).
		Where("version_id = ? AND is_applied = ?", userFailureCountMigrationVersion, true).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("期望补写 failure_count 迁移版本记录，实际 %d", count)
	}
}
