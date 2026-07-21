-- +goose Up
CREATE TABLE sync_jobs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, connection_id BIGINT UNSIGNED NOT NULL, run_id BIGINT UNSIGNED NULL, inbox_event_id BIGINT UNSIGNED NULL,
  job_type VARCHAR(30) NOT NULL, status VARCHAR(30) NOT NULL DEFAULT 'pending', payload_json JSON NULL,
  available_at DATETIME(3) NOT NULL, lease_owner VARCHAR(120) NOT NULL DEFAULT '', lease_expires_at DATETIME(3) NULL,
  attempts INT NOT NULL DEFAULT 0, max_attempts INT NOT NULL DEFAULT 8, last_error TEXT NOT NULL,
  finished_at DATETIME(3) NULL, created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL,
  UNIQUE KEY uk_sync_jobs_inbox_event (inbox_event_id), KEY idx_sync_jobs_connection (connection_id), KEY idx_sync_jobs_run (run_id),
  KEY idx_sync_jobs_type (job_type), KEY idx_sync_jobs_status (status), KEY idx_sync_jobs_available (available_at),
  KEY idx_sync_jobs_lease_owner (lease_owner), KEY idx_sync_jobs_lease_expires (lease_expires_at),
  CONSTRAINT fk_sync_jobs_connection FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE,
  CONSTRAINT fk_sync_jobs_run FOREIGN KEY (run_id) REFERENCES directory_sync_runs(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE sync_schedules (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, connection_id BIGINT UNSIGNED NOT NULL, schedule_type VARCHAR(30) NOT NULL,
  cron_expression VARCHAR(100) NOT NULL DEFAULT '', timezone VARCHAR(80) NOT NULL DEFAULT 'UTC', enabled TINYINT(1) NOT NULL DEFAULT 1,
  next_run_at DATETIME(3) NULL, last_enqueued_at DATETIME(3) NULL, created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL,
  UNIQUE KEY uk_sync_schedule (connection_id, schedule_type), KEY idx_sync_schedules_enabled (enabled), KEY idx_sync_schedules_next_run (next_run_at),
  CONSTRAINT fk_sync_schedules_connection FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE external_event_inbox (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, connection_id BIGINT UNSIGNED NOT NULL, provider_event_id VARCHAR(255) NOT NULL,
  event_type VARCHAR(120) NOT NULL, object_type VARCHAR(40) NOT NULL DEFAULT '', external_object_id VARCHAR(255) NOT NULL DEFAULT '',
  status VARCHAR(30) NOT NULL DEFAULT 'received', signature_valid TINYINT(1) NOT NULL DEFAULT 0,
  payload_ciphertext LONGTEXT NOT NULL, payload_nonce VARCHAR(255) NOT NULL, payload_algorithm VARCHAR(30) NOT NULL, payload_key_version INT NOT NULL DEFAULT 1,
  attempts INT NOT NULL DEFAULT 0, last_error TEXT NOT NULL, received_at DATETIME(3) NOT NULL, processed_at DATETIME(3) NULL,
  created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL,
  UNIQUE KEY uk_external_event (connection_id, provider_event_id), KEY idx_external_events_type (event_type),
  KEY idx_external_events_object_type (object_type), KEY idx_external_events_object_id (external_object_id),
  KEY idx_external_events_status (status), KEY idx_external_events_received (received_at),
  CONSTRAINT fk_external_events_connection FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE sync_jobs ADD CONSTRAINT fk_sync_jobs_inbox_event FOREIGN KEY (inbox_event_id) REFERENCES external_event_inbox(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN session_revoked_at DATETIME(3) NULL;

-- +goose Down
ALTER TABLE users DROP COLUMN session_revoked_at;
DROP TABLE IF EXISTS sync_jobs;
DROP TABLE IF EXISTS sync_schedules;
DROP TABLE IF EXISTS external_event_inbox;
