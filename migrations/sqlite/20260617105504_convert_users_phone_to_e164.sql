-- +goose Up
-- 存量手机号为中国大陆裸号，统一前缀 +86 转为 E.164
-- +goose StatementBegin
UPDATE users
   SET phone = '+86' || phone
 WHERE phone IS NOT NULL
   AND phone <> ''
   AND phone NOT LIKE '+%';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE users
   SET phone = SUBSTR(phone, 4)
 WHERE phone LIKE '+86%';
-- +goose StatementEnd
