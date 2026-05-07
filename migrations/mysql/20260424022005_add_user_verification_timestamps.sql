-- +goose Up
-- +goose StatementBegin
ALTER TABLE `users`
  ADD COLUMN `email_verified_at` DATETIME NULL DEFAULT NULL AFTER `is_verified`,
  ADD COLUMN `phone_verified_at` DATETIME NULL DEFAULT NULL AFTER `email_verified_at`;
-- +goose StatementEnd

-- 历史数据回填：is_verified=true 的用户按 created_at 追认验证时间
-- +goose StatementBegin
UPDATE `users`
   SET `email_verified_at` = `created_at`
 WHERE `is_verified` = TRUE
   AND `email` IS NOT NULL
   AND `email` <> ''
   AND `email_verified_at` IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `users`
   SET `phone_verified_at` = `created_at`
 WHERE `is_verified` = TRUE
   AND `phone` IS NOT NULL
   AND `phone` <> ''
   AND `phone_verified_at` IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `users` DROP COLUMN `phone_verified_at`;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE `users` DROP COLUMN `email_verified_at`;
-- +goose StatementEnd
