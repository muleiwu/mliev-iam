-- +goose Up
-- +goose StatementBegin
CREATE TABLE identity_providers (
  id BIGSERIAL PRIMARY KEY,
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
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  attributes JSONB,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
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
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  provider_id BIGINT NOT NULL,
  external_user_id VARCHAR(255) NOT NULL,
  union_id VARCHAR(255) NOT NULL DEFAULT '',
  tenant_id VARCHAR(255) NOT NULL DEFAULT '',
  user_info JSONB,
  access_token TEXT,
  refresh_token TEXT,
  expires_at TIMESTAMP WITH TIME ZONE,
  token_type VARCHAR(50) NOT NULL DEFAULT '',
  last_used_at TIMESTAMP WITH TIME ZONE,
  login_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  attributes JSONB,
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
