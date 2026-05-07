-- +goose Up
-- +goose StatementBegin
CREATE TABLE captcha_configs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(100) NOT NULL,
  provider VARCHAR(50) NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  params TEXT,
  remark VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_captcha_configs_provider ON captcha_configs (provider);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_captcha_configs_deleted_at ON captcha_configs (deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS captcha_configs;
-- +goose StatementEnd
