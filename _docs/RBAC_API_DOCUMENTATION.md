# RBAC权限管理系统API接口文档

## 📋 接口概览

本文档描述了RBAC权限管理系统的所有API接口，包括角色管理、权限管理和用户角色管理等功能模块。

### 基础信息
- **Base URL**: `http://localhost:8080/api/admin`
- **认证方式**: JWT Token (Header: `Authorization: Bearer <token>`)
- **内容类型**: `application/json`
- **字符编码**: `UTF-8`

### 通用响应格式

#### 成功响应
```json
{
  "code": 200,
  "message": "操作成功",
  "data": {
    // 响应数据
  }
}
```

#### 错误响应
```json
{
  "code": 400,
  "message": "错误描述",
  "data": null
}
```

#### 分页响应
```json
{
  "code": 200,
  "message": "查询成功",
  "data": {
    "total": 100,
    "page": 1,
    "size": 20,
    "items": [
      // 数据列表
    ]
  }
}
```

### 状态码说明
- `200` - 操作成功
- `400` - 请求参数错误
- `401` - 未授权访问
- `403` - 权限不足
- `404` - 资源不存在
- `500` - 服务器内部错误

---

## 🔐 角色管理接口

### 1. 创建角色

**接口地址**: `POST /roles`

**请求参数**:
```json
{
  "name": "管理员",
  "code": "admin",
  "description": "系统管理员角色",
  "type": 0,
  "parentId": null,
  "dataScope": 1,
  "scopeDesc": "全部数据权限",
  "sort": 1,
  "remark": "备注信息",
  "attributes": {}
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 角色名称 (2-50字符) |
| code | string | 是 | 角色代码 (2-50字符，唯一) |
| description | string | 否 | 角色描述 |
| type | integer | 否 | 角色类型 (0:系统角色, 1:自定义角色) |
| parentId | integer | 否 | 父角色ID |
| dataScope | integer | 否 | 数据权限范围 (1-4) |
| scopeDesc | string | 否 | 数据权限描述 |
| sort | integer | 否 | 排序号 |
| remark | string | 否 | 备注 |
| attributes | object | 否 | 扩展属性 |

**响应示例**:
```json
{
  "code": 200,
  "message": "角色创建成功",
  "data": {
    "id": 1,
    "name": "管理员",
    "code": "admin",
    "description": "系统管理员角色",
    "type": 0,
    "typeName": "system",
    "status": 1,
    "statusName": "enabled",
    "parentId": null,
    "level": 1,
    "path": "/1",
    "dataScope": 1,
    "scopeDesc": "全部数据权限",
    "createdAt": "2025-10-10T10:00:00Z",
    "updatedAt": "2025-10-10T10:00:00Z",
    "userCount": 0
  }
}
```

### 2. 获取角色列表

**接口地址**: `GET /roles`

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | integer | 否 | 页码 (默认1) |
| pageSize | integer | 否 | 每页大小 (默认10) |
| name | string | 否 | 角色名称 (模糊查询) |
| code | string | 否 | 角色代码 (模糊查询) |
| type | integer | 否 | 角色类型 |
| status | integer | 否 | 角色状态 |
| parentId | integer | 否 | 父角色ID |
| orderBy | string | 否 | 排序字段 |

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": {
    "total": 50,
    "page": 1,
    "size": 10,
    "items": [
      {
        "id": 1,
        "name": "管理员",
        "code": "admin",
        "type": 0,
        "typeName": "system",
        "status": 1,
        "statusName": "enabled",
        "dataScope": 1,
        "userCount": 5,
        "createdAt": "2025-10-10T10:00:00Z"
      }
    ]
  }
}
```

### 3. 获取角色树形结构

**接口地址**: `GET /roles/tree`

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": {
    "roles": [
      {
        "id": 1,
        "name": "管理员",
        "code": "admin",
        "children": [
          {
            "id": 2,
            "name": "部门管理员",
            "code": "dept_admin",
            "children": []
          }
        ]
      }
    ]
  }
}
```

### 4. 获取角色详情

**接口地址**: `GET /roles/{id}`

**路径参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | integer | 是 | 角色ID |

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": {
    "id": 1,
    "name": "管理员",
    "code": "admin",
    "description": "系统管理员角色",
    "type": 0,
    "status": 1,
    "parentId": null,
    "level": 1,
    "path": "/1",
    "dataScope": 1,
    "userCount": 5,
    "parent": null,
    "children": [
      {
        "id": 2,
        "name": "部门管理员",
        "code": "dept_admin"
      }
    ],
    "permissions": [
      {
        "id": 1,
        "name": "用户管理",
        "code": "user:manage"
      }
    ]
  }
}
```

### 5. 更新角色

**接口地址**: `PUT /roles/{id}`

**请求参数**:
```json
{
  "name": "高级管理员",
  "description": "高级系统管理员",
  "dataScope": 2,
  "scopeDesc": "本部门数据权限",
  "remark": "更新后的备注"
}
```

### 6. 删除角色

**接口地址**: `DELETE /roles/{id}`

**响应示例**:
```json
{
  "code": 200,
  "message": "角色删除成功",
  "data": null
}
```

### 7. 更新角色状态

**接口地址**: `PUT /roles/{id}/status`

**请求参数**:
```json
{
  "status": 1,
  "remark": "启用角色"
}
```

### 8. 批量操作角色

**接口地址**: `POST /roles/batch`

**请求参数**:
```json
{
  "roleIds": [1, 2, 3],
  "action": "enable",
  "remark": "批量启用角色"
}
```

**操作类型**:
- `enable` - 启用
- `disable` - 禁用  
- `delete` - 删除

### 9. 分配权限

**接口地址**: `POST /roles/{id}/permissions`

**请求参数**:
```json
{
  "permissionIds": [1, 2, 3, 4, 5]
}
```

### 10. 获取角色权限

**接口地址**: `GET /roles/{id}/permissions`

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": [
    {
      "id": 1,
      "name": "用户管理",
      "code": "user:manage",
      "type": 0,
      "typeName": "menu",
      "resource": "user",
      "action": "manage"
    }
  ]
}
```

### 11. 获取角色统计

**接口地址**: `GET /roles/stats`

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功", 
  "data": {
    "totalRoles": 50,
    "enabledRoles": 45,
    "disabledRoles": 5,
    "systemRoles": 5,
    "customRoles": 45
  }
}
```

---

## 🛡️ 权限管理接口

### 1. 创建权限

**接口地址**: `POST /permissions`

**请求参数**:
```json
{
  "name": "用户管理",
  "code": "user:manage",
  "description": "用户管理权限",
  "type": 0,
  "parentId": null,
  "resource": "user",
  "action": "manage",
  "method": "GET",
  "url": "/api/admin/users",
  "component": "/user/index",
  "icon": "user",
  "redirect": "",
  "external": false,
  "hidden": false,
  "keepAlive": true,
  "showInMenu": true,
  "sort": 1,
  "remark": "用户管理权限"
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 权限名称 |
| code | string | 是 | 权限代码 (唯一) |
| description | string | 否 | 权限描述 |
| type | integer | 是 | 权限类型 (0:菜单,1:按钮,2:API,3:数据) |
| parentId | integer | 否 | 父权限ID |
| resource | string | 否 | 资源标识符 |
| action | string | 否 | 操作类型 |
| method | string | 否 | HTTP方法 |
| url | string | 否 | API路径 |
| component | string | 否 | 前端组件路径 |
| icon | string | 否 | 图标 |
| redirect | string | 否 | 重定向路径 |
| external | boolean | 否 | 是否外部链接 |
| hidden | boolean | 否 | 是否隐藏 |
| keepAlive | boolean | 否 | 是否缓存 |
| showInMenu | boolean | 否 | 是否在菜单中显示 |
| sort | integer | 否 | 排序号 |
| remark | string | 否 | 备注 |

### 2. 获取权限列表

**接口地址**: `GET /permissions`

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | integer | 否 | 页码 |
| pageSize | integer | 否 | 每页大小 |
| name | string | 否 | 权限名称 |
| code | string | 否 | 权限代码 |
| type | integer | 否 | 权限类型 |
| status | integer | 否 | 权限状态 |
| parentId | integer | 否 | 父权限ID |
| resource | string | 否 | 资源标识符 |

### 3. 获取权限树形结构

**接口地址**: `GET /permissions/tree`

### 4. 获取菜单树形结构

**接口地址**: `GET /permissions/menu-tree`

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": {
    "menus": [
      {
        "id": 1,
        "name": "系统管理",
        "code": "system",
        "path": "/system",
        "component": "/system/index",
        "icon": "system",
        "children": [
          {
            "id": 2,
            "name": "用户管理",
            "code": "user:manage",
            "path": "/system/user",
            "component": "/system/user/index",
            "icon": "user"
          }
        ]
      }
    ]
  }
}
```

### 5. 其他权限接口

权限管理的其他接口（获取详情、更新、删除、状态管理、批量操作、统计信息）与角色管理接口类似，请参考角色管理接口文档。

---

## 👥 用户角色管理接口

### 1. 分配角色

**接口地址**: `POST /users/{id}/roles/{roleId}`

**请求参数**:
```json
{
  "isPrimary": false,
  "departmentId": 1,
  "organizationId": null,
  "applyScope": 1,
  "dataScope": 2,
  "expiresAt": "2025-12-31T23:59:59Z",
  "constraints": {
    "maxUsers": 100
  },
  "remark": "分配角色备注"
}
```

### 2. 移除角色

**接口地址**: `DELETE /users/{id}/roles/{roleId}`

### 3. 获取用户角色

**接口地址**: `GET /users/{id}/roles`

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": [
    {
      "userId": 1,
      "roleId": 1,
      "assignedBy": 1,
      "assignedAt": "2025-10-10T10:00:00Z",
      "expiresAt": null,
      "isActive": true,
      "isPrimary": true,
      "dataScope": 1,
      "dataScopeName": "全部数据",
      "approvalStatus": 1,
      "approvalStatusName": "已批准",
      "isExpired": false,
      "isValid": true,
      "role": {
        "id": 1,
        "name": "管理员",
        "code": "admin"
      }
    }
  ]
}
```

### 4. 获取用户及角色信息

**接口地址**: `GET /users/{id}/with-roles`

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": {
    "id": 1,
    "username": "admin",
    "email": "admin@example.com",
    "nickname": "管理员",
    "roles": [
      {
        "id": 1,
        "name": "管理员",
        "code": "admin"
      }
    ],
    "primaryRole": {
      "id": 1,
      "name": "管理员", 
      "code": "admin"
    },
    "roleCount": 1
  }
}
```

### 5. 批量分配角色

**接口地址**: `POST /users/{id}/roles/batch`

**请求参数**:
```json
{
  "roleIds": [1, 2, 3],
  "remark": "批量分配角色"
}
```

### 6. 批量移除角色

**接口地址**: `DELETE /users/{id}/roles/batch`

**请求参数**:
```json
{
  "roleIds": [2, 3],
  "remark": "批量移除角色"
}
```

### 7. 设置主要角色

**接口地址**: `PUT /users/{id}/roles/{roleId}/primary`

### 8. 获取用户权限

**接口地址**: `GET /users/{id}/permissions`

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": [
    {
      "id": 1,
      "name": "用户管理",
      "code": "user:manage",
      "type": 0,
      "typeName": "menu",
      "resource": "user",
      "action": "manage",
      "url": "/api/admin/users",
      "method": "GET"
    }
  ]
}
```

### 9. 获取用户菜单树

**接口地址**: `GET /users/{id}/menu-tree`

**响应示例**:
```json
{
  "code": 200,
  "message": "查询成功",
  "data": {
    "menus": [
      {
        "id": 1,
        "name": "系统管理",
        "path": "/system",
        "component": "/system/index",
        "icon": "system",
        "children": [
          {
            "id": 2,
            "name": "用户管理",
            "path": "/system/user",
            "component": "/system/user/index",
            "icon": "user"
          }
        ]
      }
    ]
  }
}
```

---

## 📝 数据字典

### 角色类型 (RoleType)
| 值 | 名称 | 说明 |
|----|------|------|
| 0 | system | 系统角色 |
| 1 | custom | 自定义角色 |

### 角色状态 (RoleStatus)
| 值 | 名称 | 说明 |
|----|------|------|
| 0 | disabled | 禁用 |
| 1 | enabled | 启用 |

### 权限类型 (PermissionType)
| 值 | 名称 | 说明 |
|----|------|------|
| 0 | menu | 菜单权限 |
| 1 | button | 按钮权限 |
| 2 | api | API权限 |
| 3 | data | 数据权限 |

### 权限状态 (PermissionStatus)
| 值 | 名称 | 说明 |
|----|------|------|
| 0 | disabled | 禁用 |
| 1 | enabled | 启用 |

### 数据权限范围 (DataScope)
| 值 | 名称 | 说明 |
|----|------|------|
| 1 | all | 全部数据 |
| 2 | department | 本部门数据 |
| 3 | department_sub | 本部门及下级数据 |
| 4 | self | 仅本人数据 |

### 审批状态 (ApprovalStatus)
| 值 | 名称 | 说明 |
|----|------|------|
| 0 | pending | 待审批 |
| 1 | approved | 已批准 |
| 2 | rejected | 已拒绝 |

---

## 🔧 错误码说明

### 通用错误码
| 错误码 | 说明 |
|--------|------|
| 200 | 操作成功 |
| 400 | 请求参数错误 |
| 401 | 未授权访问 |
| 403 | 权限不足 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

### 业务错误码
| 错误码 | 说明 |
|--------|------|
| 1001 | 用户不存在 |
| 1002 | 用户已存在 |
| 1003 | 邮箱已存在 |
| 1004 | 手机号已存在 |
| 2001 | 角色不存在 |
| 2002 | 角色名称已存在 |
| 2003 | 角色代码已存在 |
| 2004 | 系统角色不允许操作 |
| 3001 | 权限不存在 |
| 3002 | 权限名称已存在 |
| 3003 | 权限代码已存在 |
| 4001 | 批量操作失败 |

---

## 📋 接口调用示例

### JavaScript/TypeScript 示例

```javascript
// 创建角色
const createRole = async (roleData) => {
  const response = await fetch('/api/admin/roles', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(roleData)
  });
  return response.json();
};

// 获取角色列表
const getRoles = async (params) => {
  const queryString = new URLSearchParams(params).toString();
  const response = await fetch(`/api/admin/roles?${queryString}`, {
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });
  return response.json();
};

// 分配角色权限
const assignPermissions = async (roleId, permissionIds) => {
  const response = await fetch(`/api/admin/roles/${roleId}/permissions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify({ permissionIds })
  });
  return response.json();
};
```

### Python 示例

```python
import requests

class RBACClient:
    def __init__(self, base_url, token):
        self.base_url = base_url
        self.headers = {
            'Content-Type': 'application/json',
            'Authorization': f'Bearer {token}'
        }
    
    def create_role(self, role_data):
        response = requests.post(
            f'{self.base_url}/roles',
            json=role_data,
            headers=self.headers
        )
        return response.json()
    
    def get_roles(self, params=None):
        response = requests.get(
            f'{self.base_url}/roles',
            params=params,
            headers=self.headers
        )
        return response.json()
    
    def assign_user_role(self, user_id, role_id, role_data):
        response = requests.post(
            f'{self.base_url}/users/{user_id}/roles/{role_id}',
            json=role_data,
            headers=self.headers
        )
        return response.json()

# 使用示例
client = RBACClient('http://localhost:8080/api/admin', 'your-token')
result = client.create_role({
    'name': '测试角色',
    'code': 'test_role',
    'description': '用于测试的角色'
})
```

---

**文档版本**: v1.0  
**更新时间**: 2025-10-10  
**联系方式**: developer@example.com
