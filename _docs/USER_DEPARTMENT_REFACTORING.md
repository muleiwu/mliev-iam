# 用户部门关联重构文档

## 概述

本次重构将用户和部门的关联关系从用户表中的直接外键改为使用独立的关联表（`user_departments`），以提升架构的灵活性，更好地支持 To C（面向消费者）和 To B（面向企业）业务场景。

## 重构目标

1. **保持用户表的独立性**：用户表不再依赖部门表，可以独立存在
2. **支持 To C 业务**：用户可以不属于任何部门
3. **支持 To B 业务**：用户可以属于一个或多个部门
4. **灵活的关联关系**：支持主部门、部门类型（成员/负责人）、职位等丰富的关联信息

## 架构变更

### 旧架构
```
User 表
├── id
├── username
├── email
├── department_id (外键)  ← 直接依赖部门
└── ...

Department 表
├── id
├── name
└── ...
```

### 新架构
```
User 表                    UserDepartment 表 (关联表)      Department 表
├── id                    ├── user_id (FK → User)         ├── id
├── username              ├── department_id (FK → Dept)   ├── name
├── email                 ├── position (职位)              └── ...
└── ...                   ├── type (成员/负责人)
                          ├── is_primary (是否主部门)
                          ├── is_active (是否激活)
                          └── ...
```

## 文件变更清单

### 1. 新增文件

#### 模型层
- **`app/model/user_department.go`**
  - 定义 `UserDepartment` 模型
  - 支持职位、部门类型（成员/负责人/副负责人）
  - 支持主部门标识、激活状态
  - 包含加入时间、离职时间等字段

#### 数据访问层
- **`app/dao/user_department_dao.go`**
  - 提供完整的 CRUD 操作
  - 支持按用户查询部门、按部门查询用户
  - 支持设置主部门、批量操作等

#### 数据传输层
- **`app/dto/user_department_dto.go`**
  - 定义创建、更新、查询 DTO
  - 定义响应 DTO 和转换函数

### 2. 修改文件

#### 模型层
- **`app/model/user.go`**
  - 移除 `DepartmentID` 字段
  - 注释掉直接关联，改为通过 `many2many` 标签说明关联方式

- **`app/model/department.go`**
  - 注释掉 `Users` 关联字段
  - 更新关联说明为通过 `user_departments` 表

#### 数据访问层
- **`app/dao/user_dao.go`**
  - 移除查询条件中的 `DepartmentID` 过滤
  - 修改 `GetUsersByDepartment` 方法，通过 JOIN `user_departments` 表查询

#### 业务逻辑层
- **`app/service/user_service.go`**
  - 创建用户时移除 `DepartmentID` 赋值
  - 更新用户时移除 `DepartmentID` 相关逻辑

#### 数据传输层
- **`app/dto/user_dto.go`**
  - 从所有 DTO 中移除 `DepartmentID` 字段
  - 包括：`CreateUserRequest`、`UpdateUserRequest`、`UserQueryRequest`、`UserResponse`

#### 中间件
- **`app/middleware/data_scope_middleware.go`**
  - 修改 `getUserDepartmentID` 函数，从 `user_departments` 表查询主部门
  - 修改数据权限过滤逻辑，通过子查询关联 `user_departments` 表
  - 优化循环以避免锁复制警告

#### 数据迁移
- **`internal/service/migration/migration.go`**
  - 创建用户时移除 `DepartmentID` 字段
  - 创建用户后，通过 `UserDepartment` 建立用户和部门的关联

- **`config/autoload/migration.go`**
  - 在迁移列表中添加 `UserDepartment` 模型

## 数据库变更

### 新表结构

```sql
CREATE TABLE `user_departments` (
  `user_id` bigint unsigned NOT NULL,
  `department_id` bigint unsigned NOT NULL,
  `position` varchar(100) DEFAULT NULL COMMENT '职位',
  `title` varchar(100) DEFAULT NULL COMMENT '职称',
  `type` tinyint NOT NULL DEFAULT '0' COMMENT '类型: 0-成员 1-负责人 2-副负责人',
  `is_active` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否激活',
  `is_primary` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否主部门',
  `sort` int NOT NULL DEFAULT '0' COMMENT '排序',
  `joined_at` datetime DEFAULT NULL COMMENT '加入时间',
  `left_at` datetime DEFAULT NULL COMMENT '离开时间',
  `attributes` json DEFAULT NULL COMMENT '扩展属性',
  `remark` text COMMENT '备注',
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`user_id`, `department_id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_department_id` (`department_id`),
  KEY `idx_deleted_at` (`deleted_at`),
  CONSTRAINT `fk_user_departments_user` 
    FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) 
    ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT `fk_user_departments_department` 
    FOREIGN KEY (`department_id`) REFERENCES `departments` (`id`) 
    ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 表结构变更

```sql
-- users 表
ALTER TABLE `users` DROP COLUMN `department_id`;
```

## API 影响分析

### 不受影响的 API
- 用户登录/登出
- 用户信息查询（基础信息）
- 角色权限相关 API

### 需要适配的 API

#### 1. 用户创建 API
**变更前：**
```json
POST /api/admin/users
{
  "username": "user001",
  "email": "user@example.com",
  "departmentId": 123
}
```

**变更后：**
```json
// Step 1: 创建用户
POST /api/admin/users
{
  "username": "user001",
  "email": "user@example.com"
}

// Step 2: 关联部门（可选）
POST /api/admin/user-departments
{
  "userId": 1001,
  "departmentId": 123,
  "position": "工程师",
  "type": 0,
  "isPrimary": true
}
```

#### 2. 用户更新 API
**变更：** 移除 `departmentId` 字段，部门变更通过专门的部门关联 API 处理

#### 3. 新增部门关联管理 API

```
# 用户部门关联管理
POST   /api/admin/user-departments          # 创建用户部门关联
PUT    /api/admin/user-departments/:id      # 更新关联信息
DELETE /api/admin/user-departments/:id      # 删除关联
GET    /api/admin/user-departments          # 查询关联列表
GET    /api/admin/users/:id/departments     # 获取用户的所有部门
GET    /api/admin/departments/:id/users     # 获取部门的所有用户
PUT    /api/admin/users/:id/primary-dept    # 设置主部门
```

## 数据迁移方案

### 迁移步骤

1. **备份现有数据**
   ```sql
   -- 备份用户表
   CREATE TABLE users_backup AS SELECT * FROM users;
   ```

2. **创建新表**
   ```sql
   -- 通过 GORM AutoMigrate 自动创建 user_departments 表
   ```

3. **迁移现有数据**
   ```sql
   -- 将现有的用户-部门关系迁移到新表
   INSERT INTO user_departments (
     user_id, department_id, type, is_active, is_primary, 
     joined_at, created_at, updated_at
   )
   SELECT 
     id, department_id, 0, 1, 1, 
     created_at, NOW(), NOW()
   FROM users 
   WHERE department_id IS NOT NULL;
   ```

4. **删除旧字段**
   ```sql
   -- 删除 users 表中的 department_id 字段
   ALTER TABLE users DROP COLUMN department_id;
   ```

### 回滚方案

如需回滚：
```sql
-- 1. 添加回 department_id 字段
ALTER TABLE users ADD COLUMN department_id bigint unsigned NULL;

-- 2. 从关联表恢复数据
UPDATE users u 
SET department_id = (
  SELECT department_id 
  FROM user_departments ud 
  WHERE ud.user_id = u.id AND ud.is_primary = 1
  LIMIT 1
);

-- 3. 删除新表
DROP TABLE user_departments;
```

## 业务场景支持

### To C 场景
```go
// 创建用户，不关联任何部门
user := &model.User{
    Username: "consumer001",
    Email: "consumer@example.com",
    // 无需设置部门
}
db.Create(user)
```

### To B 场景
```go
// 1. 创建用户
user := &model.User{
    Username: "employee001",
    Email: "employee@company.com",
}
db.Create(user)

// 2. 关联到部门（单部门）
userDept := &model.UserDepartment{
    UserID: user.ID,
    DepartmentID: deptID,
    Position: "软件工程师",
    Type: model.UserDepartmentTypeMember,
    IsPrimary: true,
    IsActive: true,
}
db.Create(userDept)

// 3. 或关联到多个部门
userDepts := []model.UserDepartment{
    {UserID: user.ID, DepartmentID: dept1, IsPrimary: true},
    {UserID: user.ID, DepartmentID: dept2, IsPrimary: false},
}
db.Create(&userDepts)
```

## 注意事项

1. **数据一致性**
   - 删除用户时，关联的 `user_departments` 记录会自动级联删除（CASCADE）
   - 删除部门时，需要先处理关联的用户或设置为 NULL

2. **查询性能**
   - 添加了必要的索引：`user_id`、`department_id`
   - 数据权限查询使用子查询，注意大数据量场景的性能

3. **主部门逻辑**
   - 每个用户只能有一个主部门（`is_primary = true`）
   - 设置新主部门时，自动取消其他部门的主部门标识

4. **兼容性**
   - 现有的用户-部门数据需要通过迁移脚本转移到新表
   - 前端需要更新创建/编辑用户的逻辑

## 测试建议

1. **单元测试**
   - `UserDepartmentDao` 的所有方法
   - 数据权限中间件的部门过滤逻辑

2. **集成测试**
   - 用户创建 + 部门关联流程
   - 多部门用户的数据权限测试
   - 主部门设置和切换

3. **性能测试**
   - 大量用户的部门查询性能
   - 数据权限过滤的查询性能

## 后续优化建议

1. **缓存优化**
   - 缓存用户的主部门信息
   - 缓存用户的所有部门列表

2. **功能扩展**
   - 支持部门角色（部门内的不同角色）
   - 支持部门权限（基于部门的细粒度权限）
   - 支持部门审批流程

3. **API 完善**
   - 添加批量设置用户部门的 API
   - 添加部门人员调动的 API
   - 添加部门变更历史查询

## 总结

本次重构通过引入独立的用户部门关联表，实现了：
- ✅ 用户表的独立性，不依赖部门
- ✅ 灵活支持 To C 和 To B 业务场景
- ✅ 支持用户多部门归属
- ✅ 丰富的关联信息（职位、类型、主部门等）
- ✅ 保持数据一致性和引用完整性
- ✅ 向后兼容的迁移方案

这种设计模式也为未来可能的组织架构扩展（如矩阵式组织、虚拟团队等）提供了良好的基础。

