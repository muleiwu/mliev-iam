-- +goose Up
-- 旧 is_verified bool 列在 20260424022005 把回填逻辑跑完后已完全被
-- email_verified_at / phone_verified_at 替代；业务代码自这次迁移起不再
-- 读写此列。此处正式 DROP COLUMN 收尾。
-- +goose StatementBegin
ALTER TABLE `users` DROP COLUMN `is_verified`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `users` ADD COLUMN `is_verified` BOOLEAN NOT NULL DEFAULT FALSE AFTER `is_activated`;
-- +goose StatementEnd

-- 回滚：按 email_verified_at / phone_verified_at 任一非空反推 is_verified=1
-- +goose StatementBegin
UPDATE `users`
   SET `is_verified` = TRUE
 WHERE `email_verified_at` IS NOT NULL
    OR `phone_verified_at` IS NOT NULL;
-- +goose StatementEnd
