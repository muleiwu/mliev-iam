-- +goose Up
-- client_credentials grants have no refresh token. A unique empty string lets
-- only one machine token exist across all clients; NULL retains uniqueness for
-- real refresh tokens without limiting machine tokens.
ALTER TABLE `oidc_tokens` MODIFY COLUMN `refresh_token` VARCHAR(255) NULL DEFAULT NULL;
UPDATE `oidc_tokens` SET `refresh_token` = NULL WHERE `refresh_token` = '';

-- +goose Down
-- The old unique NOT NULL column cannot represent multiple machine tokens.
-- Restore a database backup to reverse this migration without losing tokens.
SELECT 1 FROM oidc_refresh_token_rollback_requires_backup;
