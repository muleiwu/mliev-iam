package middleware

import (
	"strings"
	"testing"
	"time"

	internaljwt "cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openUserSessionTestDB(t *testing.T, withRevocationColumn bool) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}

	createSQL := `CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		status INTEGER NOT NULL,
		is_activated INTEGER NOT NULL,
		deleted_at DATETIME`
	if withRevocationColumn {
		createSQL += ", session_revoked_at DATETIME"
	}
	createSQL += ")"
	if err := database.Exec(createSQL).Error; err != nil {
		t.Fatalf("create users table: %v", err)
	}

	return database
}

func issuedClaims(userID uint, issuedAt time.Time) *internaljwt.Claims {
	return &internaljwt.Claims{
		UserID: userID,
		RegisteredClaims: jwtlib.RegisteredClaims{
			IssuedAt: jwtlib.NewNumericDate(issuedAt),
		},
	}
}

func TestValidateUserSessionSupportsSchemaBeforeRevocationColumn(t *testing.T) {
	database := openUserSessionTestDB(t, false)
	if err := database.Exec(
		"INSERT INTO users (id, status, is_activated) VALUES (?, ?, ?)",
		1, 1, true,
	).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := validateUserSessionWithDB(database, issuedClaims(1, time.Now())); err != nil {
		t.Fatalf("active user on pre-migration schema should be accepted: %v", err)
	}
}

func TestValidateUserSessionStillRejectsRevokedSession(t *testing.T) {
	database := openUserSessionTestDB(t, true)
	revokedAt := time.Now()
	if err := database.Exec(
		"INSERT INTO users (id, status, is_activated, session_revoked_at) VALUES (?, ?, ?, ?)",
		1, 1, true, revokedAt,
	).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	err := validateUserSessionWithDB(database, issuedClaims(1, revokedAt.Add(-time.Minute)))
	if err == nil || !strings.Contains(err.Error(), "会话已撤销") {
		t.Fatalf("revoked session should be rejected, got: %v", err)
	}
}

func TestValidateUserSessionDoesNotReportDatabaseFailureAsMissingUser(t *testing.T) {
	database := openUserSessionTestDB(t, false)
	if err := database.Exec("DROP TABLE users").Error; err != nil {
		t.Fatalf("drop users table: %v", err)
	}

	err := validateUserSessionWithDB(database, issuedClaims(1, time.Now()))
	if err == nil || err.Error() != "用户状态校验失败" {
		t.Fatalf("database failure should have a distinct error, got: %v", err)
	}
}
