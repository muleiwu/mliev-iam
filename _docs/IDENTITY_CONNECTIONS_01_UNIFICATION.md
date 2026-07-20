# 身份连接 01：统一连接与外部身份

> 状态：已实施并通过后端、管理端与 OIDC 前端验收（2026-07-20）
> 前置：无
> 后继：[02 飞书手动同步](./IDENTITY_CONNECTIONS_02_FEISHU_MANUAL_SYNC.md)

## 0. 同类产品调研与设计结论

调研日期：2026-07-20。这里要区分“用户如何登录”和“企业人员如何被创建、更新、停用”两个问题：两者共享组织和外部人员映射，但凭据、权限、失败模式和治理风险不同。

| 产品 | 公开设计 | 对本项目的启示 |
| --- | --- | --- |
| Auth0 | 使用统一的 Connection 概念，并在身份提供商层区分 Enterprise 与 Social；官方还明确把企业登录和访问外部 API 的 Connected Accounts 分开。 | 顶层连接可以统一，登录页仍应按企业/社交分组；认证不能承载目录或委托访问职责。 |
| WorkOS | SSO Connection 和 Directory Sync 是不同资源，但都绑定 Organization；Directory Sync 把不同目录归一为 directory/user/group，通过事件维持生命周期。 | 管理入口按企业组织聚合，内部保留认证、目录和事件三个能力协议；上层只消费标准模型。 |
| Microsoft Entra | 企业应用的登录配置与 Provisioning 页分开；自动供给包含创建、更新、停用，并提供 on-demand 运行用于验证映射和范围。 | 连接详情需要独立“目录同步”页、手动预览、范围/映射诊断和自动化历史，不能保存后直接大批量写入。 |

参考：[Auth0 Enterprise Identity Providers](https://auth0.com/docs/authenticate/identity-providers/enterprise-identity-providers)、[WorkOS Directory Sync](https://workos.com/docs/directory-sync)、[WorkOS Directory API](https://workos.com/docs/reference/directory-sync/directory)、[Microsoft Entra Provisioning](https://learn.microsoft.com/en-us/entra/identity/app-provisioning/user-provisioning)、[Microsoft Entra on-demand provisioning](https://learn.microsoft.com/en-us/entra/identity/app-provisioning/provision-on-demand)。

结论不是保留两个平级菜单，也不是把所有行为塞进一次 OAuth 登录：采用一个“身份连接”顶层资源和一个管理入口，以 `category` 负责展示分组、以 `capabilities` 负责能力组合；认证、目录、事件使用独立凭据和适配器，在连接详情中分开配置。这样既减少管理员认知重复，也不模糊安全边界。

## 1. 目标

将社会化登录与企业登录从两套重复模型收敛为统一的“身份连接”，同时保留面向管理员和最终用户的业务分类。首期完成数据库、服务、API、管理端、OIDC 登录页和个人中心的硬切，为目录同步提供稳定的连接与外部身份底座。

验收后的系统只有以下核心对象：

- `identity_connections`：上游身份连接。
- `identity_connection_credentials`：按用途加密保存的连接凭据。
- `external_principals`：外部自然人及其本地用户关联。
- `external_identifiers`：带命名空间的外部标识。

## 2. 非目标

- 不实现通讯录拉取、部门同步、定时任务或事件回调。
- 不实现 SCIM、LDAP、AD、出站供给或双向写回。
- 不长期保存登录流程获得的用户 access token/refresh token。
- 不保留旧管理、登录、回调、绑定或小程序 API 别名。

## 3. 领域模型

### 3.1 统一枚举

| 枚举 | 值 |
| --- | --- |
| `ConnectionCategory` | `social`、`enterprise` |
| `ConnectionStatus` | `disabled`、`enabled` |
| `ConnectionCapability` | `authentication`、`directory_sync`、`events` |
| `CredentialPurpose` | `authentication`、`directory`、`events` |
| `PrincipalStatus` | `active`、`blocked`、`unmatched` |
| `IdentifierNamespace` | `login_subject`、`directory_user_id`、`open_id`、`union_id` |

`provider_type` 表示实现类型，例如 `github`、`google`、`oidc`、`wechat`、`wechat_miniprogram`、`qq`、`feishu`、`wecom`、`dingtalk`。协议或厂商类型不得再直接决定 social/enterprise 分类。

### 3.2 认证适配器

认证适配器统一输出：

```go
type ExternalProfile struct {
    DisplayName string
    Avatar      string
    Email       string
    Phone       string
    Identifiers []ExternalIdentifier
    Raw         map[string]any
}

type AuthenticationAdapter interface {
    ValidateConfig() error
    AuthorizationURL(state string) string
    ExchangeCode(ctx context.Context, code string) (*AuthToken, error)
    FetchProfile(ctx context.Context, token *AuthToken) (*ExternalProfile, error)
}
```

目录和事件能力分别使用后续文档定义的 `DirectoryAdapter`、`EventAdapter`，禁止扩展 `AuthenticationAdapter` 承担同步职责。

## 4. 数据库设计

### 4.1 `identity_connections`

- 基础字段：`id`、`name`、`identifier`、`category`、`provider_type`、`description`。
- 能力字段：`capabilities` JSON 数组；迁移数据默认仅含 `authentication`。
- 非敏感配置：`config_json`，包含端点、回调地址、scope、agent ID、显示配置等。
- 状态字段：`status`、`is_default`、`sort_order`、`icon`。
- 健康字段：`last_checked_at`、`last_check_status`、`last_check_error`。
- 审计字段：`created_at`、`updated_at`、`deleted_at`、`remark`。
- `identifier` 在未软删记录中全局唯一。

### 4.2 `identity_connection_credentials`

- `connection_id + purpose` 唯一。
- `ciphertext`、`nonce` 均保存 Base64 文本。
- `algorithm` 固定 `AES-256-GCM`，`key_version` 首期固定 `1`。
- `rotated_at`、`created_at`、`updated_at`。
- API 只返回 `configured`、`keyVersion`、`rotatedAt`，永不返回密文或明文。

凭据 JSON 按提供商存储，认证用途至少保留旧 `client_id/client_secret`。加密使用环境变量 `IDENTITY_CREDENTIALS_KEY`，其值必须是 Base64 编码的 32 字节随机密钥；AAD 为 `connection_id:purpose:key_version`。

### 4.3 `external_principals`

- `connection_id`、可空 `user_id`、`status`、`display_name`、`avatar`、`email`、`phone`。
- `raw_profile`、`last_login_at`、`last_synced_at`、`created_at`、`updated_at`、`deleted_at`。
- 同一连接下一个 principal 最多关联一个本地用户；一个本地用户可关联多个连接的 principal。

### 4.4 `external_identifiers`

- `connection_id`、`principal_id`、`namespace`、`value`、`is_primary`。
- `(connection_id, namespace, value)` 唯一。
- 每个 principal 必须有且只有一个主 `login_subject` 才能用于认证登录。

## 5. API

### 5.1 管理 API

- `GET/POST /api/admin/identity-connections`
- `GET/PUT/DELETE /api/admin/identity-connections/:id`
- `POST /api/admin/identity-connections/:id/test`
- `POST /api/admin/identity-connections/batch`
- `GET /api/admin/identity-connections/stats`

列表支持 `category`、`providerType`、`capability`、`status`、分页。创建/更新把 `credentials` 按用途加密；密钥为空表示编辑时保持原凭据。

### 5.2 公共与用户 API

- `GET /api/identity-connections?category=&capability=authentication`
- `GET /external-auth/:identifier/authorize`
- `GET /external-auth/:identifier/callback`
- `GET /external-auth/:identifier/bind`
- `POST /external-auth/bind-with-login`
- `POST /external-auth/register`
- `GET /api/user/external-identities`
- `DELETE /api/user/external-identities/:id`
- `POST /external-auth/:identifier/miniprogram/login`
- `POST /external-auth/:identifier/miniprogram/qrcode`
- `GET /external-auth/:identifier/miniprogram/qrcode/:ticket`
- `POST /external-auth/:identifier/miniprogram/qrcode/scan`
- `POST /external-auth/:identifier/miniprogram/qrcode/confirm`

旧 `/social/*`、`/identity/*`、`/api/admin/social-providers`、`/api/admin/identity-providers`、`/api/user/social-accounts`、`/api/user/identity-accounts` 全部删除。

客户端配置将 `features.identityLogin` 更名为 `features.enterpriseLogin`，保留 `features.socialLogin`。

## 6. 数据流

1. 管理员创建连接并提交认证凭据。
2. 服务端校验配置，将凭据加密后保存。
3. 登录时按 `identifier` 加载连接与解密后的认证凭据，创建对应适配器。
4. 适配器返回标准档案和标识集合。
5. 服务按 `(connection_id, login_subject)` 查找 principal；已关联用户则登录，未关联则产生临时绑定票据。
6. 绑定或注册完成后建立 principal、本地用户和 identifiers 的关系。

## 7. 管理界面

- 导航“登录集成”改为“身份连接”，只有一个“连接管理”列表。
- 列表优先显示名称、业务分类、提供商、能力、状态、最后检查结果；协议字段进入详情。
- 创建流程先选择提供商与用途，再渐进展示厂商配置。
- OIDC 可选择 social 或 enterprise；飞书/企微/钉钉默认 enterprise。
- OIDC 登录页继续分别展示“企业登录方式”和“第三方账号登录”。
- 个人中心使用统一接口加载，但按连接 category 分区显示。

## 8. 安全策略

- 启动检测到存量凭据但缺少或无效主密钥时立即失败。
- 凭据解密只发生在服务内部，日志、DTO、错误和追踪字段不得包含密文或明文。
- state 一次性消费、5 分钟过期；绑定 state 必须携带已登录用户 ID。
- 不迁移、也不重新落库历史用户 access token/refresh token。
- 删除连接前检查关联 principal；有关联时只能禁用，不能删除。

## 9. 迁移与发布

迁移运行器按以下顺序执行：

1. SQL 版本 A 创建新表。
2. Go 数据迁移预检主密钥、跨旧表重复 identifier 和 provider type。
3. 在事务中复制连接、加密凭据、建立旧 ID 映射、复制账号与 identifiers。
4. 核对连接数、账号数、identifier 唯一性及每个账号的主登录标识。
5. SQL 版本 B 删除四张旧表。

任一步失败都不得执行版本 B。迁移完成后不可原地回滚，必须依赖部署前数据库备份。

发布使用维护窗口：停用入口、备份、配置主密钥、部署、迁移、同步修改全部外部回调和小程序客户端、冒烟验证、恢复服务。

## 10. 测试与验收

- 三种数据库均覆盖空库和存量数据迁移。
- 重复 identifier、未知 provider、无主密钥均阻止启动且保留旧表。
- 凭据密文不含明文；错误密钥无法解密；相同明文两次加密结果不同。
- 旧连接和绑定数量完整迁移，令牌字段不迁移。
- GitHub、Google、OIDC、QQ、微信、微信小程序、飞书、企微、钉钉认证与绑定通过新 API。
- 旧 API 返回 404；前端不再请求旧路径。
- `go test ./...` 与两个前端构建通过。

实施验收记录：

- [x] 分阶段 goose 迁移、旧数据复制、凭据加密和旧表硬切已实现。
- [x] 缺少主密钥、重复 identifier、非法 provider 类型会在破坏性切换前停止。
- [x] SQLite 空库/存量迁移、重复标识、密钥与密文测试通过；MySQL/PostgreSQL 提供等价版本 SQL，发布前仍需在目标版本实例执行预演。
- [x] 新管理、公共认证、绑定、外部身份和小程序路由已接入；路由测试确认旧 API 不存在。
- [x] `go test ./...`、管理端 typecheck/build、OIDC 前端 typecheck/build 纳入最终验收命令。

## 11. 阶段依赖

02–04 只能依赖本阶段的统一连接、凭据、principal 和 identifier，不得重新引入 social/identity 双模型。
