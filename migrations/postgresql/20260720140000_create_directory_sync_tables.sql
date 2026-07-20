-- +goose Up
CREATE TABLE directory_sync_policies (
  id BIGSERIAL PRIMARY KEY, connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  root_department_id BIGINT NOT NULL, sync_scope_json JSONB, field_mapping_json JSONB, field_ownership_json JSONB,
  match_policy VARCHAR(30) NOT NULL DEFAULT 'manual',
  deprovision_policy VARCHAR(60) NOT NULL DEFAULT 'block_identity_require_user_approval',
  enabled BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ,
  CONSTRAINT uk_directory_policy_connection UNIQUE(connection_id), CONSTRAINT uk_directory_policy_root UNIQUE(root_department_id)
);
CREATE INDEX idx_directory_policy_enabled ON directory_sync_policies(enabled);

CREATE TABLE external_org_units (
  id BIGSERIAL PRIMARY KEY, connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  external_id VARCHAR(255) NOT NULL, department_id BIGINT, external_parent_id VARCHAR(255) NOT NULL DEFAULT '',
  name VARCHAR(100) NOT NULL, sort_order INTEGER NOT NULL DEFAULT 0, status VARCHAR(20) NOT NULL DEFAULT 'active',
  leader_external_id VARCHAR(255) NOT NULL DEFAULT '', snapshot_version VARCHAR(64) NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL, raw_data JSONB, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ,
  CONSTRAINT uk_external_org_unit UNIQUE(connection_id, external_id)
);
CREATE INDEX idx_external_org_units_department_id ON external_org_units(department_id);
CREATE INDEX idx_external_org_units_parent ON external_org_units(external_parent_id);
CREATE INDEX idx_external_org_units_snapshot ON external_org_units(snapshot_version);
CREATE INDEX idx_external_org_units_seen ON external_org_units(last_seen_at);

CREATE TABLE external_memberships (
  id BIGSERIAL PRIMARY KEY, connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  principal_id BIGINT NOT NULL REFERENCES external_principals(id) ON DELETE CASCADE,
  external_org_unit_id BIGINT NOT NULL REFERENCES external_org_units(id) ON DELETE CASCADE,
  user_id BIGINT, department_id BIGINT, is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  is_leader BOOLEAN NOT NULL DEFAULT FALSE, position VARCHAR(100) NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'active', snapshot_version VARCHAR(64) NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL, raw_data JSONB, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ,
  CONSTRAINT uk_external_membership UNIQUE(connection_id, principal_id, external_org_unit_id)
);
CREATE INDEX idx_external_memberships_principal ON external_memberships(principal_id);
CREATE INDEX idx_external_memberships_org_unit ON external_memberships(external_org_unit_id);
CREATE INDEX idx_external_memberships_user ON external_memberships(user_id);
CREATE INDEX idx_external_memberships_department ON external_memberships(department_id);
CREATE INDEX idx_external_memberships_snapshot ON external_memberships(snapshot_version);
CREATE INDEX idx_external_memberships_seen ON external_memberships(last_seen_at);

CREATE TABLE directory_sync_runs (
  id BIGSERIAL PRIMARY KEY, connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  snapshot_version VARCHAR(64) NOT NULL UNIQUE, trigger_type VARCHAR(20) NOT NULL DEFAULT 'manual',
  status VARCHAR(30) NOT NULL DEFAULT 'pending', total_users INTEGER NOT NULL DEFAULT 0,
  total_org_units INTEGER NOT NULL DEFAULT 0, total_memberships INTEGER NOT NULL DEFAULT 0,
  change_count INTEGER NOT NULL DEFAULT 0, applied_count INTEGER NOT NULL DEFAULT 0,
  conflict_count INTEGER NOT NULL DEFAULT 0, error_message TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ, created_by BIGINT,
  created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
);
CREATE INDEX idx_directory_sync_runs_connection ON directory_sync_runs(connection_id);
CREATE INDEX idx_directory_sync_runs_status ON directory_sync_runs(status);
CREATE INDEX idx_directory_sync_runs_created_by ON directory_sync_runs(created_by);

CREATE TABLE directory_sync_changes (
  id BIGSERIAL PRIMARY KEY, run_id BIGINT NOT NULL REFERENCES directory_sync_runs(id) ON DELETE CASCADE,
  connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  change_type VARCHAR(20) NOT NULL, object_type VARCHAR(20) NOT NULL, object_key VARCHAR(255) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending', risk_level VARCHAR(20) NOT NULL DEFAULT 'low',
  before_json JSONB, after_json JSONB, match_reason VARCHAR(255) NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '', reviewed_by BIGINT, reviewed_at TIMESTAMPTZ,
  applied_at TIMESTAMPTZ, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
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
