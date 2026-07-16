-- +goose Up
-- +goose StatementBegin
CREATE TABLE `agreements` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `scope` VARCHAR(20) NOT NULL,
  `application_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `code` VARCHAR(100) NOT NULL,
  `title` VARCHAR(200) NOT NULL,
  `content_md` LONGTEXT NOT NULL,
  `enabled` BOOLEAN NOT NULL DEFAULT FALSE,
  `sort` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_agreements_scope_app_code` (`scope`, `application_id`, `code`, `deleted_at`),
  KEY `idx_agreements_scope_app_enabled` (`scope`, `application_id`, `enabled`),
  KEY `idx_agreements_sort` (`sort`),
  KEY `idx_agreements_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `agreement_acceptances` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `application_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `agreement_id` BIGINT UNSIGNED NOT NULL,
  `scope` VARCHAR(20) NOT NULL,
  `code` VARCHAR(100) NOT NULL,
  `title` VARCHAR(200) NOT NULL,
  `content_sha256` VARCHAR(64) NOT NULL,
  `accepted_at` DATETIME NOT NULL,
  `created_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agreement_acceptances_user_id` (`user_id`),
  KEY `idx_agreement_acceptances_application_id` (`application_id`),
  KEY `idx_agreement_acceptances_agreement_id` (`agreement_id`),
  KEY `idx_agreement_acceptances_scope_code` (`scope`, `code`),
  KEY `idx_agreement_acceptances_accepted_at` (`accepted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `agreements` (`scope`, `application_id`, `code`, `title`, `content_md`, `enabled`, `sort`, `created_at`, `updated_at`)
VALUES
  ('global', 0, 'terms', '服务协议', '', FALSE, 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
  ('global', 0, 'privacy', '隐私协议', '', FALSE, 20, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `agreement_acceptances`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `agreements`;
-- +goose StatementEnd
