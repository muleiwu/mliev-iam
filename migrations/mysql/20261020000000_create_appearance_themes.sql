-- +goose Up
-- +goose StatementBegin
CREATE TABLE appearance_themes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  uuid VARCHAR(36) NOT NULL,
  identity_key VARCHAR(64) NOT NULL,
  name VARCHAR(100) NOT NULL,
  version VARCHAR(32) NOT NULL,
  author VARCHAR(100) NOT NULL DEFAULT '',
  description VARCHAR(1000) NOT NULL DEFAULT '',
  format_version INTEGER NOT NULL,
  appearance_json TEXT NOT NULL,
  created_at DATETIME NULL,
  CONSTRAINT uk_appearance_themes_uuid UNIQUE (uuid),
  CONSTRAINT uk_appearance_themes_identity UNIQUE (identity_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS appearance_themes;
-- +goose StatementEnd
