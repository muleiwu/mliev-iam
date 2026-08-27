-- +goose Up
-- +goose StatementBegin
ALTER TABLE applications
  ADD COLUMN is_protected INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE applications DROP COLUMN is_protected;
-- +goose StatementEnd
