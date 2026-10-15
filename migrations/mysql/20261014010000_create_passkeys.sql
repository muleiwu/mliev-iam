-- +goose Up
CREATE TABLE passkeys (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 user_id BIGINT UNSIGNED NOT NULL,
 rp_id VARCHAR(253) NOT NULL,
 user_handle VARCHAR(64) NOT NULL,
 credential_id_hash VARCHAR(64) NOT NULL UNIQUE,
 credential_id TEXT NOT NULL,
 credential_data TEXT NOT NULL,
 name VARCHAR(100) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 last_used_at DATETIME(6) NULL
);
CREATE INDEX idx_passkeys_user_id ON passkeys(user_id);

-- Older administrator/directory updates could retain a previous address's
-- verification timestamp. An old enrolled receiver mismatch is evidence that
-- this timestamp must not automatically enroll the replacement address.
UPDATE users SET email_verified_at = NULL, two_factor_version = two_factor_version + 1
WHERE email_verified_at IS NOT NULL AND EXISTS (
 SELECT 1 FROM two_factor_methods f WHERE f.user_id = users.id AND f.method = 'email' AND CAST(f.receiver AS BINARY) <> CAST(COALESCE(users.email, '') AS BINARY)
);
UPDATE users SET phone_verified_at = NULL, two_factor_version = two_factor_version + 1
WHERE phone_verified_at IS NOT NULL AND EXISTS (
 SELECT 1 FROM two_factor_methods f WHERE f.user_id = users.id AND f.method = 'phone' AND CAST(f.receiver AS BINARY) <> CAST(COALESCE(users.phone, '') AS BINARY)
);

-- +goose Down
-- Credentials are login methods. Restore a coordinated backup to roll back.
SELECT 1 FROM passkey_rollback_requires_backup;
