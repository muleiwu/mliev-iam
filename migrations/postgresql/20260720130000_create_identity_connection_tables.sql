-- +goose Up
-- +goose StatementBegin
CREATE TABLE identity_connections (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  identifier VARCHAR(80) NOT NULL,
  category VARCHAR(20) NOT NULL,
  provider_type VARCHAR(40) NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT '',
  capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
  config_json JSONB,
  icon VARCHAR(255) NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(20) NOT NULL DEFAULT 'disabled',
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  last_checked_at TIMESTAMP WITH TIME ZONE,
  last_check_status VARCHAR(20) NOT NULL DEFAULT '',
  last_check_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  remark TEXT NOT NULL DEFAULT ''
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_identity_connections_identifier ON identity_connections (identifier);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_identity_connections_category ON identity_connections (category);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_identity_connections_provider_type ON identity_connections (provider_type);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_identity_connections_status ON identity_connections (status);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_identity_connections_deleted_at ON identity_connections (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE identity_connection_credentials (
  id BIGSERIAL PRIMARY KEY,
  connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  purpose VARCHAR(30) NOT NULL,
  ciphertext TEXT NOT NULL,
  nonce VARCHAR(255) NOT NULL,
  algorithm VARCHAR(30) NOT NULL,
  key_version INTEGER NOT NULL DEFAULT 1,
  rotated_at TIMESTAMP WITH TIME ZONE NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  CONSTRAINT uk_connection_credential UNIQUE (connection_id, purpose)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE external_principals (
  id BIGSERIAL PRIMARY KEY,
  connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  user_id BIGINT,
  status VARCHAR(20) NOT NULL DEFAULT 'unmatched',
  display_name VARCHAR(100) NOT NULL DEFAULT '',
  avatar VARCHAR(255) NOT NULL DEFAULT '',
  email VARCHAR(100) NOT NULL DEFAULT '',
  phone VARCHAR(30) NOT NULL DEFAULT '',
  raw_profile JSONB,
  last_login_at TIMESTAMP WITH TIME ZONE,
  last_synced_at TIMESTAMP WITH TIME ZONE,
  login_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  CONSTRAINT uk_principal_connection_user UNIQUE (connection_id, user_id)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_external_principals_connection_id ON external_principals (connection_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_external_principals_user_id ON external_principals (user_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_external_principals_status ON external_principals (status);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_external_principals_email ON external_principals (email);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_external_principals_phone ON external_principals (phone);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_external_principals_deleted_at ON external_principals (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE external_identifiers (
  id BIGSERIAL PRIMARY KEY,
  connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  principal_id BIGINT NOT NULL REFERENCES external_principals(id) ON DELETE CASCADE,
  namespace VARCHAR(40) NOT NULL,
  value VARCHAR(255) NOT NULL,
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  CONSTRAINT uk_external_identifier UNIQUE (connection_id, namespace, value)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_external_identifiers_principal_id ON external_identifiers (principal_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS external_identifiers;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS external_principals;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS identity_connection_credentials;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS identity_connections;
-- +goose StatementEnd
