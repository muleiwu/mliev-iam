-- +goose Up
-- +goose StatementBegin
CREATE TABLE `attachments` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uuid` VARCHAR(36) NOT NULL,
  `file_name` VARCHAR(255) NOT NULL,
  `file_size` BIGINT NOT NULL DEFAULT 0,
  `mime_type` VARCHAR(100) NOT NULL,
  `storage_type` VARCHAR(20) NOT NULL DEFAULT 's3',
  `storage_key` VARCHAR(500) NOT NULL,
  `biz_type` VARCHAR(50) NOT NULL DEFAULT 'avatar',
  `uploader_user_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  `deleted_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_attachments_uuid` (`uuid`),
  KEY `idx_attachments_uploader` (`uploader_user_id`),
  KEY `idx_attachments_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `attachments`;
-- +goose StatementEnd
