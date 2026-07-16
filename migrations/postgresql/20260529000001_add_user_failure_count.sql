-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN failure_count BIGINT NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN failure_count;
-- +goose StatementEnd
