-- +goose Up
-- +goose StatementBegin
ALTER TABLE `users` ADD COLUMN `uuid` VARCHAR(36) NOT NULL DEFAULT '' AFTER `id`;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX `idx_users_uuid` ON `users` (`uuid`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX `idx_users_uuid` ON `users`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `users` DROP COLUMN `uuid`;
-- +goose StatementEnd
