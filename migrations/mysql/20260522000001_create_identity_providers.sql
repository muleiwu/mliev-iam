-- +goose Up
-- +goose StatementBegin
CREATE TABLE `identity_providers` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(50) NOT NULL,
  `identifier` VARCHAR(50) NOT NULL,
  `type` VARCHAR(20) NOT NULL,
  `description` VARCHAR(255) NOT NULL DEFAULT '',
  `client_id` VARCHAR(255) NOT NULL,
  `client_secret` VARCHAR(255) NOT NULL,
  `auth_url` VARCHAR(255) NOT NULL DEFAULT '',
  `token_url` VARCHAR(255) NOT NULL DEFAULT '',
  `user_info_url` VARCHAR(255) NOT NULL DEFAULT '',
  `redirect_url` VARCHAR(255) NOT NULL,
  `scopes` TEXT,
  `icon` VARCHAR(255) NOT NULL DEFAULT '',
  `sort_order` INT NOT NULL DEFAULT 0,
  `status` INT NOT NULL DEFAULT 0,
  `is_default` BOOLEAN NOT NULL DEFAULT FALSE,
  `attributes` JSON NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `remark` TEXT,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_identity_providers_identifier` (`identifier`),
  KEY `idx_identity_providers_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `user_identity_accounts` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `external_user_id` VARCHAR(255) NOT NULL,
  `union_id` VARCHAR(255) NOT NULL DEFAULT '',
  `tenant_id` VARCHAR(255) NOT NULL DEFAULT '',
  `user_info` JSON NULL,
  `access_token` TEXT,
  `refresh_token` TEXT,
  `expires_at` DATETIME NULL,
  `token_type` VARCHAR(50) NOT NULL DEFAULT '',
  `last_used_at` DATETIME NULL,
  `login_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `attributes` JSON NULL,
  `remark` TEXT,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_identity_provider_external_user` (`provider_id`, `external_user_id`),
  KEY `idx_user_identity_provider` (`user_id`, `provider_id`),
  KEY `idx_user_identity_accounts_provider_id` (`provider_id`),
  KEY `idx_user_identity_accounts_union_id` (`union_id`),
  KEY `idx_user_identity_accounts_tenant_id` (`tenant_id`),
  KEY `idx_user_identity_accounts_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `user_identity_accounts`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `identity_providers`;
-- +goose StatementEnd
