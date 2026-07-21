-- +goose Up
CREATE TABLE external_principal_conflicts (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, connection_id BIGINT UNSIGNED NOT NULL,
  identifier_json JSON NOT NULL, candidate_principal_ids_json JSON NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending', canonical_principal_id BIGINT UNSIGNED NULL,
  target_user_id BIGINT UNSIGNED NULL, resolution_note TEXT NOT NULL, reviewed_by BIGINT UNSIGNED NULL,
  reviewed_at DATETIME(3) NULL, created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL,
  KEY idx_principal_conflicts_connection (connection_id), KEY idx_principal_conflicts_status (status),
  CONSTRAINT fk_principal_conflicts_connection FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE identity_connection_health_checks (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, connection_id BIGINT UNSIGNED NOT NULL,
  capability VARCHAR(30) NOT NULL, status VARCHAR(20) NOT NULL, error_message TEXT NOT NULL,
  details_json JSON NULL, checked_at DATETIME(3) NOT NULL, created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL,
  UNIQUE KEY uk_connection_capability_health (connection_id, capability),
  CONSTRAINT fk_connection_health_connection FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE directory_sync_snapshot_objects (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, run_id BIGINT UNSIGNED NOT NULL, connection_id BIGINT UNSIGNED NOT NULL,
  object_type VARCHAR(20) NOT NULL, object_key VARCHAR(255) NOT NULL, payload_json JSON NOT NULL,
  content_hash VARCHAR(64) NOT NULL, created_at DATETIME(3) NULL,
  UNIQUE KEY uk_directory_snapshot_object (run_id, object_type, object_key),
  KEY idx_directory_snapshot_connection (connection_id), KEY idx_directory_snapshot_hash (content_hash),
  CONSTRAINT fk_directory_snapshot_run FOREIGN KEY (run_id) REFERENCES directory_sync_runs(id) ON DELETE CASCADE,
  CONSTRAINT fk_directory_snapshot_connection FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE directory_operation_leases (
  lease_key VARCHAR(255) PRIMARY KEY, owner VARCHAR(120) NOT NULL, run_id BIGINT UNSIGNED NULL,
  lease_expires_at DATETIME(3) NOT NULL, created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL,
  KEY idx_directory_lease_owner (owner), KEY idx_directory_lease_expires (lease_expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE directory_sync_runs ADD COLUMN scope_hash VARCHAR(64) NOT NULL DEFAULT '',
  ADD COLUMN is_complete TINYINT(1) NOT NULL DEFAULT 0, ADD COLUMN superseded_at DATETIME(3) NULL,
  ADD KEY idx_directory_runs_scope_hash (scope_hash);
ALTER TABLE directory_sync_changes ADD COLUMN snapshot_object_id BIGINT UNSIGNED NULL,
  ADD COLUMN attempt_count INT NOT NULL DEFAULT 0, ADD COLUMN last_attempt_at DATETIME(3) NULL,
  ADD KEY idx_directory_changes_snapshot_object (snapshot_object_id),
  ADD CONSTRAINT fk_directory_change_snapshot FOREIGN KEY (snapshot_object_id) REFERENCES directory_sync_snapshot_objects(id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE directory_sync_changes DROP FOREIGN KEY fk_directory_change_snapshot,
  DROP INDEX idx_directory_changes_snapshot_object, DROP COLUMN last_attempt_at,
  DROP COLUMN attempt_count, DROP COLUMN snapshot_object_id;
ALTER TABLE directory_sync_runs DROP INDEX idx_directory_runs_scope_hash,
  DROP COLUMN superseded_at, DROP COLUMN is_complete, DROP COLUMN scope_hash;
DROP TABLE directory_operation_leases;
DROP TABLE directory_sync_snapshot_objects;
DROP TABLE identity_connection_health_checks;
DROP TABLE external_principal_conflicts;
