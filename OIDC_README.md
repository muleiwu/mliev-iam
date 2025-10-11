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
- ✅ RS256 ID Token 签名
- ✅ JWKS 端点 (JSON Web Key Set)
- ✅ 会话管理 (Session Management)

### 管理功能
- ✅ OIDC客户端管理 (创建、更新、删除、查询)
- ✅ 客户端状态管理 (启用/禁用)
- ✅ 批量操作支持
- ✅ 统计信息查看
- ✅ 过期数据清理

### 授权流程
- ✅ 完整的用户登录页面
- ✅ 授权确认页面
- ✅ 可嵌入 Iframe 的组件支持
- ✅ 基于 Redis 的会话管理

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

### 基本配置（使用RS256签名）

```yaml
oidc:
  # 颁发者标识符（通常是服务的基础URL）
  issuer: "https://your-domain.com"
  
  # 签名密钥配置
  signing:
    # 推荐使用 RS256 算法
    algorithm: "RS256"
    key_id: "default"
    # RSA私钥和公钥路径
    rsa_private_key_path: "./keys/oidc_private_key.pem"
    rsa_public_key_path: "./keys/oidc_public_key.pem"
    # 或使用环境变量
    # rsa_private_key_env: "OIDC_RSA_PRIVATE_KEY"
  
  # 默认令牌生命周期（秒）
  token_lifetime:
    access_token: 3600     # 1小时
    refresh_token: 2592000 # 30天
    id_token: 3600         # 1小时
    authorization_code: 600 # 10分钟
  
  # 会话配置
  session:
    redis_key_prefix: "oidc:session:"
    expiration: 600  # 10分钟
  
  # 支持的功能配置
  features:
    pkce_enabled: true
    introspection_enabled: true
    revocation_enabled: true
    refresh_token_enabled: true
```

### 生成RSA密钥对

使用提供的脚本生成密钥对：

```bash
# 生成2048位密钥（推荐用于生产）
./scripts/generate-rsa-keys.sh --size 2048 --output ./keys

# 或生成4096位密钥（更高安全性）
./scripts/generate-rsa-keys.sh --size 4096 --output ./keys
```

**重要提示**：
- 私钥必须妥善保管，切勿泄露
- 建议在生产环境使用环境变量或密钥管理服务存储私钥
- 定期轮换密钥以提高安全性

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

### 4. 客户端凭据流程 (Client Credentials Flow)

用于服务间认证，不涉及用户：

```bash
curl -X POST http://localhost:8080/api/oidc/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=客户端密钥&scope=api"
```

响应示例：
```json
{
  "access_token": "访问令牌",
  "token_type": "Bearer",
  "expires_in": 3600,
  "scope": "api"
}
```

**注意**：客户端凭据流程不返回 `refresh_token` 和 `id_token`。

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

## 前端授权流程组件

本项目提供了完整的前端授权流程组件，支持独立页面和 Iframe 嵌入模式。

### 登录组件 (OIDCLoginForm)

独立使用：
```vue
<template>
  <OIDCLoginForm
    :session-id="sessionId"
    title="用户登录"
    subtitle="使用您的账户登录以继续"
    @success="handleSuccess"
    @error="handleError"
  />
</template>
```

Iframe 嵌入：
```vue
<template>
  <OIDCLoginForm
    :session-id="sessionId"
    :embed-mode="true"
    @success="handleSuccess"
  />
</template>
```

在 Iframe 模式下，组件会通过 `postMessage` 与父窗口通信。

### 授权确认组件 (OIDCConsentForm)

```vue
<template>
  <OIDCConsentForm
    :session-id="sessionId"
    :show-remember-choice="false"
    @approved="handleApproved"
    @rejected="handleRejected"
  />
</template>
```

### 页面路由

- 登录页面：`/oidc/login?session_id={sessionId}`
- 授权确认页面：`/oidc/consent?session_id={sessionId}`

这些页面不需要认证，可直接访问。

### Iframe 集成示例

在第三方应用中嵌入：

```html
<iframe
  src="https://your-iam-domain.com/oidc/login?session_id=xxx"
  width="400"
  height="500"
  frameborder="0"
></iframe>

<script>
window.addEventListener('message', (event) => {
  if (event.data.type === 'oidc-login-success') {
    console.log('Login successful:', event.data);
    // 处理登录成功
  }
});
</script>
```

## 扩展点

- 可以扩展自定义作用域和声明
- 支持自定义JWT签名算法 (RS256, HS256)
- 可以集成外部身份验证系统
- 支持多租户场景
- 前端组件可定制主题和样式

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
