-- +goose Up
-- +goose StatementBegin
CREATE TABLE `departments` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(100) NOT NULL,
  `code` VARCHAR(50) NOT NULL,
  `short_name` VARCHAR(50) NOT NULL DEFAULT '',
  `description` VARCHAR(255) NOT NULL DEFAULT '',
  `type` INT NOT NULL DEFAULT 2,
  `status` INT NOT NULL DEFAULT 1,
  `parent_id` BIGINT UNSIGNED NULL,
  `level` INT NOT NULL DEFAULT 1,
  `path` VARCHAR(500) NOT NULL DEFAULT '',
  `sort` INT NOT NULL DEFAULT 0,
  `manager_id` BIGINT UNSIGNED NULL,
  `phone` VARCHAR(20) NOT NULL DEFAULT '',
  `email` VARCHAR(100) NOT NULL DEFAULT '',
  `address` VARCHAR(255) NOT NULL DEFAULT '',
  `postal_code` VARCHAR(20) NOT NULL DEFAULT '',
  `fax` VARCHAR(20) NOT NULL DEFAULT '',
  `website` VARCHAR(255) NOT NULL DEFAULT '',
  `location` VARCHAR(255) NOT NULL DEFAULT '',
  `cost_center` VARCHAR(50) NOT NULL DEFAULT '',
  `budget_code` VARCHAR(50) NOT NULL DEFAULT '',
  `tax_id` VARCHAR(50) NOT NULL DEFAULT '',
  `legal_entity` VARCHAR(100) NOT NULL DEFAULT '',
  `business_type` VARCHAR(100) NOT NULL DEFAULT '',
  `data_scope` INT NOT NULL DEFAULT 1,
  `data_scope_desc` VARCHAR(255) NOT NULL DEFAULT '',
  `is_leaf` BOOLEAN NOT NULL DEFAULT TRUE,
  `has_children` BOOLEAN NOT NULL DEFAULT FALSE,
  `established_at` DATETIME NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `attributes` JSON NULL,
  `remark` TEXT,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_departments_code` (`code`),
  KEY `idx_departments_parent_id` (`parent_id`),
  KEY `idx_departments_manager_id` (`manager_id`),
  KEY `idx_departments_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `roles` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(50) NOT NULL,
  `code` VARCHAR(50) NOT NULL,
  `description` VARCHAR(255) NOT NULL DEFAULT '',
  `type` INT NOT NULL DEFAULT 1,
  `status` INT NOT NULL DEFAULT 1,
  `parent_id` BIGINT UNSIGNED NULL,
  `level` INT NOT NULL DEFAULT 1,
  `path` VARCHAR(500) NOT NULL DEFAULT '',
  `sort` INT NOT NULL DEFAULT 0,
  `data_scope` INT NOT NULL DEFAULT 1,
  `scope_desc` VARCHAR(255) NOT NULL DEFAULT '',
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `attributes` JSON NULL,
  `remark` TEXT,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_roles_name` (`name`, `deleted_at`),
  UNIQUE KEY `uk_roles_code` (`code`, `deleted_at`),
  KEY `idx_roles_parent_id` (`parent_id`),
  KEY `idx_roles_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `permissions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(100) NOT NULL,
  `code` VARCHAR(100) NOT NULL,
  `description` VARCHAR(255) NOT NULL DEFAULT '',
  `type` INT NOT NULL DEFAULT 0,
  `status` INT NOT NULL DEFAULT 1,
  `parent_id` BIGINT UNSIGNED NULL,
  `level` INT NOT NULL DEFAULT 1,
  `path` VARCHAR(500) NOT NULL DEFAULT '',
  `sort` INT NOT NULL DEFAULT 0,
  `resource` VARCHAR(255) NOT NULL DEFAULT '',
  `action` VARCHAR(100) NOT NULL DEFAULT '',
  `method` VARCHAR(20) NOT NULL DEFAULT '',
  `url` VARCHAR(255) NOT NULL DEFAULT '',
  `component` VARCHAR(255) NOT NULL DEFAULT '',
  `icon` VARCHAR(100) NOT NULL DEFAULT '',
  `redirect` VARCHAR(255) NOT NULL DEFAULT '',
  `external` BOOLEAN NOT NULL DEFAULT FALSE,
  `hidden` BOOLEAN NOT NULL DEFAULT FALSE,
  `keep_alive` BOOLEAN NOT NULL DEFAULT FALSE,
  `show_in_menu` BOOLEAN NOT NULL DEFAULT TRUE,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `attributes` JSON NULL,
  `remark` TEXT,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_permissions_code` (`code`),
  KEY `idx_permissions_parent_id` (`parent_id`),
  KEY `idx_permissions_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `users` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `username` VARCHAR(50) NOT NULL,
  `email` VARCHAR(100) NULL,
  `phone` VARCHAR(20) NULL,
  `nickname` VARCHAR(50) NOT NULL DEFAULT '',
  `real_name` VARCHAR(50) NOT NULL DEFAULT '',
  `avatar` VARCHAR(255) NOT NULL DEFAULT '',
  `salt` VARCHAR(32) NOT NULL DEFAULT '',
  `password_hash` VARCHAR(100) NOT NULL DEFAULT '',
  `status` INT NOT NULL DEFAULT 0,
  `is_activated` BOOLEAN NOT NULL DEFAULT FALSE,
  `is_verified` BOOLEAN NOT NULL DEFAULT FALSE,
  `login_count` BIGINT NOT NULL DEFAULT 0,
  `last_login_at` DATETIME NULL,
  `last_login_ip` VARCHAR(45) NOT NULL DEFAULT '',
  `password_expiry` DATETIME NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `attributes` JSON NULL,
  `remark` TEXT,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_users_username` (`username`, `deleted_at`),
  UNIQUE KEY `uk_users_email` (`email`, `deleted_at`),
  UNIQUE KEY `uk_users_phone` (`phone`, `deleted_at`),
  KEY `idx_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `user_departments` (
  `user_id` BIGINT UNSIGNED NOT NULL,
  `department_id` BIGINT UNSIGNED NOT NULL,
  `position` VARCHAR(100) NOT NULL DEFAULT '',
  `title` VARCHAR(100) NOT NULL DEFAULT '',
  `type` INT NOT NULL DEFAULT 0,
  `is_active` BOOLEAN NOT NULL DEFAULT TRUE,
  `is_primary` BOOLEAN NOT NULL DEFAULT FALSE,
  `sort` INT NOT NULL DEFAULT 0,
  `joined_at` DATETIME NULL,
  `left_at` DATETIME NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `attributes` JSON NULL,
  `remark` TEXT,
  PRIMARY KEY (`user_id`, `department_id`),
  KEY `idx_user_departments_user_id` (`user_id`),
  KEY `idx_user_departments_department_id` (`department_id`),
  KEY `idx_user_departments_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `user_roles` (
  `user_id` BIGINT UNSIGNED NOT NULL,
  `role_id` BIGINT UNSIGNED NOT NULL,
  `assigned_by` BIGINT UNSIGNED NOT NULL,
  `assigned_at` DATETIME NOT NULL,
  `expires_at` DATETIME NULL,
  `constraints` JSON NULL,
  `data_scope` INT NOT NULL DEFAULT 1,
  `is_active` BOOLEAN NOT NULL DEFAULT TRUE,
  `is_primary` BOOLEAN NOT NULL DEFAULT FALSE,
  `department_id` BIGINT UNSIGNED NULL,
  `organization_id` BIGINT UNSIGNED NULL,
  `apply_scope` INT NOT NULL DEFAULT 1,
  `approval_status` INT NOT NULL DEFAULT 1,
  `approved_by` BIGINT UNSIGNED NULL,
  `approved_at` DATETIME NULL,
  `approval_remark` TEXT,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`user_id`, `role_id`),
  KEY `idx_user_roles_user_id` (`user_id`),
  KEY `idx_user_roles_role_id` (`role_id`),
  KEY `idx_user_roles_department_id` (`department_id`),
  KEY `idx_user_roles_organization_id` (`organization_id`),
  KEY `idx_user_roles_approved_by` (`approved_by`),
  KEY `idx_user_roles_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `role_permissions` (
  `role_id` BIGINT UNSIGNED NOT NULL,
  `permission_id` BIGINT UNSIGNED NOT NULL,
  `granted_by` BIGINT UNSIGNED NOT NULL,
  `granted_at` DATETIME NOT NULL,
  `expires_at` DATETIME NULL,
  `conditions` JSON NULL,
  `data_scope` INT NOT NULL DEFAULT 1,
  `is_active` BOOLEAN NOT NULL DEFAULT TRUE,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`role_id`, `permission_id`),
  KEY `idx_role_permissions_role_id` (`role_id`),
  KEY `idx_role_permissions_permission_id` (`permission_id`),
  KEY `idx_role_permissions_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `applications` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uuid` VARCHAR(36) NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `description` TEXT,
  `protocol_type` VARCHAR(20) NOT NULL,
  `protocol_config` JSON NOT NULL,
  `is_active` BOOLEAN NOT NULL DEFAULT TRUE,
  `is_verified` BOOLEAN NOT NULL DEFAULT FALSE,
  `access_scope` JSON NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `created_by_user_id` BIGINT UNSIGNED NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_applications_uuid` (`uuid`, `deleted_at`),
  KEY `idx_applications_protocol_type` (`protocol_type`),
  KEY `idx_applications_created_by_user_id` (`created_by_user_id`),
  KEY `idx_applications_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `application_oidc_keys` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `application_id` BIGINT UNSIGNED NOT NULL,
  `key_id` VARCHAR(100) NOT NULL,
  `algorithm` VARCHAR(20) NOT NULL,
  `key_type` VARCHAR(20) NOT NULL,
  `private_key` TEXT NOT NULL,
  `public_key` TEXT,
  `key_size` INT NOT NULL DEFAULT 0,
  `is_primary` BOOLEAN NOT NULL DEFAULT FALSE,
  `expires_at` DATETIME NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_application_oidc_keys_key_id` (`key_id`),
  KEY `idx_app_keys` (`application_id`, `is_primary`),
  KEY `idx_application_oidc_keys_expires_at` (`expires_at`),
  KEY `idx_application_oidc_keys_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `authorization_codes` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(100) NOT NULL,
  `client_id` VARCHAR(100) NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `redirect_uri` TEXT NOT NULL,
  `scopes` JSON NULL,
  `state` TEXT,
  `nonce` TEXT,
  `code_challenge` VARCHAR(128) NOT NULL DEFAULT '',
  `code_challenge_method` VARCHAR(10) NOT NULL DEFAULT '',
  `is_used` BOOLEAN NOT NULL DEFAULT FALSE,
  `expires_at` DATETIME NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_authorization_codes_code` (`code`),
  KEY `idx_authorization_codes_client_id` (`client_id`),
  KEY `idx_authorization_codes_user_id` (`user_id`),
  KEY `idx_authorization_codes_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `oidc_tokens` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `access_token` TEXT NOT NULL,
  `access_token_hash` VARCHAR(64) NOT NULL,
  `token_type` VARCHAR(20) NOT NULL DEFAULT 'Bearer',
  `client_id` VARCHAR(100) NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `scopes` JSON NULL,
  `refresh_token` VARCHAR(255) NOT NULL DEFAULT '',
  `id_token` TEXT,
  `expires_at` DATETIME NULL,
  `refresh_expires_at` DATETIME NULL,
  `is_revoked` BOOLEAN NOT NULL DEFAULT FALSE,
  `authorization_code_id` BIGINT UNSIGNED NULL,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  `attributes` JSON NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_oidc_tokens_access_token_hash` (`access_token_hash`),
  UNIQUE KEY `uk_oidc_tokens_refresh_token` (`refresh_token`),
  KEY `idx_oidc_tokens_client_id` (`client_id`),
  KEY `idx_oidc_tokens_user_id` (`user_id`),
  KEY `idx_oidc_tokens_authorization_code_id` (`authorization_code_id`),
  KEY `idx_oidc_tokens_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `social_providers` (
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
  `redirect_url` VARCHAR(255) NOT NULL DEFAULT '',
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
  UNIQUE KEY `uk_social_providers_identifier` (`identifier`),
  KEY `idx_social_providers_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `user_social_accounts` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `open_id` VARCHAR(255) NOT NULL,
  `union_id` VARCHAR(255) NOT NULL DEFAULT '',
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
  UNIQUE KEY `idx_provider_openid` (`provider_id`, `open_id`),
  KEY `idx_user_provider` (`user_id`, `provider_id`),
  KEY `idx_user_social_accounts_provider_id` (`provider_id`),
  KEY `idx_user_social_accounts_union_id` (`union_id`),
  KEY `idx_user_social_accounts_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `settings` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `setting_key` VARCHAR(100) NOT NULL,
  `value` TEXT,
  `category` VARCHAR(50) NOT NULL DEFAULT '',
  `remark` VARCHAR(255) NOT NULL DEFAULT '',
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_settings_setting_key` (`setting_key`),
  KEY `idx_settings_category` (`category`),
  KEY `idx_settings_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `settings`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `user_social_accounts`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `social_providers`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `oidc_tokens`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `authorization_codes`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `application_oidc_keys`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `applications`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `role_permissions`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `user_roles`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `user_departments`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `users`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `permissions`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `roles`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `departments`;
-- +goose StatementEnd
