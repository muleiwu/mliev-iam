-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMP WITH TIME ZONE;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN phone_verified_at TIMESTAMP WITH TIME ZONE;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users
   SET email_verified_at = created_at
 WHERE is_verified = TRUE
   AND email IS NOT NULL
   AND email <> ''
   AND email_verified_at IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users
   SET phone_verified_at = created_at
 WHERE is_verified = TRUE
   AND phone IS NOT NULL
   AND phone <> ''
   AND phone_verified_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN phone_verified_at;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN email_verified_at;
-- +goose StatementEnd
