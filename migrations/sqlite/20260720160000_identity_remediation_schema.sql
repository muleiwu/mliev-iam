-- +goose Up
CREATE TABLE external_principal_conflicts (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL,
  identifier_json TEXT NOT NULL, candidate_principal_ids_json TEXT NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending', canonical_principal_id INTEGER,
  target_user_id INTEGER, resolution_note TEXT NOT NULL DEFAULT '', reviewed_by INTEGER,
  reviewed_at DATETIME, created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE INDEX idx_principal_conflicts_connection ON external_principal_conflicts(connection_id);
CREATE INDEX idx_principal_conflicts_status ON external_principal_conflicts(status);

CREATE TABLE identity_connection_health_checks (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL, capability VARCHAR(30) NOT NULL,
  status VARCHAR(20) NOT NULL, error_message TEXT NOT NULL DEFAULT '', details_json TEXT,
  checked_at DATETIME NOT NULL, created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_connection_capability_health ON identity_connection_health_checks(connection_id, capability);

CREATE TABLE directory_sync_snapshot_objects (
  id INTEGER PRIMARY KEY AUTOINCREMENT, run_id INTEGER NOT NULL, connection_id INTEGER NOT NULL,
  object_type VARCHAR(20) NOT NULL, object_key VARCHAR(255) NOT NULL, payload_json TEXT NOT NULL,
  content_hash VARCHAR(64) NOT NULL, created_at DATETIME,
  FOREIGN KEY (run_id) REFERENCES directory_sync_runs(id) ON DELETE CASCADE,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_directory_snapshot_object ON directory_sync_snapshot_objects(run_id, object_type, object_key);
CREATE INDEX idx_directory_snapshot_connection ON directory_sync_snapshot_objects(connection_id);
CREATE INDEX idx_directory_snapshot_hash ON directory_sync_snapshot_objects(content_hash);

CREATE TABLE directory_operation_leases (
  lease_key VARCHAR(255) PRIMARY KEY, owner VARCHAR(120) NOT NULL, run_id INTEGER,
  lease_expires_at DATETIME NOT NULL, created_at DATETIME, updated_at DATETIME
);
CREATE INDEX idx_directory_lease_owner ON directory_operation_leases(owner);
CREATE INDEX idx_directory_lease_expires ON directory_operation_leases(lease_expires_at);

ALTER TABLE directory_sync_runs ADD COLUMN scope_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE directory_sync_runs ADD COLUMN is_complete INTEGER NOT NULL DEFAULT 0;
ALTER TABLE directory_sync_runs ADD COLUMN superseded_at DATETIME;
CREATE INDEX idx_directory_runs_scope_hash ON directory_sync_runs(scope_hash);
ALTER TABLE directory_sync_changes ADD COLUMN snapshot_object_id INTEGER;
ALTER TABLE directory_sync_changes ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE directory_sync_changes ADD COLUMN last_attempt_at DATETIME;
CREATE INDEX idx_directory_changes_snapshot_object ON directory_sync_changes(snapshot_object_id);

-- +goose Down
DROP INDEX IF EXISTS idx_directory_changes_snapshot_object;
ALTER TABLE directory_sync_changes DROP COLUMN last_attempt_at;
ALTER TABLE directory_sync_changes DROP COLUMN attempt_count;
ALTER TABLE directory_sync_changes DROP COLUMN snapshot_object_id;
DROP INDEX IF EXISTS idx_directory_runs_scope_hash;
ALTER TABLE directory_sync_runs DROP COLUMN superseded_at;
ALTER TABLE directory_sync_runs DROP COLUMN is_complete;
ALTER TABLE directory_sync_runs DROP COLUMN scope_hash;
DROP TABLE IF EXISTS directory_operation_leases;
DROP TABLE IF EXISTS directory_sync_snapshot_objects;
DROP TABLE IF EXISTS identity_connection_health_checks;
DROP TABLE IF EXISTS external_principal_conflicts;
