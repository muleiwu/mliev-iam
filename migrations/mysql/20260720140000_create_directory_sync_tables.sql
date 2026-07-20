-- +goose Up
CREATE TABLE `directory_sync_policies` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `connection_id` BIGINT UNSIGNED NOT NULL,
  `root_department_id` BIGINT UNSIGNED NOT NULL, `sync_scope_json` JSON NULL,
  `field_mapping_json` JSON NULL, `field_ownership_json` JSON NULL,
  `match_policy` VARCHAR(30) NOT NULL DEFAULT 'manual',
  `deprovision_policy` VARCHAR(60) NOT NULL DEFAULT 'block_identity_require_user_approval',
  `enabled` BOOLEAN NOT NULL DEFAULT FALSE, `created_at` DATETIME NULL, `updated_at` DATETIME NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_directory_policy_connection` (`connection_id`),
  UNIQUE KEY `uk_directory_policy_root` (`root_department_id`), KEY `idx_directory_policy_enabled` (`enabled`),
  CONSTRAINT `fk_directory_policy_connection` FOREIGN KEY (`connection_id`) REFERENCES `identity_connections` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `external_org_units` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `connection_id` BIGINT UNSIGNED NOT NULL,
  `external_id` VARCHAR(255) NOT NULL, `department_id` BIGINT UNSIGNED NULL,
  `external_parent_id` VARCHAR(255) NOT NULL DEFAULT '', `name` VARCHAR(100) NOT NULL,
  `sort_order` INT NOT NULL DEFAULT 0, `status` VARCHAR(20) NOT NULL DEFAULT 'active',
  `leader_external_id` VARCHAR(255) NOT NULL DEFAULT '', `snapshot_version` VARCHAR(64) NOT NULL,
  `last_seen_at` DATETIME NOT NULL, `raw_data` JSON NULL, `created_at` DATETIME NULL, `updated_at` DATETIME NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_external_org_unit` (`connection_id`,`external_id`),
  KEY `idx_external_org_units_department_id` (`department_id`), KEY `idx_external_org_units_parent` (`external_parent_id`),
  KEY `idx_external_org_units_snapshot` (`snapshot_version`), KEY `idx_external_org_units_seen` (`last_seen_at`),
  CONSTRAINT `fk_external_org_connection` FOREIGN KEY (`connection_id`) REFERENCES `identity_connections` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `external_memberships` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `connection_id` BIGINT UNSIGNED NOT NULL,
  `principal_id` BIGINT UNSIGNED NOT NULL, `external_org_unit_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NULL, `department_id` BIGINT UNSIGNED NULL,
  `is_primary` BOOLEAN NOT NULL DEFAULT FALSE, `is_leader` BOOLEAN NOT NULL DEFAULT FALSE,
  `position` VARCHAR(100) NOT NULL DEFAULT '', `status` VARCHAR(20) NOT NULL DEFAULT 'active',
  `snapshot_version` VARCHAR(64) NOT NULL, `last_seen_at` DATETIME NOT NULL, `raw_data` JSON NULL,
  `created_at` DATETIME NULL, `updated_at` DATETIME NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_external_membership` (`connection_id`,`principal_id`,`external_org_unit_id`),
  KEY `idx_external_memberships_principal` (`principal_id`), KEY `idx_external_memberships_org_unit` (`external_org_unit_id`),
  KEY `idx_external_memberships_user` (`user_id`), KEY `idx_external_memberships_department` (`department_id`),
  KEY `idx_external_memberships_snapshot` (`snapshot_version`), KEY `idx_external_memberships_seen` (`last_seen_at`),
  CONSTRAINT `fk_external_membership_connection` FOREIGN KEY (`connection_id`) REFERENCES `identity_connections` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_external_membership_principal` FOREIGN KEY (`principal_id`) REFERENCES `external_principals` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_external_membership_org` FOREIGN KEY (`external_org_unit_id`) REFERENCES `external_org_units` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `directory_sync_runs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `connection_id` BIGINT UNSIGNED NOT NULL,
  `snapshot_version` VARCHAR(64) NOT NULL, `trigger_type` VARCHAR(20) NOT NULL DEFAULT 'manual',
  `status` VARCHAR(30) NOT NULL DEFAULT 'pending', `total_users` INT NOT NULL DEFAULT 0,
  `total_org_units` INT NOT NULL DEFAULT 0, `total_memberships` INT NOT NULL DEFAULT 0,
  `change_count` INT NOT NULL DEFAULT 0, `applied_count` INT NOT NULL DEFAULT 0,
  `conflict_count` INT NOT NULL DEFAULT 0, `error_message` TEXT,
  `started_at` DATETIME NULL, `finished_at` DATETIME NULL, `created_by` BIGINT UNSIGNED NULL,
  `created_at` DATETIME NULL, `updated_at` DATETIME NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_directory_sync_runs_snapshot` (`snapshot_version`),
  KEY `idx_directory_sync_runs_connection` (`connection_id`), KEY `idx_directory_sync_runs_status` (`status`),
  KEY `idx_directory_sync_runs_created_by` (`created_by`),
  CONSTRAINT `fk_directory_run_connection` FOREIGN KEY (`connection_id`) REFERENCES `identity_connections` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `directory_sync_changes` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `run_id` BIGINT UNSIGNED NOT NULL,
  `connection_id` BIGINT UNSIGNED NOT NULL, `change_type` VARCHAR(20) NOT NULL,
  `object_type` VARCHAR(20) NOT NULL, `object_key` VARCHAR(255) NOT NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'pending', `risk_level` VARCHAR(20) NOT NULL DEFAULT 'low',
  `before_json` JSON NULL, `after_json` JSON NULL, `match_reason` VARCHAR(255) NOT NULL DEFAULT '',
  `error_message` TEXT, `reviewed_by` BIGINT UNSIGNED NULL, `reviewed_at` DATETIME NULL,
  `applied_at` DATETIME NULL, `created_at` DATETIME NULL, `updated_at` DATETIME NULL,
  PRIMARY KEY (`id`), KEY `idx_directory_changes_run` (`run_id`),
  KEY `idx_directory_changes_connection` (`connection_id`), KEY `idx_directory_changes_type` (`change_type`),
  KEY `idx_directory_changes_object_type` (`object_type`), KEY `idx_directory_changes_object_key` (`object_key`),
  KEY `idx_directory_changes_status` (`status`), KEY `idx_directory_changes_risk` (`risk_level`),
  KEY `idx_directory_changes_reviewed_by` (`reviewed_by`),
  CONSTRAINT `fk_directory_change_run` FOREIGN KEY (`run_id`) REFERENCES `directory_sync_runs` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_directory_change_connection` FOREIGN KEY (`connection_id`) REFERENCES `identity_connections` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS `directory_sync_changes`;
DROP TABLE IF EXISTS `directory_sync_runs`;
DROP TABLE IF EXISTS `external_memberships`;
DROP TABLE IF EXISTS `external_org_units`;
DROP TABLE IF EXISTS `directory_sync_policies`;
