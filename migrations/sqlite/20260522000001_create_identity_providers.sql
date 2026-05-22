-- +goose Up
-- +goose StatementBegin
CREATE TABLE identity_providers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(50) NOT NULL,
  identifier VARCHAR(50) NOT NULL,
  type VARCHAR(20) NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT '',
  client_id VARCHAR(255) NOT NULL,
  client_secret VARCHAR(255) NOT NULL,
  auth_url VARCHAR(255) NOT NULL DEFAULT '',
  token_url VARCHAR(255) NOT NULL DEFAULT '',
  user_info_url VARCHAR(255) NOT NULL DEFAULT '',
  redirect_url VARCHAR(255) NOT NULL,
  scopes TEXT,
  icon VARCHAR(255) NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 0,
  is_default INTEGER NOT NULL DEFAULT 0,
  attributes TEXT,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  remark TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_identity_providers_identifier ON identity_providers (identifier);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_identity_providers_deleted_at ON identity_providers (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE user_identity_accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  provider_id INTEGER NOT NULL,
  external_user_id VARCHAR(255) NOT NULL,
  union_id VARCHAR(255) NOT NULL DEFAULT '',
  tenant_id VARCHAR(255) NOT NULL DEFAULT '',
  user_info TEXT,
  access_token TEXT,
  refresh_token TEXT,
  expires_at DATETIME,
  token_type VARCHAR(50) NOT NULL DEFAULT '',
  last_used_at DATETIME,
  login_count INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  attributes TEXT,
  remark TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_identity_provider_external_user ON user_identity_accounts (provider_id, external_user_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_identity_provider ON user_identity_accounts (user_id, provider_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_identity_accounts_provider_id ON user_identity_accounts (provider_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_identity_accounts_union_id ON user_identity_accounts (union_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_identity_accounts_tenant_id ON user_identity_accounts (tenant_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_identity_accounts_deleted_at ON user_identity_accounts (deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_identity_accounts;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS identity_providers;
-- +goose StatementEnd
