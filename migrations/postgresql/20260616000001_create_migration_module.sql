-- +goose Up
-- +goose StatementBegin
CREATE TABLE migration_sources (
  id BIGSERIAL PRIMARY KEY,
  uuid VARCHAR(36) NOT NULL,
  name VARCHAR(100) NOT NULL,
  type VARCHAR(20) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  sort INTEGER NOT NULL DEFAULT 0,
  connection_config JSONB,
  login_script TEXT,
  transform_script TEXT,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  remark TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_migration_sources_uuid ON migration_sources (uuid, deleted_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_migration_sources_deleted_at ON migration_sources (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE legacy_users (
  id BIGSERIAL PRIMARY KEY,
  source_id BIGINT NOT NULL,
  username VARCHAR(100) NOT NULL,
  external_id VARCHAR(255) NOT NULL DEFAULT '',
  email VARCHAR(100),
  phone VARCHAR(20),
  nickname VARCHAR(100) NOT NULL DEFAULT '',
  password_hash VARCHAR(255) NOT NULL DEFAULT '',
  salt VARCHAR(64) NOT NULL DEFAULT '',
  raw_data JSONB,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  migrated_user_id BIGINT,
  migrated_at TIMESTAMP WITH TIME ZONE,
  migration_remark TEXT,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_legacy_users_source_username ON legacy_users (source_id, username, deleted_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_legacy_users_source_id ON legacy_users (source_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_legacy_users_username ON legacy_users (username);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_legacy_users_email ON legacy_users (email);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_legacy_users_phone ON legacy_users (phone);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_legacy_users_external_id ON legacy_users (external_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_legacy_users_status ON legacy_users (status);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_legacy_users_migrated_user_id ON legacy_users (migrated_user_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_legacy_users_deleted_at ON legacy_users (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE migration_tasks (
  id BIGSERIAL PRIMARY KEY,
  source_id BIGINT NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  total BIGINT NOT NULL DEFAULT 0,
  processed BIGINT NOT NULL DEFAULT 0,
  succeeded BIGINT NOT NULL DEFAULT 0,
  failed BIGINT NOT NULL DEFAULT 0,
  error TEXT,
  started_at TIMESTAMP WITH TIME ZONE,
  finished_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_migration_tasks_source_id ON migration_tasks (source_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_migration_tasks_status ON migration_tasks (status);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_migration_tasks_deleted_at ON migration_tasks (deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS migration_tasks;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS legacy_users;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS migration_sources;
-- +goose StatementEnd
