-- 从 MySQL 迁移数据到 PostgreSQL 后重置各表自增序列。
-- 带显式 id 导入的数据不会推进 BIGSERIAL 序列，不重置会导致后续 INSERT 主键冲突。
-- 用法: psql -d <dbname> -f scripts/pg_reset_sequences.sql
--
-- 复合主键中间表无序列，无需处理: user_roles / user_departments / role_permissions

SELECT setval(pg_get_serial_sequence('users', 'id'), COALESCE((SELECT MAX(id) FROM users), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('departments', 'id'), COALESCE((SELECT MAX(id) FROM departments), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('roles', 'id'), COALESCE((SELECT MAX(id) FROM roles), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('permissions', 'id'), COALESCE((SELECT MAX(id) FROM permissions), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('applications', 'id'), COALESCE((SELECT MAX(id) FROM applications), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('application_oidc_keys', 'id'), COALESCE((SELECT MAX(id) FROM application_oidc_keys), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('authorization_codes', 'id'), COALESCE((SELECT MAX(id) FROM authorization_codes), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('oidc_tokens', 'id'), COALESCE((SELECT MAX(id) FROM oidc_tokens), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('social_providers', 'id'), COALESCE((SELECT MAX(id) FROM social_providers), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('user_social_accounts', 'id'), COALESCE((SELECT MAX(id) FROM user_social_accounts), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('identity_providers', 'id'), COALESCE((SELECT MAX(id) FROM identity_providers), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('user_identity_accounts', 'id'), COALESCE((SELECT MAX(id) FROM user_identity_accounts), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('settings', 'id'), COALESCE((SELECT MAX(id) FROM settings), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('captcha_configs', 'id'), COALESCE((SELECT MAX(id) FROM captcha_configs), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('agreements', 'id'), COALESCE((SELECT MAX(id) FROM agreements), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('agreement_acceptances', 'id'), COALESCE((SELECT MAX(id) FROM agreement_acceptances), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('migration_sources', 'id'), COALESCE((SELECT MAX(id) FROM migration_sources), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('legacy_users', 'id'), COALESCE((SELECT MAX(id) FROM legacy_users), 0) + 1, false);
SELECT setval(pg_get_serial_sequence('migration_tasks', 'id'), COALESCE((SELECT MAX(id) FROM migration_tasks), 0) + 1, false);
