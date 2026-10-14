-- +goose Up
CREATE TABLE custom_permissions (
 id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT, code VARCHAR(100) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 name VARCHAR(100) NOT NULL, description TEXT NOT NULL, enabled BOOLEAN NOT NULL,
 created_by BIGINT NOT NULL, updated_by BIGINT NOT NULL,
 created_at DATETIME(3) NOT NULL, updated_at DATETIME(3) NOT NULL, deleted_at DATETIME(3)
);
CREATE INDEX idx_custom_permissions_deleted_at ON custom_permissions(deleted_at);
CREATE TABLE custom_permission_grants (
 id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT, permission_id BIGINT NOT NULL, subject_type VARCHAR(20) NOT NULL,
 subject_id BIGINT NOT NULL, include_sub_departments BOOLEAN NOT NULL DEFAULT FALSE,
 starts_at DATETIME(3), ends_at DATETIME(3), remark TEXT NOT NULL,
 created_by BIGINT NOT NULL, updated_by BIGINT NOT NULL,
 created_at DATETIME(3) NOT NULL, updated_at DATETIME(3) NOT NULL, deleted_at DATETIME(3),
 CHECK ((starts_at IS NULL AND ends_at IS NULL) OR (starts_at IS NOT NULL AND ends_at IS NOT NULL AND starts_at < ends_at))
);
CREATE INDEX idx_custom_grants_permission ON custom_permission_grants(permission_id);
CREATE INDEX idx_custom_grants_subject ON custom_permission_grants(subject_type, subject_id);
CREATE INDEX idx_custom_grants_deleted_at ON custom_permission_grants(deleted_at);

-- +goose Down
-- Old binaries would restore obsolete app-local custom grants. Restore a
-- coordinated application/database backup instead of silently downgrading.
SELECT 1 FROM custom_permission_rollback_requires_backup;
