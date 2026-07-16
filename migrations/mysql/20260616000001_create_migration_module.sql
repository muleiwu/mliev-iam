-- +goose Up
-- +goose StatementBegin
CREATE TABLE `migration_sources` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uuid` VARCHAR(36) NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `type` VARCHAR(20) NOT NULL,
  `enabled` BOOLEAN NOT NULL DEFAULT TRUE,
  `sort` INT NOT NULL DEFAULT 0,
  `connection_config` JSON NULL,
  `login_script` TEXT,
  `transform_script` TEXT,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `remark` TEXT,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_migration_sources_uuid` (`uuid`, `deleted_at`),
  KEY `idx_migration_sources_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `legacy_users` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `source_id` BIGINT UNSIGNED NOT NULL,
  `username` VARCHAR(100) NOT NULL,
  `external_id` VARCHAR(255) NOT NULL DEFAULT '',
  `email` VARCHAR(100) NULL,
  `phone` VARCHAR(20) NULL,
  `nickname` VARCHAR(100) NOT NULL DEFAULT '',
  `password_hash` VARCHAR(255) NOT NULL DEFAULT '',
  `salt` VARCHAR(64) NOT NULL DEFAULT '',
  `raw_data` JSON NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
  `migrated_user_id` BIGINT UNSIGNED NULL,
  `migrated_at` DATETIME NULL,
  `migration_remark` TEXT,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_legacy_users_source_username` (`source_id`, `username`, `deleted_at`),
  KEY `idx_legacy_users_source_id` (`source_id`),
  KEY `idx_legacy_users_username` (`username`),
  KEY `idx_legacy_users_email` (`email`),
  KEY `idx_legacy_users_phone` (`phone`),
  KEY `idx_legacy_users_external_id` (`external_id`),
  KEY `idx_legacy_users_status` (`status`),
  KEY `idx_legacy_users_migrated_user_id` (`migrated_user_id`),
  KEY `idx_legacy_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `migration_tasks` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `source_id` BIGINT UNSIGNED NOT NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
  `total` BIGINT NOT NULL DEFAULT 0,
  `processed` BIGINT NOT NULL DEFAULT 0,
  `succeeded` BIGINT NOT NULL DEFAULT 0,
  `failed` BIGINT NOT NULL DEFAULT 0,
  `error` TEXT,
  `started_at` DATETIME NULL,
  `finished_at` DATETIME NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  KEY `idx_migration_tasks_source_id` (`source_id`),
  KEY `idx_migration_tasks_status` (`status`),
  KEY `idx_migration_tasks_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `migration_tasks`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `legacy_users`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `migration_sources`;
-- +goose StatementEnd
