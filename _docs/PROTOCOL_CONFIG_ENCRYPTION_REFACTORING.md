# 协议配置加密重构文档

## 概述

本次重构将协议配置中敏感字段的哈希处理逻辑从 `PrepareConfig` 方法中分离出来，添加了专门的 `EncryptSensitiveFields` 方法，确保密钥等敏感信息仅在创建时显示一次。

## 重构目标

1. **职责分离**：将配置生成和敏感字段加密分离为两个独立的方法
2. **安全性增强**：确保敏感信息（如 ClientSecret）仅在创建时返回给前端一次
3. **存储安全**：数据库中仅存储哈希后的敏感信息
4. **向后兼容**：保持现有的哈希算法和验证逻辑不变

## 主要变更

### 1. 协议处理器接口 (`app/service/protocol/handler.go`)

添加了新方法：

```go
// EncryptSensitiveFields 加密/哈希配置中的敏感字段（用于存储）
EncryptSensitiveFields(config map[string]interface{}) error
```

### 2. OIDC 协议处理器 (`app/service/protocol/oidc_handler.go`)

#### 2.1 重构 `PrepareConfig` 方法

**变更前**：
- 生成 ClientID 和 ClientSecret
- 立即对 ClientSecret 进行哈希
- 返回包含哈希密钥的配置和临时的 `clientSecretPlain` 字段

**变更后**：
- 生成 ClientID 和 ClientSecret（明文）
- 直接返回包含明文 ClientSecret 的配置
- 移除了 `clientSecretPlain` 临时字段

#### 2.2 新增 `EncryptSensitiveFields` 方法

- 对配置中的 `clientSecret` 字段进行 SHA256 哈希
- 直接修改传入的配置对象（in-place 修改）
- 保留其他字段不变

### 3. 应用服务 (`app/service/application_service.go`)

#### 3.1 `CreateApplication` 方法流程

**变更前的流程**：
```
PrepareConfig (生成并哈希) → 创建 Application → 保存数据库 → 返回响应
```

**变更后的流程**：
```
PrepareConfig (明文) 
  ↓
创建明文配置副本（用于响应）
  ↓
EncryptSensitiveFields (哈希原配置)
  ↓
创建 Application（使用哈希配置）
  ↓
保存到数据库
  ↓
返回明文配置响应
```

**关键代码**：
```go
// 准备配置（明文）
preparedConfig, err := handler.PrepareConfig(req.ProtocolConfig)

// 创建明文副本用于响应
plainConfig := make(map[string]interface{})
for k, v := range preparedConfig {
    plainConfig[k] = v
}

// 加密敏感字段
err = handler.EncryptSensitiveFields(preparedConfig)

// 使用加密后的配置创建应用
app := &model.Application{
    ProtocolConfig: preparedConfig,
    // ...
}

// 保存到数据库
err = s.appDAO.Create(app)

// 返回时使用明文配置
return s.modelToResponse(app, plainConfig), nil
```

#### 3.2 简化 `modelToResponse` 方法

**变更前**：
- 复杂的逻辑处理 `clientSecretPlain` 临时字段
- 需要判断协议类型并替换密钥

**变更后**：
- 如果提供了明文配置（创建操作），直接使用
- 如果未提供（查询操作），过滤掉 `clientSecret` 字段
- 逻辑更简洁清晰

## 配置处理流程详解

### 创建应用时

1. **PrepareConfig** 返回：
   ```json
   {
     "clientId": "uuid-generated",
     "clientSecret": "plain_text_secret_base64",
     "redirectUris": [...],
     ...
   }
   ```

2. **保存明文副本**：用于稍后返回给前端

3. **EncryptSensitiveFields** 修改配置为：
   ```json
   {
     "clientId": "uuid-generated",
     "clientSecret": "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
     "redirectUris": [...],
     ...
   }
   ```

4. **存储到数据库**：保存哈希版本

5. **返回给前端**：使用明文副本，前端可以看到明文的 `clientSecret`

### 查询应用时

1. **从数据库读取**：获取包含哈希密钥的配置

2. **modelToResponse** 处理：过滤掉 `clientSecret` 字段

3. **返回给前端**：不包含敏感信息
   ```json
   {
     "clientId": "uuid-generated",
     "redirectUris": [...],
     ...
     // clientSecret 已被过滤
   }
   ```

## 安全特性

1. **单次显示**：ClientSecret 仅在创建应用时返回一次
2. **bcrypt 哈希存储**：数据库中存储的是 bcrypt 哈希值（带随机盐），不可逆
3. **验证机制**：使用 `verifyClientSecret` 方法验证客户端凭据
4. **访问控制**：后续查询不返回敏感信息
5. **抗暴力破解**：bcrypt 的计算成本使暴力破解变得不可行

## 测试

创建了完整的测试套件 (`oidc_handler_test.go`)：

- `TestOIDCProtocolHandler_PrepareConfig`：验证 PrepareConfig 返回明文
- `TestOIDCProtocolHandler_EncryptSensitiveFields`：验证加密功能
- `TestOIDCProtocolHandler_EncryptSensitiveFields_Idempotent`：验证重复加密行为
- `TestOIDCProtocolHandler_PrepareAndEncrypt_Integration`：集成测试

所有测试通过 ✓

## 兼容性

- ✅ 使用 bcrypt 哈希算法（已升级）
- ✅ 数据库结构无需修改
- ✅ API 响应格式兼容（创建时返回明文，查询时不返回）
- ⚠️ 不向后兼容旧的 SHA256 哈希（但系统无生产数据）

## 扩展性

### 添加新的协议处理器

当实现新的协议处理器（如 SAML、CAS、OAuth2）时，需要：

1. 实现 `ProtocolHandler` 接口
2. 实现 `PrepareConfig` 方法（生成明文凭据）
3. 实现 `EncryptSensitiveFields` 方法（加密敏感字段）
4. 在工厂中注册新的协议类型

示例：

```go
type SAMLProtocolHandler struct {
    db *gorm.DB
}

func (h *SAMLProtocolHandler) PrepareConfig(config map[string]interface{}) (map[string]interface{}, error) {
    // 生成 SAML 特定的明文凭据
    // ...
    return config, nil
}

func (h *SAMLProtocolHandler) EncryptSensitiveFields(config map[string]interface{}) error {
    // 加密 SAML 特定的敏感字段
    // 例如：privateKey, certificate 等
    // ...
    return nil
}
```

## 注意事项

1. **配置副本**：创建应用时必须先保存明文配置副本，再调用 `EncryptSensitiveFields`
2. **顺序重要**：必须在加密后创建 Application 对象，确保数据库存储的是加密版本
3. **内存安全**：明文配置仅在内存中短暂存在，用于响应后即被丢弃
4. **不要二次加密**：`EncryptSensitiveFields` 不是幂等的，避免对已加密的数据再次加密

## 安全升级历史

### 2025-10-29: 升级为 bcrypt 密钥哈希

**背景**：原实现使用简单的 SHA256 哈希（无盐），存在安全隐患：
- 无盐哈希易受彩虹表攻击
- SHA256 速度快，容易被暴力破解

**改进**：升级为 bcrypt 哈希算法：
- ✅ 每个密码使用唯一随机盐（盐自动嵌入哈希结果）
- ✅ 计算成本可调（默认 cost=10），抗暴力破解
- ✅ 业界标准，经过时间验证
- ✅ 使用 `golang.org/x/crypto/bcrypt` 库

**影响范围**：
- 修改文件：`app/service/protocol/oidc_handler.go`、`app/service/protocol/oidc_handler_test.go`
- 哈希格式变化：从 64 字符的十六进制 → 60 字符左右的 bcrypt 格式（`$2a$10$...`）
- 向后兼容性：不兼容已有 SHA256 哈希（但系统无生产数据）

## 未来改进

1. 考虑使用对称加密（如 AES）替代单向哈希，以支持某些特殊场景
2. 添加密钥轮换机制
3. 添加审计日志，记录敏感信息的访问

## 相关文件

- `app/service/protocol/handler.go` - 协议处理器接口
- `app/service/protocol/oidc_handler.go` - OIDC 协议处理器实现
- `app/service/application_service.go` - 应用服务
- `app/service/protocol/oidc_handler_test.go` - 测试文件

