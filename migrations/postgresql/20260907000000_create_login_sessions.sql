-- +goose Up
CREATE TABLE login_sessions (
 id VARCHAR(36) PRIMARY KEY, user_id BIGINT NOT NULL,
 device VARCHAR(32) NOT NULL, browser VARCHAR(64) NOT NULL, os VARCHAR(64) NOT NULL,
 login_method VARCHAR(64) NOT NULL, login_ip VARCHAR(64) NOT NULL, last_ip VARCHAR(64) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL, last_active_at TIMESTAMPTZ NOT NULL, expires_at TIMESTAMPTZ NOT NULL,
 has_admin_access BOOLEAN NOT NULL DEFAULT FALSE,
 ended_at TIMESTAMPTZ NULL, end_reason VARCHAR(64) NOT NULL DEFAULT '', ended_by BIGINT NULL
);
CREATE INDEX idx_login_sessions_user_activity ON login_sessions(user_id, last_active_at);
CREATE INDEX idx_login_sessions_expires ON login_sessions(expires_at);
CREATE INDEX idx_login_sessions_ended ON login_sessions(ended_at);
ALTER TABLE authorization_codes ADD COLUMN login_session_id VARCHAR(36) NOT NULL DEFAULT '';
ALTER TABLE oidc_tokens ADD COLUMN login_session_id VARCHAR(36) NOT NULL DEFAULT '';
CREATE INDEX idx_auth_code_login_session ON authorization_codes(login_session_id);
CREATE INDEX idx_oidc_token_login_session ON oidc_tokens(login_session_id);
-- Old user credentials cannot be attributed to a browser. Machine grants stay valid.
UPDATE oidc_tokens SET is_revoked = TRUE WHERE user_id > 0;
UPDATE authorization_codes SET is_used = TRUE;

-- +goose Down
DROP INDEX idx_oidc_token_login_session;
DROP INDEX idx_auth_code_login_session;
ALTER TABLE oidc_tokens DROP COLUMN login_session_id;
ALTER TABLE authorization_codes DROP COLUMN login_session_id;
DROP TABLE login_sessions;
