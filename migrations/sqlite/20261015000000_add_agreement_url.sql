-- +goose Up
ALTER TABLE agreements ADD COLUMN content_type VARCHAR(20) NOT NULL DEFAULT 'markdown';
ALTER TABLE agreements ADD COLUMN content_url VARCHAR(2048) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agreements DROP COLUMN content_url;
ALTER TABLE agreements DROP COLUMN content_type;
