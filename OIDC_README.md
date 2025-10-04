# OIDC (OpenID Connect) 服务能力

本项目已集成了完整的OIDC (OpenID Connect) 服务能力，支持标准的OAuth 2.0 + OIDC协议，可以作为身份提供商(Identity Provider, IdP)为其他应用提供单点登录(SSO)服务。

## 功能特性

### 核心OIDC功能
- ✅ 授权码流程 (Authorization Code Flow)
- ✅ 隐式流程 (Implicit Flow) 
- ✅ 客户端凭据流程 (Client Credentials Flow)
- ✅ 刷新令牌流程 (Refresh Token Flow)
- ✅ PKCE 支持 (Proof Key for Code Exchange)
- ✅ 用户信息端点 (UserInfo Endpoint)
- ✅ 令牌内省 (Token Introspection)
- ✅ 令牌撤销 (Token Revocation)
- ✅ OIDC 发现文档 (Discovery Document)

### 管理功能
- ✅ OIDC客户端管理 (创建、更新、删除、查询)
- ✅ 客户端状态管理 (启用/禁用)
- ✅ 批量操作支持
- ✅ 统计信息查看
- ✅ 过期数据清理

## API 端点

### OIDC 标准端点

| 端点 | 方法 | 描述 |
|------|------|------|
| `/.well-known/openid-configuration` | GET | OIDC发现文档 |
| `/api/oidc/authorize` | GET | 授权端点 |
| `/api/oidc/token` | POST | 令牌端点 |
| `/api/oidc/userinfo` | GET | 用户信息端点 |
| `/api/oidc/introspect` | POST | 令牌内省端点 |
| `/api/oidc/revoke` | POST | 令牌撤销端点 |
| `/api/oidc/jwks` | GET | JSON Web Key Set端点 |

### 管理端点 (需要管理员权限)

| 端点 | 方法 | 描述 |
|------|------|------|
| `/api/admin/oidc/clients` | POST | 创建OIDC客户端 |
| `/api/admin/oidc/clients` | GET | 获取客户端列表 |
| `/api/admin/oidc/clients/:id` | GET | 获取客户端详情 |
| `/api/admin/oidc/clients/:id` | PUT | 更新客户端 |
| `/api/admin/oidc/clients/:id` | DELETE | 删除客户端 |
| `/api/admin/oidc/clients/batch` | POST | 批量操作客户端 |
| `/api/admin/oidc/stats` | GET | 获取统计信息 |
| `/api/admin/oidc/cleanup` | POST | 清理过期数据 |

## 配置说明

OIDC相关配置在 `config.yaml` 文件中的 `oidc` 部分：

```yaml
oidc:
  # 颁发者标识符（通常是服务的基础URL）
  issuer: "http://localhost:8080"
  
  # 签名密钥配置
  signing:
    algorithm: "HS256"
    hmac_secret: "your-oidc-hmac-secret-key"
  
  # 默认令牌生命周期（秒）
  token_lifetime:
    access_token: 3600     # 1小时
    refresh_token: 2592000 # 30天
    id_token: 3600         # 1小时
    authorization_code: 600 # 10分钟
  
  # 支持的功能配置
  features:
    pkce_enabled: true
    introspection_enabled: true
    revocation_enabled: true
    refresh_token_enabled: true
```

## 使用示例

### 1. 创建OIDC客户端

```bash
curl -X POST http://localhost:8080/api/admin/oidc/clients \
  -H "Content-Type: application/json" \
  -d '{
    "name": "示例应用",
    "description": "这是一个示例OIDC客户端",
    "redirectUris": ["http://localhost:3000/callback"],
    "grantTypes": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"],
    "clientType": "confidential",
    "requirePkce": false
  }'
```

响应示例：
```json
{
  "code": 200,
  "data": {
    "id": 1,
    "clientId": "550e8400-e29b-41d4-a716-446655440000",
    "clientSecret": "超长的客户端密钥字符串",
    "name": "示例应用",
    "redirectUris": ["http://localhost:3000/callback"],
    "grantTypes": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"],
    "clientType": "confidential",
    "isActive": true
  }
}
```

### 2. 授权码流程

#### 步骤1: 重定向到授权端点
```
http://localhost:8080/api/oidc/authorize?
  response_type=code&
  client_id=550e8400-e29b-41d4-a716-446655440000&
  redirect_uri=http://localhost:3000/callback&
  scope=openid profile email&
  state=xyz&
  nonce=abc123
```

#### 步骤2: 用户登录并授权后，重定向回客户端
```
http://localhost:3000/callback?
  code=授权码&
  state=xyz
```

#### 步骤3: 交换授权码获取令牌
```bash
curl -X POST http://localhost:8080/api/oidc/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&code=授权码&redirect_uri=http://localhost:3000/callback&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=客户端密钥"
```

响应示例：
```json
{
  "access_token": "访问令牌",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "刷新令牌",
  "id_token": "ID令牌(JWT)",
  "scope": "openid profile email"
}
```

### 3. 获取用户信息

```bash
curl -X GET http://localhost:8080/api/oidc/userinfo \
  -H "Authorization: Bearer 访问令牌"
```

响应示例：
```json
{
  "sub": "123",
  "name": "张三",
  "preferred_username": "zhangsan",
  "email": "zhangsan@example.com",
  "email_verified": true
}
```

## 数据模型

### OIDC客户端 (oidc_clients)
- 客户端ID和密钥
- 重定向URI列表
- 支持的授权类型和响应类型
- 作用域配置
- 客户端类型（机密/公开）
- PKCE要求设置

### 授权码 (authorization_codes)
- 授权码值
- 关联的客户端和用户
- 作用域和参数
- PKCE验证信息
- 过期时间

### OIDC令牌 (oidc_tokens)
- 访问令牌和刷新令牌
- ID令牌
- 令牌类型和作用域
- 过期时间管理
- 撤销状态

## 安全考虑

1. **客户端密钥管理**: 机密客户端的密钥使用SHA-256哈希存储
2. **令牌安全**: 所有令牌都是随机生成的，具有足够的熵
3. **PKCE支持**: 支持PKCE以增强公开客户端的安全性
4. **作用域控制**: 严格的作用域验证确保最小权限原则
5. **过期管理**: 自动清理过期的授权码和令牌
6. **速率限制**: 支持配置速率限制防止滥用

## 扩展点

- 可以扩展自定义作用域和声明
- 支持自定义JWT签名算法
- 可以集成外部身份验证系统
- 支持多租户场景

## 故障排除

### 常见错误

1. **invalid_client**: 客户端ID或密钥错误
2. **invalid_grant**: 授权码无效或已过期
3. **invalid_scope**: 请求的作用域无效
4. **invalid_redirect_uri**: 重定向URI不匹配

### 日志查看

OIDC相关的日志会记录在应用日志中，可以通过配置的日志级别来调整详细程度。

## 兼容性

本OIDC实现遵循以下标准：
- OpenID Connect Core 1.0
- OAuth 2.0 (RFC 6749)
- OAuth 2.0 Token Introspection (RFC 7662)
- OAuth 2.0 Token Revocation (RFC 7009)
- PKCE for OAuth 2.0 (RFC 7636)
