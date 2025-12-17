# OIDC 密钥管理数据库化重构文档

## 概述

本次重构将 OIDC 应用的签名密钥管理从配置文件迁移到数据库，实现了每个应用独立密钥、支持多种密钥类型，并为密钥轮转功能预留了扩展空间。

## 重构时间

2025年11月

## 主要变更

### 1. 数据库架构

#### 新增表：`application_oidc_keys`

存储每个 OIDC 应用的签名密钥信息。

**字段说明：**

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `id` | uint | 主键 |
| `application_id` | uint | 关联的应用ID（外键） |
| `key_id` | string | 密钥ID（kid），用于JWT header |
| `algorithm` | string | 签名算法（RS256, RS512, HS256等） |
| `key_type` | string | 密钥类型（RSA, HMAC, ECDSA） |
| `private_key` | text | 私钥数据（PEM或base64格式） |
| `public_key` | text | 公钥数据（PEM格式，非对称加密使用） |
| `key_size` | int | 密钥长度（2048, 4096, 256等） |
| `is_primary` | bool | 是否为主密钥 |
| `expires_at` | timestamp | 密钥过期时间（可选，为轮转预留） |
| `created_at` | timestamp | 创建时间 |
| `updated_at` | timestamp | 更新时间 |

**索引：**
- `application_id` + `is_primary` 组合索引
- `key_id` 唯一索引

### 2. 密钥管理架构

#### 接口抽象

新增 `KeyManager` 接口，支持多种密钥类型的统一管理：

```go
type KeyManager interface {
    GenerateKey() error
    GetKeyID() string
    GetAlgorithm() string
    GetKeyType() KeyType
    GetKeySize() int
    ExportPrivateKey() (string, error)
    ExportPublicKey() (string, error)
    LoadPrivateKey(data string) error
    SignToken(claims jwt.Claims) (string, error)
    VerifyToken(tokenString string) (*jwt.Token, error)
    GetJWK() (map[string]interface{}, error)
    IsInitialized() bool
}
```

#### 实现类

1. **RSAKeyManager**
   - 支持算法：RS256, RS384, RS512
   - 默认密钥长度：2048位（RS256/RS384），4096位（RS512）
   - 密钥格式：PEM（PKCS1或PKCS8）

2. **HMACKeyManager**
   - 支持算法：HS256, HS384, HS512
   - 默认密钥长度：256位（HS256），384位（HS384），512位（HS512）
   - 密钥格式：Base64编码

3. **KeyManagerFactory**
   - 根据算法类型自动创建对应的密钥管理器
   - 支持从元数据加载已有密钥

### 3. 服务层

#### ApplicationKeyService

新增密钥服务层，提供密钥的完整生命周期管理：

**主要方法：**

- `GenerateKeyForApplication(appID, algorithm, keySize)` - 为应用生成密钥
- `GetPrimaryKeyManagerForApplication(appID)` - 获取应用的主密钥管理器
- `GetKeyManagerByKeyID(keyID)` - 根据密钥ID获取密钥管理器
- `LoadKeyManager(key)` - 从数据库记录加载密钥管理器
- `SetPrimaryKey(appID, keyID)` - 设置主密钥（为轮转预留）
- `DeleteKeysByApplication(appID)` - 删除应用的所有密钥

### 4. 协议处理器更新

#### OIDCProtocolHandler

**CreateApplication 变更：**
- 在创建 OIDC 应用时自动生成签名密钥
- 默认使用 RS256 算法，2048位密钥
- 支持从配置中指定签名算法（可选）

**DeleteApplication 变更：**
- 删除应用时自动清理相关密钥

### 5. 端点更新

#### JWKS 端点 (`/oidc/jwks`)

**变更前：**
- 从配置文件读取全局密钥
- 所有应用共享同一个密钥

**变更后：**
- 支持通过 `client_id` 查询参数指定应用
- 从数据库读取应用特定的密钥
- 对称加密算法（HMAC）返回空密钥集

**使用示例：**
```
GET /oidc/jwks?client_id=<application-uuid>
```

#### Token 生成

**ID Token 生成变更：**
- 从数据库获取应用的主密钥管理器
- 根据密钥类型动态创建 Token 服务
- 支持 RSA 和 HMAC 两种密钥类型

### 6. 配置文件变更

**移除的配置项：**
```yaml
oidc:
  signing:
    algorithm: "RS256"
    key_id: "default"
    rsa_private_key_path: "./keys/oidc_private_key.pem"
    rsa_public_key_path: "./keys/oidc_public_key.pem"
    rsa_private_key_env: "OIDC_RSA_PRIVATE_KEY"
    hmac_secret: "your-secret"
```

**替换说明：**
```yaml
oidc:
  # 注意：签名密钥配置已移除
  # 每个 OIDC 应用在创建时会自动生成独立的签名密钥（默认 RS256, 2048位）
  # 密钥存储在数据库中，支持密钥轮转和多种算法（RS256, RS512, HS256等）
  # 无需手动配置密钥文件
```

## 迁移指南

### 现有应用迁移

如果系统中已有 OIDC 应用，需要为其生成密钥：

**方式一：删除并重新创建应用**
- 优点：自动生成密钥
- 缺点：需要更新客户端配置（client_secret 会改变）

**方式二：创建迁移脚本**
```go
// 为所有现有 OIDC 应用生成密钥
keyService := service.NewApplicationKeyService(db)
apps := // ... 查询所有 OIDC 应用
for _, app := range apps {
    _, err := keyService.GenerateKeyForApplication(app.ID, "RS256", 2048)
    if err != nil {
        log.Printf("为应用 %d 生成密钥失败: %v", app.ID, err)
    }
}
```

### 数据库迁移

系统启动时会自动创建 `application_oidc_keys` 表。

## 新增文件

```
app/
  model/
    application_oidc_key.go          # 密钥模型
  dao/
    application_oidc_key_dao.go      # 密钥DAO
  service/
    application_key_service.go       # 密钥服务

internal/pkg/jwt/
  key_manager.go                     # 密钥管理器接口
  hmac_key_manager.go                # HMAC实现
  key_manager_factory.go             # 工厂类
```

## 修改文件

```
app/
  service/
    protocol/
      oidc_handler.go                # 添加密钥生成逻辑
  controller/
    oidc_controller.go               # 更新JWKS端点
  service/
    oidc_authorization_service.go    # 更新Token生成
    
internal/pkg/jwt/
  rsa_key_manager.go                 # 实现KeyManager接口

config/
  autoload/
    migration.go                     # 添加密钥表迁移

config.yaml.example                  # 移除密钥配置
```

## 架构优势

### 1. 安全性提升
- **密钥隔离**：每个应用使用独立密钥，一个应用的密钥泄露不影响其他应用
- **灵活的算法选择**：不同应用可选择不同的签名算法
- **便于密钥管理**：集中式密钥存储，便于审计和管理

### 2. 可扩展性
- **接口抽象**：通过 KeyManager 接口，轻松支持新的密钥类型（如 ECDSA）
- **密钥轮转支持**：数据库设计支持多密钥共存和过期时间
- **多租户友好**：天然支持多租户场景，每个租户独立密钥

### 3. 运维友好
- **自动化**：应用创建时自动生成密钥，无需手动配置
- **无文件依赖**：不再依赖文件系统存储密钥，便于容器化部署
- **易于备份**：密钥随数据库备份，恢复更简单

## 后续扩展计划

### 1. 密钥加密存储
当前私钥以明文形式存储在数据库中，后续可实现：
- 应用级加密密钥（从环境变量或密钥管理服务获取）
- 使用 AES-256-GCM 加密私钥
- 支持云服务商的密钥管理服务（如 AWS KMS）

### 2. 密钥轮转
- 支持多个有效密钥共存
- 自动密钥过期和清理
- 平滑的密钥切换（新 Token 用新密钥签名，旧 Token 用旧密钥验证）

### 3. 更多密钥类型
- ECDSA (ES256, ES384, ES512)
- EdDSA (EdDSA)
- PS256/PS384/PS512（RSA-PSS）

### 4. 密钥管理 API
- 查看应用的所有密钥
- 手动生成新密钥
- 设置主密钥
- 删除过期密钥

## 注意事项

### 1. 性能考虑
- 密钥查询可能较频繁，考虑添加缓存层（Redis或内存缓存）
- 建议监控密钥查询的性能指标

### 2. 安全建议
- 尽快实现私钥加密存储
- 定期审计密钥使用情况
- 设置密钥过期策略

### 3. 向后兼容
- JWKS 端点现在需要 `client_id` 参数
- 客户端需要更新 JWKS URI（如果有直接使用）
- 建议客户端从 Discovery 端点获取 JWKS URI

## 测试建议

### 单元测试
- [ ] KeyManager 接口各实现的测试
- [ ] ApplicationKeyService 各方法测试
- [ ] 密钥生成和加载测试

### 集成测试
- [ ] 创建 OIDC 应用并验证密钥自动生成
- [ ] JWKS 端点测试
- [ ] Token 签名和验证测试
- [ ] 应用删除时密钥清理测试

### 性能测试
- [ ] 密钥查询性能
- [ ] Token 生成性能对比（前后）
- [ ] 高并发场景测试

## 相关文档

- [OIDC 实现总结](./OIDC_IMPLEMENTATION_SUMMARY.md)
- [JWT 认证指南](./JWT_AUTHENTICATION_GUIDE.md)
- [项目规范](./PROJECT_SPECIFICATION.md)

## 技术支持

如有问题，请参考：
1. 代码中的注释和文档
2. 相关的测试用例
3. 本文档的示例代码


