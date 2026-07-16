-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ALTER COLUMN password_hash TYPE VARCHAR(255);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users ALTER COLUMN password_hash TYPE VARCHAR(100);
-- +goose StatementEnd
