-- +goose Up
CREATE TABLE we_chat_identities (
 hash VARCHAR(64) PRIMARY KEY, kind VARCHAR(16) NOT NULL,
 scope VARCHAR(128) NOT NULL, subject VARCHAR(255) NOT NULL,
 user_id BIGINT NOT NULL, created_at DATETIME(3) NOT NULL
);
CREATE INDEX idx_we_chat_identities_user_id ON we_chat_identities(user_id);
CREATE TABLE we_chat_identity_conflicts (
 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, connection_id BIGINT NOT NULL,
 identity_hash VARCHAR(64) NOT NULL, created_at DATETIME(3) NOT NULL
);
CREATE INDEX idx_we_chat_identity_conflicts_connection_id ON we_chat_identity_conflicts(connection_id);
ALTER TABLE login_sessions ADD COLUMN native_transaction_hash VARCHAR(64) NULL;
CREATE UNIQUE INDEX uk_login_native_transaction ON login_sessions(native_transaction_hash);
-- Session keys are not profile data. Existing identifiers and bindings stay intact.
UPDATE external_principals SET raw_profile = JSON_REMOVE(raw_profile, '$.session_key') WHERE raw_profile IS NOT NULL;

-- +goose Down
DROP INDEX uk_login_native_transaction ON login_sessions;
ALTER TABLE login_sessions DROP COLUMN native_transaction_hash;
DROP TABLE we_chat_identity_conflicts;
DROP TABLE we_chat_identities;
