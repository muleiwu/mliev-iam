-- +goose Up
CREATE TABLE external_principal_conflicts (
  id BIGSERIAL PRIMARY KEY, connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  identifier_json JSONB NOT NULL, candidate_principal_ids_json JSONB NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending', canonical_principal_id BIGINT, target_user_id BIGINT,
  resolution_note TEXT NOT NULL DEFAULT '', reviewed_by BIGINT, reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
);
CREATE INDEX idx_principal_conflicts_connection ON external_principal_conflicts(connection_id);
CREATE INDEX idx_principal_conflicts_status ON external_principal_conflicts(status);

CREATE TABLE identity_connection_health_checks (
  id BIGSERIAL PRIMARY KEY, connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  capability VARCHAR(30) NOT NULL, status VARCHAR(20) NOT NULL, error_message TEXT NOT NULL DEFAULT '',
  details_json JSONB, checked_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ,
  UNIQUE(connection_id, capability)
);

CREATE TABLE directory_sync_snapshot_objects (
  id BIGSERIAL PRIMARY KEY, run_id BIGINT NOT NULL REFERENCES directory_sync_runs(id) ON DELETE CASCADE,
  connection_id BIGINT NOT NULL REFERENCES identity_connections(id) ON DELETE CASCADE,
  object_type VARCHAR(20) NOT NULL, object_key VARCHAR(255) NOT NULL, payload_json JSONB NOT NULL,
  content_hash VARCHAR(64) NOT NULL, created_at TIMESTAMPTZ, UNIQUE(run_id, object_type, object_key)
);
CREATE INDEX idx_directory_snapshot_connection ON directory_sync_snapshot_objects(connection_id);
CREATE INDEX idx_directory_snapshot_hash ON directory_sync_snapshot_objects(content_hash);

CREATE TABLE directory_operation_leases (
  lease_key VARCHAR(255) PRIMARY KEY, owner VARCHAR(120) NOT NULL, run_id BIGINT,
  lease_expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
);
CREATE INDEX idx_directory_lease_owner ON directory_operation_leases(owner);
CREATE INDEX idx_directory_lease_expires ON directory_operation_leases(lease_expires_at);

ALTER TABLE directory_sync_runs ADD COLUMN scope_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE directory_sync_runs ADD COLUMN is_complete BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE directory_sync_runs ADD COLUMN superseded_at TIMESTAMPTZ;
CREATE INDEX idx_directory_runs_scope_hash ON directory_sync_runs(scope_hash);
ALTER TABLE directory_sync_changes ADD COLUMN snapshot_object_id BIGINT REFERENCES directory_sync_snapshot_objects(id) ON DELETE RESTRICT;
ALTER TABLE directory_sync_changes ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE directory_sync_changes ADD COLUMN last_attempt_at TIMESTAMPTZ;
CREATE INDEX idx_directory_changes_snapshot_object ON directory_sync_changes(snapshot_object_id);

-- +goose Down
ALTER TABLE directory_sync_changes DROP COLUMN last_attempt_at;
ALTER TABLE directory_sync_changes DROP COLUMN attempt_count;
ALTER TABLE directory_sync_changes DROP COLUMN snapshot_object_id;
DROP INDEX idx_directory_runs_scope_hash;
ALTER TABLE directory_sync_runs DROP COLUMN superseded_at;
ALTER TABLE directory_sync_runs DROP COLUMN is_complete;
ALTER TABLE directory_sync_runs DROP COLUMN scope_hash;
DROP TABLE directory_operation_leases;
DROP TABLE directory_sync_snapshot_objects;
DROP TABLE identity_connection_health_checks;
DROP TABLE external_principal_conflicts;
