-- +goose Up
-- +goose StatementBegin
ALTER TABLE `applications`
  ADD COLUMN `is_protected` BOOLEAN NOT NULL DEFAULT FALSE AFTER `is_verified`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `applications` DROP COLUMN `is_protected`;
-- +goose StatementEnd
