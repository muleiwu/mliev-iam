-- +goose Up
-- SQLite cannot remove a NOT NULL constraint without rebuilding the table.
-- Keep the highest allocated id even when the highest rows were deleted.
CREATE TEMP TABLE oidc_tokens_saved_sequence AS
SELECT seq FROM sqlite_sequence WHERE name = 'oidc_tokens';

CREATE TABLE oidc_tokens_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  access_token TEXT NOT NULL,
  access_token_hash VARCHAR(64) NOT NULL,
  token_type VARCHAR(20) NOT NULL DEFAULT 'Bearer',
  client_id VARCHAR(100) NOT NULL,
  user_id INTEGER NOT NULL,
  scopes TEXT,
  refresh_token VARCHAR(255) DEFAULT NULL,
  id_token TEXT,
  expires_at DATETIME,
  refresh_expires_at DATETIME,
  is_revoked INTEGER NOT NULL DEFAULT 0,
  authorization_code_id INTEGER,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  attributes TEXT,
  login_session_id VARCHAR(36) NOT NULL DEFAULT ''
);

INSERT INTO oidc_tokens_new (
  id, access_token, access_token_hash, token_type, client_id, user_id, scopes,
  refresh_token, id_token, expires_at, refresh_expires_at, is_revoked,
  authorization_code_id, created_at, updated_at, deleted_at, attributes,
  login_session_id
)
SELECT
  id, access_token, access_token_hash, token_type, client_id, user_id, scopes,
  NULLIF(refresh_token, ''), id_token, expires_at, refresh_expires_at,
  is_revoked, authorization_code_id, created_at, updated_at, deleted_at,
  attributes, login_session_id
FROM oidc_tokens;

DROP TABLE oidc_tokens;
ALTER TABLE oidc_tokens_new RENAME TO oidc_tokens;

UPDATE sqlite_sequence
SET seq = MAX(seq, (SELECT seq FROM oidc_tokens_saved_sequence))
WHERE name = 'oidc_tokens' AND EXISTS (SELECT 1 FROM oidc_tokens_saved_sequence);
INSERT INTO sqlite_sequence (name, seq)
SELECT 'oidc_tokens', seq FROM oidc_tokens_saved_sequence
WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'oidc_tokens');
DROP TABLE oidc_tokens_saved_sequence;

CREATE UNIQUE INDEX uk_oidc_tokens_access_token_hash ON oidc_tokens (access_token_hash);
CREATE UNIQUE INDEX uk_oidc_tokens_refresh_token ON oidc_tokens (refresh_token);
CREATE INDEX idx_oidc_tokens_client_id ON oidc_tokens (client_id);
CREATE INDEX idx_oidc_tokens_user_id ON oidc_tokens (user_id);
CREATE INDEX idx_oidc_tokens_authorization_code_id ON oidc_tokens (authorization_code_id);
CREATE INDEX idx_oidc_tokens_deleted_at ON oidc_tokens (deleted_at);
CREATE INDEX idx_oidc_token_login_session ON oidc_tokens (login_session_id);

-- +goose Down
-- The old unique NOT NULL column cannot represent multiple machine tokens.
-- Restore a database backup to reverse this migration without losing tokens.
SELECT 1 FROM oidc_refresh_token_rollback_requires_backup;
