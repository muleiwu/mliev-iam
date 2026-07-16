-- +goose Up
-- +goose StatementBegin
-- SQLite 不强制 VARCHAR 长度，password_hash 已可存储任意长度（argon2id 编码哈希）。
-- 此处为保持三方言迁移版本号一致而占位，无需实际变更列。
SELECT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
