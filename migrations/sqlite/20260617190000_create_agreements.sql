-- +goose Up
-- +goose StatementBegin
CREATE TABLE agreements (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  scope VARCHAR(20) NOT NULL,
  application_id INTEGER NOT NULL DEFAULT 0,
  code VARCHAR(100) NOT NULL,
  title VARCHAR(200) NOT NULL,
  content_md TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  sort INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NULL,
  updated_at DATETIME NULL,
  deleted_at DATETIME NULL,
  CONSTRAINT uk_agreements_scope_app_code UNIQUE (scope, application_id, code, deleted_at)
);
CREATE INDEX idx_agreements_scope_app_enabled ON agreements (scope, application_id, enabled);
CREATE INDEX idx_agreements_sort ON agreements (sort);
CREATE INDEX idx_agreements_deleted_at ON agreements (deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE agreement_acceptances (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  application_id INTEGER NOT NULL DEFAULT 0,
  agreement_id INTEGER NOT NULL,
  scope VARCHAR(20) NOT NULL,
  code VARCHAR(100) NOT NULL,
  title VARCHAR(200) NOT NULL,
  content_sha256 VARCHAR(64) NOT NULL,
  accepted_at DATETIME NOT NULL,
  created_at DATETIME NULL
);
CREATE INDEX idx_agreement_acceptances_user_id ON agreement_acceptances (user_id);
CREATE INDEX idx_agreement_acceptances_application_id ON agreement_acceptances (application_id);
CREATE INDEX idx_agreement_acceptances_agreement_id ON agreement_acceptances (agreement_id);
CREATE INDEX idx_agreement_acceptances_scope_code ON agreement_acceptances (scope, code);
CREATE INDEX idx_agreement_acceptances_accepted_at ON agreement_acceptances (accepted_at);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO agreements (scope, application_id, code, title, content_md, enabled, sort, created_at, updated_at)
VALUES
  ('global', 0, 'terms', '服务协议', '', FALSE, 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
  ('global', 0, 'privacy', '隐私协议', '', FALSE, 20, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agreement_acceptances;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS agreements;
-- +goose StatementEnd
