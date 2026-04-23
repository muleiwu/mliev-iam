-- +goose Up
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN is_verified;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN is_verified BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users
   SET is_verified = TRUE
 WHERE email_verified_at IS NOT NULL
    OR phone_verified_at IS NOT NULL;
-- +goose StatementEnd
