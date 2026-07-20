-- +goose Up
CREATE TABLE sync_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL, run_id INTEGER, inbox_event_id INTEGER,
  job_type VARCHAR(30) NOT NULL, status VARCHAR(30) NOT NULL DEFAULT 'pending', payload_json TEXT,
  available_at DATETIME NOT NULL, lease_owner VARCHAR(120) NOT NULL DEFAULT '', lease_expires_at DATETIME,
  attempts INTEGER NOT NULL DEFAULT 0, max_attempts INTEGER NOT NULL DEFAULT 8, last_error TEXT NOT NULL DEFAULT '',
  finished_at DATETIME, created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE,
  FOREIGN KEY (run_id) REFERENCES directory_sync_runs(id) ON DELETE SET NULL,
  FOREIGN KEY (inbox_event_id) REFERENCES external_event_inbox(id) ON DELETE SET NULL
);
CREATE INDEX idx_sync_jobs_connection ON sync_jobs(connection_id);
CREATE INDEX idx_sync_jobs_run ON sync_jobs(run_id);
CREATE UNIQUE INDEX uk_sync_jobs_inbox_event ON sync_jobs(inbox_event_id);
CREATE INDEX idx_sync_jobs_type ON sync_jobs(job_type);
CREATE INDEX idx_sync_jobs_status ON sync_jobs(status);
CREATE INDEX idx_sync_jobs_available ON sync_jobs(available_at);
CREATE INDEX idx_sync_jobs_lease_owner ON sync_jobs(lease_owner);
CREATE INDEX idx_sync_jobs_lease_expires ON sync_jobs(lease_expires_at);

CREATE TABLE sync_schedules (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL, schedule_type VARCHAR(30) NOT NULL,
  cron_expression VARCHAR(100) NOT NULL DEFAULT '', timezone VARCHAR(80) NOT NULL DEFAULT 'UTC',
  enabled INTEGER NOT NULL DEFAULT 1, next_run_at DATETIME, last_enqueued_at DATETIME,
  created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_sync_schedule ON sync_schedules(connection_id, schedule_type);
CREATE INDEX idx_sync_schedules_enabled ON sync_schedules(enabled);
CREATE INDEX idx_sync_schedules_next_run ON sync_schedules(next_run_at);

CREATE TABLE external_event_inbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id INTEGER NOT NULL, provider_event_id VARCHAR(255) NOT NULL,
  event_type VARCHAR(120) NOT NULL, object_type VARCHAR(40) NOT NULL DEFAULT '', external_object_id VARCHAR(255) NOT NULL DEFAULT '',
  status VARCHAR(30) NOT NULL DEFAULT 'received', signature_valid INTEGER NOT NULL DEFAULT 0,
  payload_ciphertext TEXT NOT NULL, payload_nonce VARCHAR(255) NOT NULL, payload_algorithm VARCHAR(30) NOT NULL,
  payload_key_version INTEGER NOT NULL DEFAULT 1, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
  received_at DATETIME NOT NULL, processed_at DATETIME, created_at DATETIME, updated_at DATETIME,
  FOREIGN KEY (connection_id) REFERENCES identity_connections(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uk_external_event ON external_event_inbox(connection_id, provider_event_id);
CREATE INDEX idx_external_events_type ON external_event_inbox(event_type);
CREATE INDEX idx_external_events_object_type ON external_event_inbox(object_type);
CREATE INDEX idx_external_events_object_id ON external_event_inbox(external_object_id);
CREATE INDEX idx_external_events_status ON external_event_inbox(status);
CREATE INDEX idx_external_events_received ON external_event_inbox(received_at);

ALTER TABLE users ADD COLUMN session_revoked_at DATETIME;

-- +goose Down
ALTER TABLE users DROP COLUMN session_revoked_at;
DROP TABLE IF EXISTS sync_jobs;
DROP TABLE IF EXISTS sync_schedules;
DROP TABLE IF EXISTS external_event_inbox;
