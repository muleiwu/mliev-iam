-- +goose Up
DROP INDEX IF EXISTS idx_users_uuid;
DROP INDEX IF EXISTS uk_users_username;
DROP INDEX IF EXISTS uk_users_email;
DROP INDEX IF EXISTS uk_users_phone;

CREATE UNIQUE INDEX uk_users_active_uuid ON users (uuid) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uk_users_active_username ON users (username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uk_users_active_email ON users (email) WHERE deleted_at IS NULL AND email IS NOT NULL;
CREATE UNIQUE INDEX uk_users_active_phone ON users (phone) WHERE deleted_at IS NULL AND phone IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS uk_users_active_uuid;
DROP INDEX IF EXISTS uk_users_active_username;
DROP INDEX IF EXISTS uk_users_active_email;
DROP INDEX IF EXISTS uk_users_active_phone;

CREATE UNIQUE INDEX idx_users_uuid ON users (uuid);
CREATE UNIQUE INDEX uk_users_username ON users (username, deleted_at);
CREATE UNIQUE INDEX uk_users_email ON users (email, deleted_at);
CREATE UNIQUE INDEX uk_users_phone ON users (phone, deleted_at);
