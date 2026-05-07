-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN uuid VARCHAR(36) NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX idx_users_uuid ON users (uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_users_uuid;
-- +goose StatementEnd

-- SQLite 3.35+ 才支持 DROP COLUMN；较旧版本需重建表回滚。
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN uuid;
-- +goose StatementEnd
