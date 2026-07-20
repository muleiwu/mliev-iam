-- +goose Up
-- +goose StatementBegin
ALTER TABLE `applications`
  ADD COLUMN `icon` VARCHAR(255) NOT NULL DEFAULT '' AFTER `description`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `applications` DROP COLUMN `icon`;
-- +goose StatementEnd
