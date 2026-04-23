-- +goose Up
-- +goose StatementBegin
CREATE TABLE departments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(100) NOT NULL,
  code VARCHAR(50) NOT NULL,
  short_name VARCHAR(50) NOT NULL DEFAULT '',
  description VARCHAR(255) NOT NULL DEFAULT '',
  type INTEGER NOT NULL DEFAULT 2,
  status INTEGER NOT NULL DEFAULT 1,
  parent_id INTEGER,
  level INTEGER NOT NULL DEFAULT 1,
  path VARCHAR(500) NOT NULL DEFAULT '',
  sort INTEGER NOT NULL DEFAULT 0,
  manager_id INTEGER,
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
  is_leaf INTEGER NOT NULL DEFAULT 1,
  has_children INTEGER NOT NULL DEFAULT 0,
  established_at DATETIME,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  attributes TEXT,
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(50) NOT NULL,
  code VARCHAR(50) NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT '',
  type INTEGER NOT NULL DEFAULT 1,
  status INTEGER NOT NULL DEFAULT 1,
  parent_id INTEGER,
  level INTEGER NOT NULL DEFAULT 1,
  path VARCHAR(500) NOT NULL DEFAULT '',
  sort INTEGER NOT NULL DEFAULT 0,
  data_scope INTEGER NOT NULL DEFAULT 1,
  scope_desc VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  attributes TEXT,
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(100) NOT NULL,
  code VARCHAR(100) NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT '',
  type INTEGER NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 1,
  parent_id INTEGER,
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
  external INTEGER NOT NULL DEFAULT 0,
  hidden INTEGER NOT NULL DEFAULT 0,
  keep_alive INTEGER NOT NULL DEFAULT 0,
  show_in_menu INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  attributes TEXT,
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username VARCHAR(50) NOT NULL,
  email VARCHAR(100),
  phone VARCHAR(20),
  nickname VARCHAR(50) NOT NULL DEFAULT '',
  real_name VARCHAR(50) NOT NULL DEFAULT '',
  avatar VARCHAR(255) NOT NULL DEFAULT '',
  salt VARCHAR(32) NOT NULL DEFAULT '',
  password_hash VARCHAR(100) NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 0,
  is_activated INTEGER NOT NULL DEFAULT 0,
  is_verified INTEGER NOT NULL DEFAULT 0,
  login_count INTEGER NOT NULL DEFAULT 0,
  last_login_at DATETIME,
  last_login_ip VARCHAR(45) NOT NULL DEFAULT '',
  password_expiry DATETIME,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  attributes TEXT,
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
  user_id INTEGER NOT NULL,
  department_id INTEGER NOT NULL,
  position VARCHAR(100) NOT NULL DEFAULT '',
  title VARCHAR(100) NOT NULL DEFAULT '',
  type INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 1,
  is_primary INTEGER NOT NULL DEFAULT 0,
  sort INTEGER NOT NULL DEFAULT 0,
  joined_at DATETIME,
  left_at DATETIME,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  attributes TEXT,
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
  user_id INTEGER NOT NULL,
  role_id INTEGER NOT NULL,
  assigned_by INTEGER NOT NULL,
  assigned_at DATETIME NOT NULL,
  expires_at DATETIME,
  constraints TEXT,
  data_scope INTEGER NOT NULL DEFAULT 1,
  is_active INTEGER NOT NULL DEFAULT 1,
  is_primary INTEGER NOT NULL DEFAULT 0,
  department_id INTEGER,
  organization_id INTEGER,
  apply_scope INTEGER NOT NULL DEFAULT 1,
  approval_status INTEGER NOT NULL DEFAULT 1,
  approved_by INTEGER,
  approved_at DATETIME,
  approval_remark TEXT,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
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
  role_id INTEGER NOT NULL,
  permission_id INTEGER NOT NULL,
  granted_by INTEGER NOT NULL,
  granted_at DATETIME NOT NULL,
  expires_at DATETIME,
  conditions TEXT,
  data_scope INTEGER NOT NULL DEFAULT 1,
  is_active INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  uuid VARCHAR(36) NOT NULL,
  name VARCHAR(100) NOT NULL,
  description TEXT,
  protocol_type VARCHAR(20) NOT NULL,
  protocol_config TEXT NOT NULL,
  is_active INTEGER NOT NULL DEFAULT 1,
  is_verified INTEGER NOT NULL DEFAULT 0,
  access_scope TEXT,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  created_by_user_id INTEGER
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  application_id INTEGER NOT NULL,
  key_id VARCHAR(100) NOT NULL,
  algorithm VARCHAR(20) NOT NULL,
  key_type VARCHAR(20) NOT NULL,
  private_key TEXT NOT NULL,
  public_key TEXT,
  key_size INTEGER NOT NULL DEFAULT 0,
  is_primary INTEGER NOT NULL DEFAULT 0,
  expires_at DATETIME,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  code VARCHAR(100) NOT NULL,
  client_id VARCHAR(100) NOT NULL,
  user_id INTEGER NOT NULL,
  redirect_uri TEXT NOT NULL,
  scopes TEXT,
  state TEXT,
  nonce TEXT,
  code_challenge VARCHAR(128) NOT NULL DEFAULT '',
  code_challenge_method VARCHAR(10) NOT NULL DEFAULT '',
  is_used INTEGER NOT NULL DEFAULT 0,
  expires_at DATETIME,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  access_token TEXT NOT NULL,
  access_token_hash VARCHAR(64) NOT NULL,
  token_type VARCHAR(20) NOT NULL DEFAULT 'Bearer',
  client_id VARCHAR(100) NOT NULL,
  user_id INTEGER NOT NULL,
  scopes TEXT,
  refresh_token VARCHAR(255) NOT NULL DEFAULT '',
  id_token TEXT,
  expires_at DATETIME,
  refresh_expires_at DATETIME,
  is_revoked INTEGER NOT NULL DEFAULT 0,
  authorization_code_id INTEGER,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  attributes TEXT
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
  redirect_url VARCHAR(255) NOT NULL DEFAULT '',
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
CREATE UNIQUE INDEX uk_social_providers_identifier ON social_providers (identifier);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_social_providers_deleted_at ON social_providers (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE user_social_accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  provider_id INTEGER NOT NULL,
  open_id VARCHAR(255) NOT NULL,
  union_id VARCHAR(255) NOT NULL DEFAULT '',
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  setting_key VARCHAR(100) NOT NULL,
  value TEXT,
  category VARCHAR(50) NOT NULL DEFAULT '',
  remark VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME
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
