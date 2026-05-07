-- +goose Up
-- +goose StatementBegin
CREATE TABLE `captcha_configs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(100) NOT NULL,
  `provider` VARCHAR(50) NOT NULL,
  `enabled` BOOLEAN NOT NULL DEFAULT TRUE,
  `params` JSON NULL,
  `remark` VARCHAR(255) NOT NULL DEFAULT '',
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  KEY `idx_captcha_configs_provider` (`provider`),
  KEY `idx_captcha_configs_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `captcha_configs`;
-- +goose StatementEnd
