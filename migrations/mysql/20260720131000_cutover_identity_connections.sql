-- +goose Up
-- Irreversible hard cut: rollback requires restoring the mandatory database backup.
-- +goose StatementBegin
DROP TABLE IF EXISTS `user_social_accounts`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `user_identity_accounts`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `social_providers`;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `identity_providers`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
