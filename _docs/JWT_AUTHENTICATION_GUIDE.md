# JWT认证实现文档

## 概述

本系统已成功实现了完整的JWT（JSON Web Token）认证功能，包括：

- JWT token生成和验证
- 认证中间件
- 管理员登录和权限控制
- Token刷新机制
- 配置化管理

## 功能特性

### 1. JWT Token生成
- 支持用户登录后生成JWT token
- Token包含用户ID、用户名、角色等信息
- 可配置的过期时间

### 2. 认证中间件
- `AuthMiddleware`: 基础JWT认证中间件
- `AdminAuthMiddleware`: 管理员权限认证中间件
- `OptionalAuthMiddleware`: 可选认证中间件

### 3. 配置管理
- 支持环境变量配置JWT密钥
- 可配置token有效期
- 支持自定义发行者

## 环境变量配置

创建 `.env` 文件并配置以下变量：

```bash
# JWT Configuration
JWT_SECRET_KEY=your-super-secret-jwt-key-change-this-in-production-please-use-a-strong-random-key-at-least-32-characters
JWT_ISSUER=mliev-iam
JWT_ACCESS_TOKEN_TTL=1    # 访问令牌有效期（小时）
JWT_REFRESH_TOKEN_TTL=30  # 刷新令牌有效期（天）
```

## API接口

### 管理员登录
```
POST /api/admin/auth/login
Content-Type: application/json

{
    "username": "admin",
    "password": "password"
}
```

响应：
```json
{
    "code": 0,
    "message": "管理员登录成功",
    "data": {
        "user": {
            "id": 1,
            "username": "admin",
            "email": "admin@example.com"
        },
        "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
        "tokenType": "Bearer",
        "expiresIn": 3600
    }
}
```

### 获取管理员信息（需要认证）
```
GET /api/admin/auth/profile
Authorization: Bearer <token>
```

### 刷新Token
```
POST /api/admin/auth/refresh
Authorization: Bearer <old_token>
```

或者：
```json
{
    "refreshToken": "<old_token>"
}
```

## 中间件使用示例

### 1. 基础认证
```go
// 需要用户登录的路由
userGroup := router.Group("/api/user")
userGroup.Use(middleware.AuthMiddleware(helper))
{
    userGroup.GET("/profile", getUserProfile)
}
```

### 2. 管理员认证
```go
// 需要管理员权限的路由
adminGroup := router.Group("/api/admin")
adminGroup.Use(middleware.AdminAuthMiddleware(helper))
{
    adminGroup.GET("/users", getUserList)
}
```

### 3. 获取当前用户信息
```go
func someHandler(c *gin.Context) {
    userID, username, exists := middleware.GetCurrentUser(c)
    if !exists {
        c.JSON(401, gin.H{"error": "未登录"})
        return
    }
    
    // 使用用户信息
    fmt.Printf("当前用户: %s (ID: %d)\n", username, userID)
}
```

## 文件结构

```
├── app/
│   ├── controller/admin/
│   │   └── auth_controller.go          # 认证控制器
│   ├── middleware/
│   │   └── auth_middleware.go          # 认证中间件
│   ├── service/
│   │   └── jwt_auth_service.go         # JWT认证服务
│   └── routes/
│       └── admin_routes.go             # 路由配置示例
├── internal/pkg/
│   ├── jwt/
│   │   └── jwt.go                      # JWT核心实现
│   └── config/
│       └── jwt_config.go               # JWT配置管理
```

## 安全注意事项

1. **JWT密钥安全**: 生产环境必须使用强随机密钥，建议至少32个字符
2. **Token存储**: 客户端应安全存储JWT token，避免XSS攻击
3. **HTTPS**: 生产环境必须使用HTTPS传输token
4. **Token过期**: 合理设置token过期时间，平衡安全性和用户体验
5. **刷新机制**: 实现token刷新机制，避免用户频繁登录

## 使用步骤

1. **配置环境变量**: 设置JWT相关配置
2. **用户登录**: 调用登录接口获取token
3. **请求认证**: 在请求头中携带`Authorization: Bearer <token>`
4. **中间件验证**: 系统自动验证token并提取用户信息
5. **访问资源**: 通过验证后可访问受保护的资源

## 扩展功能

系统支持以下扩展：

- **黑名单机制**: 可实现token黑名单功能
- **角色权限**: 支持基于角色的访问控制（RBAC）
- **单点登录**: 可扩展为SSO系统
- **OAuth2**: 可集成OAuth2协议

## 错误处理

系统提供完整的错误处理：

- `401 Unauthorized`: token无效或过期
- `403 Forbidden`: 权限不足
- `400 Bad Request`: 请求参数错误

## 测试

可以使用以下工具测试JWT功能：

1. **Postman**: 导入API集合进行测试
2. **curl**: 命令行测试
3. **单元测试**: 为JWT功能编写测试用例

```bash
# 登录获取token
curl -X POST http://localhost:8080/api/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"password"}'

# 使用token访问受保护资源
curl -X GET http://localhost:8080/api/admin/auth/profile \
  -H "Authorization: Bearer <your-token>"
```
