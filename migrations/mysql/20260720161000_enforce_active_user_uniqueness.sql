-- +goose Up
ALTER TABLE `users`
  DROP INDEX `idx_users_uuid`,
  DROP INDEX `uk_users_username`,
  DROP INDEX `uk_users_email`,
  DROP INDEX `uk_users_phone`,
  ADD COLUMN `active_uuid` VARCHAR(36) GENERATED ALWAYS AS (IF(`deleted_at` IS NULL, `uuid`, NULL)) STORED,
  ADD COLUMN `active_username` VARCHAR(50) GENERATED ALWAYS AS (IF(`deleted_at` IS NULL, `username`, NULL)) STORED,
  ADD COLUMN `active_email` VARCHAR(100) GENERATED ALWAYS AS (IF(`deleted_at` IS NULL, `email`, NULL)) STORED,
  ADD COLUMN `active_phone` VARCHAR(20) GENERATED ALWAYS AS (IF(`deleted_at` IS NULL, `phone`, NULL)) STORED,
  ADD UNIQUE INDEX `uk_users_active_uuid` (`active_uuid`),
  ADD UNIQUE INDEX `uk_users_active_username` (`active_username`),
  ADD UNIQUE INDEX `uk_users_active_email` (`active_email`),
  ADD UNIQUE INDEX `uk_users_active_phone` (`active_phone`);

-- +goose Down
ALTER TABLE `users`
  DROP INDEX `uk_users_active_uuid`,
  DROP INDEX `uk_users_active_username`,
  DROP INDEX `uk_users_active_email`,
  DROP INDEX `uk_users_active_phone`,
  DROP COLUMN `active_uuid`,
  DROP COLUMN `active_username`,
  DROP COLUMN `active_email`,
  DROP COLUMN `active_phone`,
  ADD UNIQUE INDEX `idx_users_uuid` (`uuid`),
  ADD UNIQUE INDEX `uk_users_username` (`username`, `deleted_at`),
  ADD UNIQUE INDEX `uk_users_email` (`email`, `deleted_at`),
  ADD UNIQUE INDEX `uk_users_phone` (`phone`, `deleted_at`);
