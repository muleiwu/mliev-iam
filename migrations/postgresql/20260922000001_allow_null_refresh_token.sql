-- +goose Up
-- Dropping NOT NULL alone leaves the old empty-string default in place.
ALTER TABLE oidc_tokens
  ALTER COLUMN refresh_token DROP NOT NULL,
  ALTER COLUMN refresh_token DROP DEFAULT;
UPDATE oidc_tokens SET refresh_token = NULL WHERE refresh_token = '';

-- +goose Down
-- The old unique NOT NULL column cannot represent multiple machine tokens.
-- Restore a database backup to reverse this migration without losing tokens.
SELECT 1 FROM oidc_refresh_token_rollback_requires_backup;
