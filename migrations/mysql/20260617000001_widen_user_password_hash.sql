-- +goose Up
-- +goose StatementBegin
ALTER TABLE `users` MODIFY `password_hash` VARCHAR(255) NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `users` MODIFY `password_hash` VARCHAR(100) NOT NULL DEFAULT '';
-- +goose StatementEnd
