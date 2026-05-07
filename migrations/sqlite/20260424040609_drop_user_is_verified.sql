-- +goose Up
-- SQLite 3.35+ 支持 DROP COLUMN；更旧版本需重建表，这里不兜底。
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN is_verified;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN is_verified INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users
   SET is_verified = 1
 WHERE email_verified_at IS NOT NULL
    OR phone_verified_at IS NOT NULL;
-- +goose StatementEnd
