-- +goose Up
ALTER TABLE identity_connections ADD COLUMN show_globally BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE identity_connections SET show_globally = TRUE;
ALTER TABLE applications ADD COLUMN login_connections JSON NULL;

-- +goose Down
ALTER TABLE applications DROP COLUMN login_connections;
ALTER TABLE identity_connections DROP COLUMN show_globally;
