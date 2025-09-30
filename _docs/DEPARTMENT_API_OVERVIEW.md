# 部门管理API接口概览

本文档提供部门管理模块所有API接口的快速概览。

## 📋 接口列表

### 基础CRUD操作

| 方法 | 路径 | 描述 | 权限 |
|------|------|------|------|
| POST | `/api/admin/departments` | 创建部门 | 管理员 |
| GET | `/api/admin/departments` | 获取部门列表 | 管理员 |
| GET | `/api/admin/departments/{id}` | 获取部门详情 | 管理员 |
| PUT | `/api/admin/departments/{id}` | 更新部门信息 | 管理员 |
| DELETE | `/api/admin/departments/{id}` | 删除部门 | 管理员 |

### 树形结构管理

| 方法 | 路径 | 描述 | 权限 |
|------|------|------|------|
| GET | `/api/admin/departments/tree` | 获取部门树形结构 | 管理员 |
| GET | `/api/admin/departments/roots` | 获取根部门列表 | 管理员 |
| GET | `/api/admin/departments/{id}/children` | 获取子部门列表 | 管理员 |
| GET | `/api/admin/departments/{id}/descendants` | 获取所有后代部门 | 管理员 |
| PUT | `/api/admin/departments/{id}/move` | 移动部门 | 管理员 |

### 状态和批量管理

| 方法 | 路径 | 描述 | 权限 |
|------|------|------|------|
| PUT | `/api/admin/departments/{id}/status` | 更新部门状态 | 管理员 |
| POST | `/api/admin/departments/batch` | 批量操作部门 | 管理员 |

### 统计和查询

| 方法 | 路径 | 描述 | 权限 |
|------|------|------|------|
| GET | `/api/admin/departments/stats/active-count` | 获取启用部门数量统计 | 管理员 |
| GET | `/api/admin/managers/{managerId}/departments` | 根据负责人获取部门列表 | 管理员 |
| GET | `/api/admin/departments/{departmentId}/users` | 获取部门用户列表 | 管理员 |

### 维护工具

| 方法 | 路径 | 描述 | 权限 |
|------|------|------|------|
| POST | `/api/admin/departments/validate-hierarchy` | 验证部门层级结构 | 管理员 |
| POST | `/api/admin/departments/repair-hierarchy` | 修复部门层级结构 | 管理员 |

## 🔍 详细说明

### 1. 创建部门 - `POST /api/admin/departments`

**功能**: 创建新部门，支持层级结构

**主要参数**:
- `name` - 部门名称（必填）
- `code` - 部门编码（必填，唯一）
- `parentId` - 父部门ID（可选）
- `managerId` - 负责人ID（可选）
- `type` - 部门类型（0:公司 1:事业部 2:部门 3:团队）
- `status` - 状态（0:禁用 1:启用 2:冻结）

**业务规则**:
- 部门编码必须全局唯一
- 同级部门名称不能重复
- 父部门必须是启用状态才能创建子部门

### 2. 获取部门列表 - `GET /api/admin/departments`

**功能**: 分页查询部门列表，支持多条件筛选

**查询参数**:
- `page`, `pageSize` - 分页参数
- `name`, `code` - 模糊查询
- `type`, `status` - 精确筛选
- `parentId`, `managerId` - 关联查询
- `keyword` - 关键词搜索
- `includeTree` - 是否返回树形结构

### 3. 更新部门信息 - `PUT /api/admin/departments/{id}`

**功能**: 更新指定部门的基本信息

**业务规则**:
- 编码修改时检查唯一性
- 名称修改时检查同级唯一性
- 支持部分字段更新

### 4. 删除部门 - `DELETE /api/admin/departments/{id}`

**功能**: 软删除指定部门

**业务规则**:
- 存在子部门时无法删除
- 存在关联用户时无法删除
- 执行软删除，数据可恢复

### 5. 获取部门树形结构 - `GET /api/admin/departments/tree`

**功能**: 获取完整的部门层级树

**特点**:
- 只返回启用状态的部门
- 按照层级和排序值组织
- 包含完整的父子关系

### 6. 移动部门 - `PUT /api/admin/departments/{id}/move`

**功能**: 将部门移动到新的层级位置

**业务规则**:
- 不能移动到自己的子部门下
- 目标父部门必须是启用状态
- 移动后自动重新计算层级路径
- 检查目标位置的名称冲突

### 7. 批量操作 - `POST /api/admin/departments/batch`

**功能**: 对多个部门执行批量操作

**支持操作**:
- `enable` - 批量启用
- `disable` - 批量禁用
- `freeze` - 批量冻结
- `delete` - 批量删除

### 8. 状态更新 - `PUT /api/admin/departments/{id}/status`

**功能**: 单独更新部门状态

**状态类型**:
- `0` - 禁用：部门暂停使用
- `1` - 启用：部门正常使用
- `2` - 冻结：部门临时冻结

### 9. 层级结构维护

**验证层级结构** - `POST /api/admin/departments/validate-hierarchy`
- 检查路径和层级的正确性
- 验证父子关系的一致性

**修复层级结构** - `POST /api/admin/departments/repair-hierarchy`
- 自动重新计算所有部门的路径和层级
- 修复数据不一致问题

## 📊 数据模型

### 部门实体主要字段

```yaml
DepartmentResponse:
  id: 部门ID
  name: 部门名称
  code: 部门编码
  type: 部门类型 (0-3)
  status: 部门状态 (0-2)
  parentId: 父部门ID
  level: 层级深度
  path: 层级路径
  managerId: 负责人ID
  # ... 其他业务字段
```

### 查询参数模型

```yaml
DepartmentQueryRequest:
  page: 页码
  pageSize: 每页数量
  name: 部门名称筛选
  code: 部门编码筛选
  type: 部门类型筛选
  status: 状态筛选
  parentId: 父部门筛选
  keyword: 关键词搜索
  includeTree: 是否树形结构
```

## ⚠️ 注意事项

### 权限要求
- 所有接口都需要管理员权限
- 需要有效的JWT认证token

### 性能考虑
- 树形结构查询对大数据量可能较慢
- 建议合理使用分页参数
- 层级修复操作是重操作，谨慎使用

### 数据完整性
- 删除操作会检查关联数据
- 移动操作会验证业务规则
- 批量操作支持事务回滚

### 错误处理
- 详细的错误信息返回
- 标准的HTTP状态码
- 业务错误码映射

## 🔗 相关资源

- [完整API文档](./department_api.yaml) - OpenAPI 3.0规范
- [使用说明](./README.md) - 详细的使用指南
- [项目规范](./PROJECT_SPECIFICATION.md) - 项目整体规范
