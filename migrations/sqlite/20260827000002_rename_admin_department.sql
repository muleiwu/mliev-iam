-- +goose Up
-- +goose StatementBegin
UPDATE departments
SET name = '系统管理部门',
    short_name = '系统管理',
    description = '系统管理部门'
WHERE code = 'admin'
  AND name = '管理员'
  AND remark = '系统默认管理员部门'
  AND deleted_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE departments
SET name = '管理员',
    short_name = '管理',
    description = '系统管理员部门'
WHERE code = 'admin'
  AND name = '系统管理部门'
  AND remark = '系统默认管理员部门'
  AND deleted_at IS NULL;
-- +goose StatementEnd
