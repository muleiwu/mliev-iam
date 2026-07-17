-- +goose Up
-- +goose StatementBegin
CREATE TABLE attachments (
  id BIGSERIAL PRIMARY KEY,
  uuid VARCHAR(36) NOT NULL,
  file_name VARCHAR(255) NOT NULL,
  file_size BIGINT NOT NULL DEFAULT 0,
  mime_type VARCHAR(100) NOT NULL,
  storage_type VARCHAR(20) NOT NULL DEFAULT 's3',
  storage_key VARCHAR(500) NOT NULL,
  biz_type VARCHAR(50) NOT NULL DEFAULT 'avatar',
  uploader_user_id BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NULL,
  updated_at TIMESTAMPTZ NULL,
  deleted_at TIMESTAMPTZ NULL,
  CONSTRAINT uk_attachments_uuid UNIQUE (uuid)
);
CREATE INDEX idx_attachments_uploader ON attachments (uploader_user_id);
CREATE INDEX idx_attachments_deleted_at ON attachments (deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS attachments;
-- +goose StatementEnd
