//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"
)

// These tests are intentionally behind the integration build tag. They are
// the production migration gate and require a running Docker daemon:
//
//	go test -tags=integration ./migrations -run TestMigrationDialects -count=1
//
// SQLite exercises the same scenarios in runner_sqlite_smoke_test.go. This
// suite verifies the real MySQL/PostgreSQL DDL and driver behaviour.
func TestMigrationDialects(t *testing.T) {
	fixtures := []struct {
		name   string
		driver string
		open   func(*testing.T) *sql.DB
	}{
		{name: "mysql-8.4", driver: "mysql", open: openMySQL84},
		{name: "postgresql-16", driver: "postgresql", open: openPostgreSQL16},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name+"/empty", func(t *testing.T) {
			db := fixture.open(t)
			assertBadKeyDoesNotTouchSchema(t, db, fixture.driver)
			setMigrationIntegrationKey(t)
			if err := RunGooseMigrations(db, fixture.driver, nil); err != nil {
				t.Fatalf("空库迁移失败: %v", err)
			}
			assertFinalMigrationSchema(t, db, fixture.driver)
		})
		t.Run(fixture.name+"/legacy-hard-cut", func(t *testing.T) {
			db := fixture.open(t)
			setMigrationIntegrationKey(t)
			prepareLegacyIdentitySchema(t, db, fixture.driver)
			seedLegacyIdentityBinding(t, db, fixture.driver)
			if err := RunGooseMigrations(db, fixture.driver, nil); err != nil {
				t.Fatalf("存量硬切失败: %v", err)
			}
			assertFinalMigrationSchema(t, db, fixture.driver)
			assertLegacyIdentityBindingMigrated(t, db, fixture.driver)
		})
		t.Run(fixture.name+"/passkey-upgrade", func(t *testing.T) {
			db := fixture.open(t)
			setMigrationIntegrationKey(t)
			assertPasskeyUpgrade(t, db, fixture.driver)
		})
		t.Run(fixture.name+"/refresh-token-upgrade", func(t *testing.T) {
			db := fixture.open(t)
			setMigrationIntegrationKey(t)
			if err := goose.SetDialect(dialectMap[fixture.driver]); err != nil {
				t.Fatal(err)
			}
			goose.SetBaseFS(FS)
			if err := goose.UpTo(db, dirByDriver[fixture.driver], 20260907000000); err != nil {
				t.Fatalf("创建升级前 schema 失败: %v", err)
			}
			gormDB, err := openGormOnSQLDB(db, fixture.driver)
			if err != nil {
				t.Fatal(err)
			}
			insert := `INSERT INTO oidc_tokens (access_token, access_token_hash, client_id, user_id, refresh_token) VALUES (?, ?, 'historical-client', 0, ?)`
			if err := gormDB.Exec(insert, "historical-empty", "historical-empty", "").Error; err != nil {
				t.Fatal(err)
			}
			if err := gormDB.Exec(insert, "historical-real", "historical-real", "real-refresh").Error; err != nil {
				t.Fatal(err)
			}
			if err := RunGooseMigrations(db, fixture.driver, nil); err != nil {
				t.Fatalf("升级失败: %v", err)
			}
			var old sql.NullString
			if err := db.QueryRow("SELECT refresh_token FROM oidc_tokens WHERE access_token = 'historical-empty'").Scan(&old); err != nil || old.Valid {
				t.Fatalf("历史空字符串应变为 NULL: value=%v err=%v", old, err)
			}
			var real string
			if err := db.QueryRow("SELECT refresh_token FROM oidc_tokens WHERE access_token = 'historical-real'").Scan(&real); err != nil || real != "real-refresh" {
				t.Fatalf("真实刷新令牌发生变化: value=%q err=%v", real, err)
			}
			assertFinalMigrationSchema(t, db, fixture.driver)
		})
	}
}

func openMySQL84(t *testing.T) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	container, err := mysqlcontainer.Run(ctx, "mysql:8.4",
		mysqlcontainer.WithDatabase("iam"),
		mysqlcontainer.WithUsername("iam"),
		mysqlcontainer.WithPassword("identity-test-password"),
	)
	if err != nil {
		t.Fatalf("启动 mysql:8.4 失败: %v", err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Logf("停止 MySQL 容器失败: %v", err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "parseTime=true&multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func openPostgreSQL16(t *testing.T) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	container, err := postgrescontainer.Run(ctx, "postgres:16",
		postgrescontainer.WithDatabase("iam"),
		postgrescontainer.WithUsername("iam"),
		postgrescontainer.WithPassword("identity-test-password"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("启动 postgres:16 失败: %v", err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Logf("停止 PostgreSQL 容器失败: %v", err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func assertBadKeyDoesNotTouchSchema(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	t.Setenv("IDENTITY_CREDENTIALS_KEYS", "")
	t.Setenv("IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION", "")
	t.Setenv("IDENTITY_CREDENTIALS_KEY", "not-base64")
	if err := RunGooseMigrations(db, driver, nil); err == nil || !strings.Contains(err.Error(), "凭据主密钥") {
		t.Fatalf("坏密钥应在 DDL 前阻断，实际错误: %v", err)
	}
	gormDB, err := openGormOnSQLDB(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	if gormDB.Migrator().HasTable("users") {
		t.Fatal("坏密钥阻断后不应创建 users 表")
	}
}

func setMigrationIntegrationKey(t *testing.T) {
	t.Helper()
	t.Setenv("IDENTITY_CREDENTIALS_KEYS", "")
	t.Setenv("IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION", "")
	t.Setenv("IDENTITY_CREDENTIALS_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
}

func prepareLegacyIdentitySchema(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	dialect := dialectMap[driver]
	dir := dirByDriver[driver]
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(FS)
	if err := goose.UpTo(db, dir, identityConnectionCreateVersion); err != nil {
		t.Fatalf("准备硬切前 schema 失败: %v", err)
	}
}

func seedLegacyIdentityBinding(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	gormDB, err := openGormOnSQLDB(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := gormDB.Exec("UPDATE social_providers SET client_id=?, client_secret=?, status=1 WHERE identifier=?", "github-client", "do-not-leak", "github").Error; err != nil {
		t.Fatal(err)
	}
	var providerID uint
	if err := gormDB.Raw("SELECT id FROM social_providers WHERE identifier=?", "github").Scan(&providerID).Error; err != nil || providerID == 0 {
		t.Fatalf("读取旧 provider 失败: id=%d err=%v", providerID, err)
	}
	if err := gormDB.Exec(`INSERT INTO user_social_accounts
		(user_id, provider_id, open_id, union_id, user_info, access_token, refresh_token, login_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		42, providerID, "github-subject", "github-union", `{"name":"Octo User"}`, "legacy-access-token", "legacy-refresh-token", 3).Error; err != nil {
		t.Fatal(err)
	}
}

func assertFinalMigrationSchema(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	gormDB, err := openGormOnSQLDB(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"passkeys", "custom_permissions", "custom_permission_grants", "two_factor_methods", "two_factor_recovery_codes", "two_factor_events", "login_sessions", "identity_connections", "external_principals", "external_principal_conflicts", "directory_sync_snapshot_objects", "directory_operation_leases"} {
		if !gormDB.Migrator().HasTable(table) {
			t.Fatalf("最终 schema 缺少表 %s", table)
		}
	}
	for _, oldTable := range []string{"social_providers", "identity_providers", "user_social_accounts", "user_identity_accounts"} {
		if gormDB.Migrator().HasTable(oldTable) {
			t.Fatalf("硬切后仍存在旧表 %s", oldTable)
		}
	}
	const expectedVersion int64 = 20261014010000
	version, err := goose.GetDBVersion(db)
	if err != nil || version != expectedVersion {
		t.Fatalf("迁移版本错误: got=%d want=%d err=%v", version, expectedVersion, err)
	}
	assertCustomPermissionSchema(t, gormDB)
	assertPasskeySchema(t, gormDB)
	assertActiveUserUniqueness(t, gormDB)
	assertOIDCRefreshTokenUniqueness(t, db, gormDB, driver)
}

func assertOIDCRefreshTokenUniqueness(t *testing.T, sqlDB *sql.DB, db *gorm.DB, driver string) {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())
	insert := `INSERT INTO oidc_tokens (access_token, access_token_hash, client_id, user_id, refresh_token) VALUES (?, ?, ?, 0, ?)`
	for i := 0; i < 2; i++ {
		access := fmt.Sprintf("machine-%s-%d", suffix, i)
		if err := db.Exec(insert, access, access, "machine-"+suffix, nil).Error; err != nil {
			t.Fatalf("多个机器令牌应允许 NULL refresh_token: %v", err)
		}
	}
	if err := db.Exec(insert, "refresh-a-"+suffix, "refresh-a-"+suffix, "machine-"+suffix, "duplicate-"+suffix).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(insert, "refresh-b-"+suffix, "refresh-b-"+suffix, "machine-"+suffix, "duplicate-"+suffix).Error; err == nil {
		t.Fatal("重复非空刷新令牌应被唯一索引拒绝")
	}
	if err := goose.Down(sqlDB, dirByDriver[driver]); err == nil {
		t.Fatal("不可逆迁移的 Down 应拒绝执行")
	}
	version, err := goose.GetDBVersion(sqlDB)
	if err != nil || version != 20261014010000 {
		t.Fatalf("拒绝 Down 后迁移版本应保持不变: version=%d err=%v", version, err)
	}
}

func assertActiveUserUniqueness(t *testing.T, db *gorm.DB) {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())
	username, email, phone := "unique-"+suffix, "unique-"+suffix+"@example.test", "+62"+suffix[len(suffix)-9:]
	insert := `INSERT INTO users (uuid, username, email, phone, status, is_activated) VALUES (?, ?, ?, ?, 1, TRUE)`
	if err := db.Exec(insert, "uuid-a-"+suffix, username, email, phone).Error; err != nil {
		t.Fatalf("写入首个活跃用户失败: %v", err)
	}
	if err := db.Exec(insert, "uuid-b-"+suffix, username, email, phone).Error; err == nil {
		t.Fatal("重复活跃 username/email/phone 应被唯一约束拒绝")
	}
	if err := db.Exec("UPDATE users SET deleted_at=CURRENT_TIMESTAMP WHERE uuid=?", "uuid-a-"+suffix).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(insert, "uuid-c-"+suffix, username, email, phone).Error; err != nil {
		t.Fatalf("软删除后应允许复用唯一字段: %v", err)
	}
}

func assertLegacyIdentityBindingMigrated(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	gormDB, err := openGormOnSQLDB(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	var principalCount int64
	if err := gormDB.Table("external_principals").Count(&principalCount).Error; err != nil || principalCount != 1 {
		t.Fatalf("旧绑定迁移数量错误: count=%d err=%v", principalCount, err)
	}
	var ciphertext string
	if err := gormDB.Table("identity_connection_credentials AS credential").
		Select("credential.ciphertext").
		Joins("JOIN identity_connections AS connection ON connection.id = credential.connection_id").
		Where("connection.identifier = ? AND credential.purpose = ?", "github", "authentication").
		Scan(&ciphertext).Error; err != nil {
		t.Fatal(err)
	}
	if ciphertext == "" || strings.Contains(ciphertext, "do-not-leak") {
		t.Fatal("旧凭据未正确加密迁移")
	}
}
