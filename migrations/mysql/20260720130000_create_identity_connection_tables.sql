-- +goose Up
-- +goose StatementBegin
CREATE TABLE `identity_connections` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(100) NOT NULL,
  `identifier` VARCHAR(80) NOT NULL,
  `category` VARCHAR(20) NOT NULL,
  `provider_type` VARCHAR(40) NOT NULL,
  `description` VARCHAR(255) NOT NULL DEFAULT '',
  `capabilities` JSON NOT NULL,
  `config_json` JSON NULL,
  `icon` VARCHAR(255) NOT NULL DEFAULT '',
  `sort_order` INT NOT NULL DEFAULT 0,
  `status` VARCHAR(20) NOT NULL DEFAULT 'disabled',
  `is_default` BOOLEAN NOT NULL DEFAULT FALSE,
  `last_checked_at` DATETIME NULL,
  `last_check_status` VARCHAR(20) NOT NULL DEFAULT '',
  `last_check_error` TEXT,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `remark` TEXT,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_identity_connections_identifier` (`identifier`),
  KEY `idx_identity_connections_category` (`category`),
  KEY `idx_identity_connections_provider_type` (`provider_type`),
  KEY `idx_identity_connections_status` (`status`),
  KEY `idx_identity_connections_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `identity_connection_credentials` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `connection_id` BIGINT UNSIGNED NOT NULL,
  `purpose` VARCHAR(30) NOT NULL,
  `ciphertext` TEXT NOT NULL,
  `nonce` VARCHAR(255) NOT NULL,
  `algorithm` VARCHAR(30) NOT NULL,
  `key_version` INT NOT NULL DEFAULT 1,
  `rotated_at` DATETIME NOT NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_connection_credential` (`connection_id`, `purpose`),
  CONSTRAINT `fk_connection_credentials_connection` FOREIGN KEY (`connection_id`) REFERENCES `identity_connections` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `external_principals` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `connection_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'unmatched',
  `display_name` VARCHAR(100) NOT NULL DEFAULT '',
  `avatar` VARCHAR(255) NOT NULL DEFAULT '',
  `email` VARCHAR(100) NOT NULL DEFAULT '',
  `phone` VARCHAR(30) NOT NULL DEFAULT '',
  `raw_profile` JSON NULL,
  `last_login_at` DATETIME NULL,
  `last_synced_at` DATETIME NULL,
  `login_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_principal_connection_user` (`connection_id`, `user_id`),
  KEY `idx_external_principals_connection_id` (`connection_id`),
  KEY `idx_external_principals_user_id` (`user_id`),
  KEY `idx_external_principals_status` (`status`),
  KEY `idx_external_principals_email` (`email`),
  KEY `idx_external_principals_phone` (`phone`),
  KEY `idx_external_principals_deleted_at` (`deleted_at`),
  CONSTRAINT `fk_external_principals_connection` FOREIGN KEY (`connection_id`) REFERENCES `identity_connections` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `external_identifiers` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `connection_id` BIGINT UNSIGNED NOT NULL,
  `principal_id` BIGINT UNSIGNED NOT NULL,
  `namespace` VARCHAR(40) NOT NULL,
  `value` VARCHAR(255) NOT NULL,
  `is_primary` BOOLEAN NOT NULL DEFAULT FALSE,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_external_identifier` (`connection_id`, `namespace`, `value`),
  KEY `idx_external_identifiers_principal_id` (`principal_id`),
  CONSTRAINT `fk_external_identifiers_connection` FOREIGN KEY (`connection_id`) REFERENCES `identity_connections` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_external_identifiers_principal` FOREIGN KEY (`principal_id`) REFERENCES `external_principals` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `external_identifiers`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `external_principals`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `identity_connection_credentials`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `identity_connections`;
-- +goose StatementEnd
