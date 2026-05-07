-- +goose Up
-- +goose StatementBegin
CREATE TABLE captcha_configs (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  provider VARCHAR(50) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  params JSONB,
  remark VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE
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
