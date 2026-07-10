# PostgreSQL 自增序列重置指南

## 背景

从 MySQL 迁移数据到 PostgreSQL 时，数据通常带着显式 `id` 值插入。PostgreSQL 的 `BIGSERIAL` 序列不会因显式插入而推进，仍停留在初始值 `1`。之后应用通过 GORM 正常插入新记录时，序列生成的 `id` 会与已导入的数据冲突，报错：

```
ERROR: duplicate key value violates unique constraint "xxx_pkey"
```

因此**每次带 ID 导入数据后，必须重置相关表的序列**。

## 方式一：逐表脚本（本项目专用）

项目内已提供 [`scripts/pg_reset_sequences.sql`](../scripts/pg_reset_sequences.sql)，覆盖全部 19 张带 `BIGSERIAL` 主键的表：

```bash
psql -d <数据库名> -f scripts/pg_reset_sequences.sql
```

以下 3 张复合主键中间表没有自增列和序列，无需处理：

- `user_roles`（UserID + RoleID）
- `user_departments`（UserID + DepartmentID）
- `role_permissions`（RoleID + PermissionID）

## 方式二：通用写法（推荐）

一条 `DO` 块自动发现 `public` schema 下所有 serial/bigserial 列并重置序列，**新增表后无需维护脚本**：

```sql
DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT c.relname AS tbl, a.attname AS col,
           pg_get_serial_sequence(c.relname, a.attname) AS seq
    FROM pg_class c
    JOIN pg_attribute a ON a.attrelid = c.oid
    WHERE c.relkind = 'r'
      AND c.relnamespace = 'public'::regnamespace
      AND pg_get_serial_sequence(c.relname, a.attname) IS NOT NULL
  LOOP
    EXECUTE format(
      'SELECT setval(%L, COALESCE((SELECT MAX(%I) FROM %I), 0) + 1, false)',
      r.seq, r.col, r.tbl);
  END LOOP;
END $$;
```

## setval 写法说明

单表的等价语句为：

```sql
SELECT setval(
  pg_get_serial_sequence('users', 'id'),
  COALESCE((SELECT MAX(id) FROM users), 0) + 1,
  false
);
```

- `pg_get_serial_sequence(表名, 列名)`：查出该列绑定的序列名，无需硬编码 `users_id_seq`。
- `MAX(id) + 1` 配合第三个参数 `false`（is_called=false）：下一次 `nextval` 恰好返回 `MAX(id) + 1`。
- `COALESCE(..., 0)`：空表时结果为 `setval(seq, 1, false)`，下一个 `nextval` 返回 `1`，同样正确。所以对空表批量执行是安全的。

## 验证

重置后可抽查序列当前值是否等于 `MAX(id) + 1`：

```sql
SELECT last_value, is_called FROM users_id_seq;
SELECT MAX(id) FROM users;
```

或直接查看所有序列状态：

```sql
SELECT schemaname, sequencename, last_value
FROM pg_sequences
WHERE schemaname = 'public'
ORDER BY sequencename;
```
