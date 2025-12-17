# API 接口对接快速参考

## 修复摘要（2025-11-21）

### 🎯 修复的问题数量：5 个

---

## ✅ 已修复的问题

| # | 问题描述 | 修复方式 | 优先级 |
|---|---------|---------|--------|
| 1 | 普通用户刷新令牌接口缺失 | 后端新增 `POST /api/auth/refresh` | P1 严重 |
| 2 | 普通用户退出登录接口缺失 | 后端新增 `POST /api/auth/logout` | P1 严重 |
| 3 | 权限码获取接口缺失 | 后端新增 `GET /api/auth/codes` | P3 中等 |
| 4 | OIDC 用户注册接口缺失 | 后端新增 `POST /api/oidc/register` | P2 高 |
| 5 | 社交账号接口路径不匹配 | 前端修复路径 `/api/user/social-accounts` | P2 高 |

---

## 📝 新增接口一览

### 用户认证接口组

```
POST   /api/auth/logout    # 用户退出登录（需认证）
POST   /api/auth/refresh   # 刷新访问令牌（需认证）
GET    /api/auth/codes     # 获取用户权限码（需认证）
```

### OIDC 授权接口

```
POST   /api/oidc/register  # OIDC 用户注册
```

---

## 🔧 修改的文件

### 后端（3个文件）
1. `app/controller/user_controller.go` - 新增 3 个方法
2. `app/controller/oidc_auth_flow_controller.go` - 新增 1 个方法
3. `config/autoload/router.go` - 新增 4 个路由

### 前端（1个文件）
1. `admin-web/apps/web-oidc/src/api/social-login.ts` - 修复路径

---

## 🧪 快速测试命令

### 测试用户认证接口
```bash
# 退出登录
curl -X POST http://localhost:8080/api/auth/logout \
  -H "Authorization: Bearer YOUR_TOKEN"

# 刷新令牌
curl -X POST http://localhost:8080/api/auth/refresh \
  -H "Authorization: Bearer YOUR_TOKEN"

# 获取权限码
curl -X GET http://localhost:8080/api/auth/codes \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### 测试 OIDC 注册
```bash
curl -X POST http://localhost:8080/api/oidc/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "password123",
    "realName": "Test User"
  }'
```

---

## 📊 对接状态总览

| 模块 | 接口数 | 状态 |
|-----|--------|------|
| 安装相关 | 4 | ✅ 完全对齐 |
| 管理员认证 | 4 | ✅ 完全对齐 |
| 用户认证 | 4 | ✅ 完全对齐（本次修复）|
| 用户管理 | 17 | ✅ 完全对齐 |
| 角色管理 | 10 | ✅ 完全对齐 |
| 权限管理 | 9 | ✅ 完全对齐 |
| 部门管理 | 14 | ✅ 完全对齐 |
| 应用管理 | 9 | ✅ 完全对齐 |
| 社交登录提供商 | 7 | ✅ 完全对齐 |
| 社交认证 | 7 | ✅ 完全对齐（本次修复）|
| OIDC 认证 | 3 | ✅ 完全对齐（本次修复）|
| **总计** | **88** | **✅ 100% 对齐** |

---

## ⚠️ 重要提示

1. **认证要求**：所有 `/api/auth/*` 接口需要有效的 JWT 令牌
2. **中间件**：使用 `middleware.AuthMiddleware` 进行认证
3. **错误处理**：所有接口都返回标准错误码
4. **会话管理**：OIDC 注册支持会话自动认证

---

## 📚 详细文档

完整的修复详情请参阅：`_docs/API_INTEGRATION_FIXES.md`

原始检查报告请参阅：`api-integration-check.plan.md`

