-- +goose Up
ALTER TABLE login_sessions ADD COLUMN remember_me BOOLEAN NULL;
ALTER TABLE login_sessions ADD COLUMN activation_pending BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE login_sessions DROP COLUMN activation_pending;
ALTER TABLE login_sessions DROP COLUMN remember_me;
