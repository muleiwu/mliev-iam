package migrations

import (
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestLoginSessionMigrationCutsOnlyLegacyUserCredentials(t *testing.T) {
	setIdentityCredentialsTestKey(t)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, RunGooseMigrations(db, "sqlite", nil))
	// Exercise the actual upgrade from the preceding schema, including Down
	// compatibility, rather than relying only on GORM-created fixture tables.
	require.NoError(t, goose.DownTo(db, "sqlite", 20260827000002))
	_, err = db.Exec(`INSERT INTO oidc_tokens (access_token, access_token_hash, client_id, user_id, refresh_token, is_revoked) VALUES
	 ('legacy-user', 'user-hash', 'client', 42, 'user-refresh', 0),
	 ('machine', 'machine-hash', 'client', 0, 'machine-unused', 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO authorization_codes (code, client_id, user_id, redirect_uri, is_used) VALUES ('pending', 'client', 42, 'https://rp.example/callback', 0)`)
	require.NoError(t, err)
	require.NoError(t, RunGooseMigrations(db, "sqlite", nil))
	var userRevoked, machineRevoked, codeUsed, sessions int
	require.NoError(t, db.QueryRow("SELECT is_revoked FROM oidc_tokens WHERE access_token_hash = 'user-hash'").Scan(&userRevoked))
	require.NoError(t, db.QueryRow("SELECT is_revoked FROM oidc_tokens WHERE access_token_hash = 'machine-hash'").Scan(&machineRevoked))
	require.NoError(t, db.QueryRow("SELECT is_used FROM authorization_codes WHERE code = 'pending'").Scan(&codeUsed))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM login_sessions").Scan(&sessions))
	require.Equal(t, 1, userRevoked)
	require.Zero(t, machineRevoked)
	require.Equal(t, 1, codeUsed)
	require.Zero(t, sessions, "old credentials must not create invented device records")
	require.NoError(t, RunGooseMigrations(db, "sqlite", nil))
}
