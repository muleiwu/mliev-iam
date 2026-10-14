-- +goose Up
CREATE TABLE custom_permissions (
 id INTEGER PRIMARY KEY AUTOINCREMENT, code VARCHAR(100) COLLATE BINARY NOT NULL UNIQUE,
 name VARCHAR(100) NOT NULL, description TEXT NOT NULL, enabled BOOLEAN NOT NULL,
 created_by BIGINT NOT NULL, updated_by BIGINT NOT NULL,
 created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, deleted_at DATETIME
);
CREATE INDEX idx_custom_permissions_deleted_at ON custom_permissions(deleted_at);
CREATE TABLE custom_permission_grants (
 id INTEGER PRIMARY KEY AUTOINCREMENT, permission_id BIGINT NOT NULL, subject_type VARCHAR(20) NOT NULL,
 subject_id BIGINT NOT NULL, include_sub_departments BOOLEAN NOT NULL DEFAULT FALSE,
 starts_at DATETIME, ends_at DATETIME, remark TEXT NOT NULL,
 created_by BIGINT NOT NULL, updated_by BIGINT NOT NULL,
 created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, deleted_at DATETIME,
 CHECK ((starts_at IS NULL AND ends_at IS NULL) OR (starts_at IS NOT NULL AND ends_at IS NOT NULL AND starts_at < ends_at))
);
CREATE INDEX idx_custom_grants_permission ON custom_permission_grants(permission_id);
CREATE INDEX idx_custom_grants_subject ON custom_permission_grants(subject_type, subject_id);
CREATE INDEX idx_custom_grants_deleted_at ON custom_permission_grants(deleted_at);

-- +goose Down
-- Old binaries would restore obsolete app-local custom grants. Restore a
-- coordinated application/database backup instead of silently downgrading.
SELECT 1 FROM custom_permission_rollback_requires_backup;
