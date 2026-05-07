-- +goose Up
-- +goose StatementBegin
CREATE TABLE departments (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  code VARCHAR(50) NOT NULL,
  short_name VARCHAR(50) NOT NULL DEFAULT '',
  description VARCHAR(255) NOT NULL DEFAULT '',
  type INTEGER NOT NULL DEFAULT 2,
  status INTEGER NOT NULL DEFAULT 1,
  parent_id BIGINT,
  level INTEGER NOT NULL DEFAULT 1,
  path VARCHAR(500) NOT NULL DEFAULT '',
  sort INTEGER NOT NULL DEFAULT 0,
  manager_id BIGINT,
  phone VARCHAR(20) NOT NULL DEFAULT '',
  email VARCHAR(100) NOT NULL DEFAULT '',
  address VARCHAR(255) NOT NULL DEFAULT '',
  postal_code VARCHAR(20) NOT NULL DEFAULT '',
  fax VARCHAR(20) NOT NULL DEFAULT '',
  website VARCHAR(255) NOT NULL DEFAULT '',
  location VARCHAR(255) NOT NULL DEFAULT '',
  cost_center VARCHAR(50) NOT NULL DEFAULT '',
  budget_code VARCHAR(50) NOT NULL DEFAULT '',
  tax_id VARCHAR(50) NOT NULL DEFAULT '',
  legal_entity VARCHAR(100) NOT NULL DEFAULT '',
  business_type VARCHAR(100) NOT NULL DEFAULT '',
  data_scope INTEGER NOT NULL DEFAULT 1,
  data_scope_desc VARCHAR(255) NOT NULL DEFAULT '',
  is_leaf BOOLEAN NOT NULL DEFAULT TRUE,
  has_children BOOLEAN NOT NULL DEFAULT FALSE,
  established_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  attributes JSONB,
  remark TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_departments_code ON departments (code);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_departments_parent_id ON departments (parent_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_departments_manager_id ON departments (manager_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_departments_deleted_at ON departments (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE roles (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(50) NOT NULL,
  code VARCHAR(50) NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT '',
  type INTEGER NOT NULL DEFAULT 1,
  status INTEGER NOT NULL DEFAULT 1,
  parent_id BIGINT,
  level INTEGER NOT NULL DEFAULT 1,
  path VARCHAR(500) NOT NULL DEFAULT '',
  sort INTEGER NOT NULL DEFAULT 0,
  data_scope INTEGER NOT NULL DEFAULT 1,
  scope_desc VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  attributes JSONB,
  remark TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_roles_name ON roles (name, deleted_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_roles_code ON roles (code, deleted_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_roles_parent_id ON roles (parent_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_roles_deleted_at ON roles (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE permissions (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  code VARCHAR(100) NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT '',
  type INTEGER NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 1,
  parent_id BIGINT,
  level INTEGER NOT NULL DEFAULT 1,
  path VARCHAR(500) NOT NULL DEFAULT '',
  sort INTEGER NOT NULL DEFAULT 0,
  resource VARCHAR(255) NOT NULL DEFAULT '',
  action VARCHAR(100) NOT NULL DEFAULT '',
  method VARCHAR(20) NOT NULL DEFAULT '',
  url VARCHAR(255) NOT NULL DEFAULT '',
  component VARCHAR(255) NOT NULL DEFAULT '',
  icon VARCHAR(100) NOT NULL DEFAULT '',
  redirect VARCHAR(255) NOT NULL DEFAULT '',
  external BOOLEAN NOT NULL DEFAULT FALSE,
  hidden BOOLEAN NOT NULL DEFAULT FALSE,
  keep_alive BOOLEAN NOT NULL DEFAULT FALSE,
  show_in_menu BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  attributes JSONB,
  remark TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_permissions_code ON permissions (code);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_permissions_parent_id ON permissions (parent_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_permissions_deleted_at ON permissions (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(50) NOT NULL,
  email VARCHAR(100),
  phone VARCHAR(20),
  nickname VARCHAR(50) NOT NULL DEFAULT '',
  real_name VARCHAR(50) NOT NULL DEFAULT '',
  avatar VARCHAR(255) NOT NULL DEFAULT '',
  salt VARCHAR(32) NOT NULL DEFAULT '',
  password_hash VARCHAR(100) NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 0,
  is_activated BOOLEAN NOT NULL DEFAULT FALSE,
  is_verified BOOLEAN NOT NULL DEFAULT FALSE,
  login_count BIGINT NOT NULL DEFAULT 0,
  last_login_at TIMESTAMP WITH TIME ZONE,
  last_login_ip VARCHAR(45) NOT NULL DEFAULT '',
  password_expiry TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  attributes JSONB,
  remark TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_users_username ON users (username, deleted_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_users_email ON users (email, deleted_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_users_phone ON users (phone, deleted_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_users_deleted_at ON users (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE user_departments (
  user_id BIGINT NOT NULL,
  department_id BIGINT NOT NULL,
  position VARCHAR(100) NOT NULL DEFAULT '',
  title VARCHAR(100) NOT NULL DEFAULT '',
  type INTEGER NOT NULL DEFAULT 0,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  sort INTEGER NOT NULL DEFAULT 0,
  joined_at TIMESTAMP WITH TIME ZONE,
  left_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  attributes JSONB,
  remark TEXT,
  PRIMARY KEY (user_id, department_id)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_departments_user_id ON user_departments (user_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_departments_department_id ON user_departments (department_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_departments_deleted_at ON user_departments (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE user_roles (
  user_id BIGINT NOT NULL,
  role_id BIGINT NOT NULL,
  assigned_by BIGINT NOT NULL,
  assigned_at TIMESTAMP WITH TIME ZONE NOT NULL,
  expires_at TIMESTAMP WITH TIME ZONE,
  constraints JSONB,
  data_scope INTEGER NOT NULL DEFAULT 1,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  department_id BIGINT,
  organization_id BIGINT,
  apply_scope INTEGER NOT NULL DEFAULT 1,
  approval_status INTEGER NOT NULL DEFAULT 1,
  approved_by BIGINT,
  approved_at TIMESTAMP WITH TIME ZONE,
  approval_remark TEXT,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  PRIMARY KEY (user_id, role_id)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_roles_user_id ON user_roles (user_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_roles_role_id ON user_roles (role_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_roles_department_id ON user_roles (department_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_roles_organization_id ON user_roles (organization_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_roles_approved_by ON user_roles (approved_by);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_roles_deleted_at ON user_roles (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE role_permissions (
  role_id BIGINT NOT NULL,
  permission_id BIGINT NOT NULL,
  granted_by BIGINT NOT NULL,
  granted_at TIMESTAMP WITH TIME ZONE NOT NULL,
  expires_at TIMESTAMP WITH TIME ZONE,
  conditions JSONB,
  data_scope INTEGER NOT NULL DEFAULT 1,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  PRIMARY KEY (role_id, permission_id)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_role_permissions_role_id ON role_permissions (role_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_role_permissions_permission_id ON role_permissions (permission_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_role_permissions_deleted_at ON role_permissions (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE applications (
  id BIGSERIAL PRIMARY KEY,
  uuid VARCHAR(36) NOT NULL,
  name VARCHAR(100) NOT NULL,
  description TEXT,
  protocol_type VARCHAR(20) NOT NULL,
  protocol_config JSONB NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  is_verified BOOLEAN NOT NULL DEFAULT FALSE,
  access_scope JSONB,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  created_by_user_id BIGINT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_applications_uuid ON applications (uuid, deleted_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_applications_protocol_type ON applications (protocol_type);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_applications_created_by_user_id ON applications (created_by_user_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_applications_deleted_at ON applications (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE application_oidc_keys (
  id BIGSERIAL PRIMARY KEY,
  application_id BIGINT NOT NULL,
  key_id VARCHAR(100) NOT NULL,
  algorithm VARCHAR(20) NOT NULL,
  key_type VARCHAR(20) NOT NULL,
  private_key TEXT NOT NULL,
  public_key TEXT,
  key_size INTEGER NOT NULL DEFAULT 0,
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  expires_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_application_oidc_keys_key_id ON application_oidc_keys (key_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_app_keys ON application_oidc_keys (application_id, is_primary);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_application_oidc_keys_expires_at ON application_oidc_keys (expires_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_application_oidc_keys_deleted_at ON application_oidc_keys (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE authorization_codes (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(100) NOT NULL,
  client_id VARCHAR(100) NOT NULL,
  user_id BIGINT NOT NULL,
  redirect_uri TEXT NOT NULL,
  scopes JSONB,
  state TEXT,
  nonce TEXT,
  code_challenge VARCHAR(128) NOT NULL DEFAULT '',
  code_challenge_method VARCHAR(10) NOT NULL DEFAULT '',
  is_used BOOLEAN NOT NULL DEFAULT FALSE,
  expires_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_authorization_codes_code ON authorization_codes (code);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_authorization_codes_client_id ON authorization_codes (client_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_authorization_codes_user_id ON authorization_codes (user_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_authorization_codes_deleted_at ON authorization_codes (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE oidc_tokens (
  id BIGSERIAL PRIMARY KEY,
  access_token TEXT NOT NULL,
  access_token_hash VARCHAR(64) NOT NULL,
  token_type VARCHAR(20) NOT NULL DEFAULT 'Bearer',
  client_id VARCHAR(100) NOT NULL,
  user_id BIGINT NOT NULL,
  scopes JSONB,
  refresh_token VARCHAR(255) NOT NULL DEFAULT '',
  id_token TEXT,
  expires_at TIMESTAMP WITH TIME ZONE,
  refresh_expires_at TIMESTAMP WITH TIME ZONE,
  is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
  authorization_code_id BIGINT,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  attributes JSONB
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_oidc_tokens_access_token_hash ON oidc_tokens (access_token_hash);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_oidc_tokens_refresh_token ON oidc_tokens (refresh_token);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_oidc_tokens_client_id ON oidc_tokens (client_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_oidc_tokens_user_id ON oidc_tokens (user_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_oidc_tokens_authorization_code_id ON oidc_tokens (authorization_code_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_oidc_tokens_deleted_at ON oidc_tokens (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE social_providers (
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
  redirect_url VARCHAR(255) NOT NULL DEFAULT '',
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
CREATE UNIQUE INDEX uk_social_providers_identifier ON social_providers (identifier);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_social_providers_deleted_at ON social_providers (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE user_social_accounts (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  provider_id BIGINT NOT NULL,
  open_id VARCHAR(255) NOT NULL,
  union_id VARCHAR(255) NOT NULL DEFAULT '',
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
CREATE UNIQUE INDEX idx_provider_openid ON user_social_accounts (provider_id, open_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_provider ON user_social_accounts (user_id, provider_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_social_accounts_provider_id ON user_social_accounts (provider_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_social_accounts_union_id ON user_social_accounts (union_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_user_social_accounts_deleted_at ON user_social_accounts (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE settings (
  id BIGSERIAL PRIMARY KEY,
  setting_key VARCHAR(100) NOT NULL,
  value TEXT,
  category VARCHAR(50) NOT NULL DEFAULT '',
  remark VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX uk_settings_setting_key ON settings (setting_key);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_settings_category ON settings (category);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_settings_deleted_at ON settings (deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS settings;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS user_social_accounts;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS social_providers;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS oidc_tokens;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS authorization_codes;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS application_oidc_keys;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS applications;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS role_permissions;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS user_roles;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS user_departments;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS permissions;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS roles;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS departments;
-- +goose StatementEnd
