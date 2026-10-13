-- +goose Up
ALTER TABLE users ADD COLUMN two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN two_factor_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE login_sessions ADD COLUMN two_factor_method VARCHAR(20) NOT NULL DEFAULT '';
ALTER TABLE login_sessions ADD COLUMN two_factor_at TIMESTAMPTZ NULL;
ALTER TABLE login_sessions ADD COLUMN two_factor_version BIGINT NOT NULL DEFAULT 0;
CREATE TABLE two_factor_methods (
 user_id BIGINT NOT NULL, method VARCHAR(20) NOT NULL,
 receiver VARCHAR(100) NOT NULL DEFAULT '', ciphertext TEXT, nonce VARCHAR(64), algorithm VARCHAR(32),
 key_version INTEGER NOT NULL DEFAULT 0, last_used_step BIGINT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY (user_id, method)
);
CREATE TABLE two_factor_recovery_codes (
 id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, code_hash VARCHAR(64) NOT NULL UNIQUE, used_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_two_factor_recovery_user ON two_factor_recovery_codes(user_id);
CREATE TABLE two_factor_events (
 id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, actor_id BIGINT NOT NULL, action VARCHAR(40) NOT NULL,
 method VARCHAR(20), reason VARCHAR(500), ip VARCHAR(64), created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_two_factor_events_user ON two_factor_events(user_id);

-- +goose Down
-- Downgrading would silently remove the second-factor requirement. Restore a
-- coordinated application/database backup instead of deleting enrolled factors.
SELECT 1 FROM two_factor_rollback_requires_backup;
