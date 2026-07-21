-- +goose Up
CREATE TABLE directory_sync_policies (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL, root_department_id INTEGER NOT NULL,
  sync_scope_json TEXT, field_mapping_json TEXT, field_ownership_json TEXT,
  match_policy VARCHAR(30) NOT NULL DEFAULT 'manual',
  deprovision_policy VARCHAR(60) NOT NULL DEFAULT 'block_identity_require_user_approval',
  enabled INTEGER NOT NULL DEFAULT 0, created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_directory_policy_connection ON directory_sync_policies(connection_id);
CREATE UNIQUE INDEX uk_directory_policy_root ON directory_sync_policies(root_department_id);
CREATE INDEX idx_directory_policy_enabled ON directory_sync_policies(enabled);

CREATE TABLE external_org_units (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL, external_id VARCHAR(255) NOT NULL,
  department_id INTEGER, external_parent_id VARCHAR(255) NOT NULL DEFAULT '', name VARCHAR(100) NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0, status VARCHAR(20) NOT NULL DEFAULT 'active',
  leader_external_id VARCHAR(255) NOT NULL DEFAULT '', snapshot_version VARCHAR(64) NOT NULL,
  last_seen_at DATETIME NOT NULL, raw_data TEXT, created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_external_org_unit ON external_org_units(connection_id, external_id);
CREATE INDEX idx_external_org_units_department_id ON external_org_units(department_id);
CREATE INDEX idx_external_org_units_parent ON external_org_units(external_parent_id);
CREATE INDEX idx_external_org_units_snapshot ON external_org_units(snapshot_version);
CREATE INDEX idx_external_org_units_seen ON external_org_units(last_seen_at);

CREATE TABLE external_memberships (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL, principal_id INTEGER NOT NULL,
  external_org_unit_id INTEGER NOT NULL, user_id INTEGER, department_id INTEGER,
  is_primary INTEGER NOT NULL DEFAULT 0, is_leader INTEGER NOT NULL DEFAULT 0,
  position VARCHAR(100) NOT NULL DEFAULT '', status VARCHAR(20) NOT NULL DEFAULT 'active',
  snapshot_version VARCHAR(64) NOT NULL, last_seen_at DATETIME NOT NULL, raw_data TEXT,
  created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE,
  FOREIGN KEY (principal_id) REFERENCES external_principals(id) ON DELETE CASCADE,
  FOREIGN KEY (external_org_unit_id) REFERENCES external_org_units(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_external_membership ON external_memberships(connection_id, principal_id, external_org_unit_id);
CREATE INDEX idx_external_memberships_principal ON external_memberships(principal_id);
CREATE INDEX idx_external_memberships_org_unit ON external_memberships(external_org_unit_id);
CREATE INDEX idx_external_memberships_user ON external_memberships(user_id);
CREATE INDEX idx_external_memberships_department ON external_memberships(department_id);
CREATE INDEX idx_external_memberships_snapshot ON external_memberships(snapshot_version);
CREATE INDEX idx_external_memberships_seen ON external_memberships(last_seen_at);

CREATE TABLE directory_sync_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL, snapshot_version VARCHAR(64) NOT NULL,
  trigger_type VARCHAR(20) NOT NULL DEFAULT 'manual', status VARCHAR(30) NOT NULL DEFAULT 'pending',
  total_users INTEGER NOT NULL DEFAULT 0, total_org_units INTEGER NOT NULL DEFAULT 0,
  total_memberships INTEGER NOT NULL DEFAULT 0, change_count INTEGER NOT NULL DEFAULT 0,
  applied_count INTEGER NOT NULL DEFAULT 0, conflict_count INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '', started_at DATETIME, finished_at DATETIME,
  created_by INTEGER, created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_directory_sync_runs_snapshot ON directory_sync_runs(snapshot_version);
CREATE INDEX idx_directory_sync_runs_connection ON directory_sync_runs(connection_id);
CREATE INDEX idx_directory_sync_runs_status ON directory_sync_runs(status);
CREATE INDEX idx_directory_sync_runs_created_by ON directory_sync_runs(created_by);

CREATE TABLE directory_sync_changes (
  id INTEGER PRIMARY KEY AUTOINCREMENT, run_id INTEGER NOT NULL, connection_id INTEGER NOT NULL,
  change_type VARCHAR(20) NOT NULL, object_type VARCHAR(20) NOT NULL, object_key VARCHAR(255) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending', risk_level VARCHAR(20) NOT NULL DEFAULT 'low',
  before_json TEXT, after_json TEXT, match_reason VARCHAR(255) NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  reviewed_by INTEGER, reviewed_at DATETIME, applied_at DATETIME, created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (run_id) REFERENCES directory_sync_runs(id) ON DELETE CASCADE,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE INDEX idx_directory_changes_run ON directory_sync_changes(run_id);
CREATE INDEX idx_directory_changes_connection ON directory_sync_changes(connection_id);
CREATE INDEX idx_directory_changes_type ON directory_sync_changes(change_type);
CREATE INDEX idx_directory_changes_object_type ON directory_sync_changes(object_type);
CREATE INDEX idx_directory_changes_object_key ON directory_sync_changes(object_key);
CREATE INDEX idx_directory_changes_status ON directory_sync_changes(status);
CREATE INDEX idx_directory_changes_risk ON directory_sync_changes(risk_level);
CREATE INDEX idx_directory_changes_reviewed_by ON directory_sync_changes(reviewed_by);

-- +goose Down
DROP TABLE IF EXISTS directory_sync_changes;
DROP TABLE IF EXISTS directory_sync_runs;
DROP TABLE IF EXISTS external_memberships;
DROP TABLE IF EXISTS external_org_units;
DROP TABLE IF EXISTS directory_sync_policies;
