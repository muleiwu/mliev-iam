# 飞书、企业微信、钉钉企业身份源对接评估

> 评估日期：2026-05-22  
> 适用范围：MLiev IAM 作为统一认证服务，对接飞书、企业微信、钉钉作为上游企业身份源，并继续向下游业务系统输出 OIDC / OAuth2 / SAML / CAS 等统一认证能力。

## 1. 结论

当前项目已经具备统一认证中心的核心基础：OIDC/OAuth2 服务端、JWT、RBAC、部门、用户、应用访问范围、社会化登录提供商配置与账号绑定能力。

但飞书、企业微信、钉钉不应只作为“三个社交登录按钮”接入。它们在企业场景下更适合作为“企业身份源 / 通讯录源”接入 IAM，由 IAM 统一完成本地账号、组织、角色、应用访问控制和协议输出。

推荐方案：

1. 短期复用现有社会化登录框架，新增三平台登录适配器，完成授权登录和账号绑定。
2. 中期新增企业身份源与通讯录同步模型，支持部门、用户、用户部门关系、离职/冻结状态同步。
3. 长期建设增量事件、角色映射、审计、同步差异预览、SCIM 2.0 和密钥治理能力。

## 2. 当前项目现状

### 2.1 已具备能力

1. OIDC/OAuth2 IdP 能力
   - 协议端点在 `config/autoload/router.go` 注册，路径形态为 `/oidc/:client_id/*`。
   - 支持 authorize、token、userinfo、introspect、revoke、jwks、discovery。
   - `Application` 模型已经抽象出 OIDC、SAML、CAS、OAuth2 协议类型，并支持应用访问范围。

2. 用户和组织模型
   - `User` 支持 UUID、用户名、邮箱、手机号、昵称、真实姓名、头像、状态、激活状态、扩展属性。
   - `Department` 支持公司、事业部、部门、团队层级，以及负责人、数据权限范围等字段。
   - `UserDepartment` 支持多部门、主部门、职位、负责人类型、离开时间。

3. RBAC 和数据权限
   - `Role` 支持层级、状态、数据权限范围。
   - `Permission` 支持菜单、按钮、API、数据权限。
   - `UserRole` 支持授权范围、审批、过期时间、部门约束。

4. 社会化登录基础
   - `SocialProvider` 已支持动态配置 OAuth2/OIDC 类提供商。
   - `UserSocialAccount` 已支持第三方 OpenID / UnionID 与本地用户绑定。
   - `SocialAuthService` 已有 state 防 CSRF、授权回调、未绑定账号临时票据、绑定已有账号、注册新账号等流程。

### 2.2 当前缺口

1. 缺少飞书、企业微信、钉钉 provider type 和专用适配器。
2. 现有 `SocialProvider` 偏“登录提供商”，还不能完整表达企业身份源的通讯录同步、事件回调、字段映射、角色映射、同步策略。
3. 现有 `UserSocialAccount` 可以承载登录绑定，但不适合承载完整生命周期同步。需要独立的外部身份映射表。
4. 缺少以下生产能力：
   - 全量/增量通讯录同步任务。
   - 外部部门 ID 与本地部门 ID 映射。
   - 外部用户 ID 与本地用户 ID 映射。
   - 同步冲突处理。
   - 同步日志和失败重放。
   - 事件回调验签、解密、去重、死信。
   - 密钥加密存储和轮换。

## 3. 统一认证架构建议

IAM 在整体架构中应同时扮演两个角色：

1. 上游身份消费者
   - 对接飞书、企业微信、钉钉。
   - 获取用户身份、通讯录、部门、状态和事件。
   - 将外部身份归一化为 IAM 本地用户、部门、角色。

2. 下游身份提供者
   - 对业务系统输出 OIDC/OAuth2/SAML/CAS。
   - 下游系统不直接感知飞书、企业微信、钉钉差异。
   - 下游只信任 IAM 发出的用户身份、组织、角色、权限和 token。

推荐链路：

```text
飞书 / 企业微信 / 钉钉
        |
        | 登录授权 / 通讯录同步 / 事件回调
        v
MLiev IAM
        |
        | 本地用户 / 部门 / 角色 / 应用访问范围
        v
OIDC / OAuth2 / SAML / CAS
        |
        v
业务系统
```

## 4. 三平台对接评估

### 4.1 飞书

适合作为首个落地平台。飞书开放平台的授权登录和通讯录 API 比较标准化，文档也提供 Markdown 版本，便于自动化验证。

登录能力：

- 浏览器授权登录可获取 `user_access_token`，再调用用户信息接口获取用户身份。
- 用户信息里可返回 `open_id`、`union_id`、`tenant_key`、姓名、头像；申请权限后可返回 `user_id`、邮箱、手机号、工号。

通讯录能力：

- 自建应用通过 `tenant_access_token` 调用服务端 API。
- `tenant_access_token` 最大有效期为 2 小时。
- 批量用户信息接口可返回用户 ID、名称、邮箱、手机号、状态、所属部门等字段，但敏感字段需要额外权限。

推荐主键策略：

- 登录绑定键：`provider_id + open_id`。
- 企业稳定身份键：`tenant_key + user_id`。
- 辅助关联键：`union_id`、`email`、`mobile`，但 email/mobile 不应作为唯一可信主键。

主要风险：

- 字段返回依赖权限审批，手机号、邮箱、工号等敏感字段可能拿不到。
- 不同 ID 类型含义不同，`open_id`、`union_id`、`user_id` 需要清晰建模。
- 员工离职、冻结、未激活等状态需要映射到 IAM 用户状态。

### 4.2 企业微信

企业微信不是标准 OAuth2，需要专用适配器。它的典型链路是：构造企业微信网页授权链接，用户授权后回调 code，后端用企业 access_token 和 code 换取 `UserId`，再调用通讯录接口读取成员详情。

登录能力：

- 需要配置 `CorpID`、`AgentID`、应用 `Secret`、回调域名。
- 授权 code 可换取企业成员身份，成员场景通常返回 `UserId`。

通讯录能力：

- 可读取部门列表、部门成员、成员详情。
- 权限受应用可见范围、通讯录权限等级、可信域名、可信 IP 等限制。

推荐主键策略：

- 企业稳定身份键：`corp_id + userid`。
- 如返回 `external_userid`、手机号、邮箱，只作为补充属性或匹配辅助。

主要风险：

- 回调域名和可信 IP 配置要求严格。
- 通讯录权限等级会影响可读取字段。
- 自建应用、第三方应用、服务商应用的授权模型不同，第一阶段建议只支持自建应用。

### 4.3 钉钉

钉钉也需要专用适配器。其登录、免登、通讯录接口与标准 OAuth2 有差异，建议以“企业内部应用”作为第一阶段目标。

登录能力：

- 企业内部应用通过 AppKey/AppSecret 获取应用访问凭证。
- 授权 code 或免登 code 可换取用户身份，再读取用户通讯录信息。

通讯录能力：

- 通讯录产品覆盖部门、用户、部门用户列表、用户详情、管理员列表等能力。
- 钉钉文档页面较动态，具体接口路径、scope 和新版/旧版 API 需要在开放平台后台联调时最终确认。

推荐主键策略：

- 企业稳定身份键：`corp_id + userid`。
- `unionid` 用于跨应用关联，不能替代企业内 `userid` 的组织身份语义。

主要风险：

- 钉钉新旧 API 并存，接口命名和路径存在历史包袱。
- 权限申请和应用类型会影响可调用接口。
- 需要明确是否只支持企业内部应用，还是也支持 ISV/套件授权。

## 5. 建议新增领域模型

### 5.1 identity_providers

企业身份源配置。不要直接复用 `social_providers` 承载所有企业同步能力。

建议字段：

- `id`
- `name`
- `type`：`feishu` / `wecom` / `dingtalk` / `oidc` / `ldap` / `saml`
- `status`
- `client_id`
- `client_secret_encrypted`
- `config_json`
  - 飞书：`app_id`、`tenant_key`、`redirect_url`、`scopes`
  - 企业微信：`corp_id`、`agent_id`、`redirect_url`
  - 钉钉：`corp_id`、`app_key`、`redirect_url`
- `sync_policy_json`
  - 是否自动建号
  - 是否自动禁用离职人员
  - 是否覆盖本地字段
  - 同步频率
  - 字段映射
- `event_config_json`
  - 回调 token
  - encoding aes key
  - secret
- `created_at`
- `updated_at`
- `deleted_at`

### 5.2 external_identities

外部用户身份映射。

建议字段：

- `id`
- `provider_id`
- `user_id`
- `external_user_id`
- `external_open_id`
- `external_union_id`
- `external_tenant_id`
- `employee_no`
- `email`
- `phone`
- `raw_profile_json`
- `status`
- `last_synced_at`
- `created_at`
- `updated_at`

唯一索引建议：

- `provider_id + external_user_id`
- `provider_id + external_open_id`

### 5.3 external_departments

外部部门映射。

建议字段：

- `id`
- `provider_id`
- `department_id`
- `external_department_id`
- `external_parent_id`
- `name`
- `path`
- `raw_json`
- `last_synced_at`

### 5.4 directory_sync_jobs

同步任务。

建议字段：

- `id`
- `provider_id`
- `type`：`full` / `incremental` / `manual`
- `status`：`pending` / `running` / `success` / `failed`
- `cursor`
- `started_at`
- `finished_at`
- `stats_json`
- `error`

### 5.5 provisioning_rules

入站账号和权限规则。

建议字段：

- `id`
- `provider_id`
- `name`
- `priority`
- `condition_json`
- `actions_json`
  - 分配角色
  - 加入部门
  - 设置应用访问范围
  - 禁用/冻结用户
- `status`

## 6. 登录流程设计

### 6.1 已绑定账号登录

```text
用户点击飞书/企业微信/钉钉登录
  -> IAM 创建 state，保存 session_id
  -> 跳转平台授权页
  -> 平台回调 IAM，携带 code 和 state
  -> IAM 校验 state
  -> IAM 换取平台用户身份
  -> 查询 external_identities 或 user_social_accounts
  -> 找到本地用户
  -> 校验用户状态、应用访问范围、MFA/风控策略
  -> 标记 OIDC session 已认证
  -> 下游应用继续 OIDC 授权码流程
```

### 6.2 未绑定账号登录

推荐提供三种策略，由管理员配置：

1. 自动建号
   - 适合企业内部 IAM 初始上线。
   - 需要通讯录权限足够，至少能拿到稳定员工 ID。

2. 按邮箱/手机号自动匹配
   - 只允许匹配已验证字段。
   - 需要审计记录，避免误合并。

3. 人工绑定/审核
   - 适合生产早期或数据质量不高时。
   - 用户登录后进入绑定页或等待管理员审核。

## 7. 通讯录同步设计

### 7.1 全量同步

建议执行顺序：

1. 获取 access token。
2. 拉取部门树。
3. Upsert 本地部门和 `external_departments`。
4. 拉取用户列表。
5. Upsert 本地用户和 `external_identities`。
6. 更新用户部门关系。
7. 根据规则映射角色。
8. 处理本次同步未出现的用户：
   - 标记离职、冻结、禁用或保留，按策略执行。
9. 写入同步日志和统计。

### 7.2 增量同步

接入平台事件回调后处理：

- 用户新增。
- 用户资料变更。
- 用户离职或禁用。
- 部门新增。
- 部门变更。
- 部门删除。
- 用户部门关系变更。

事件处理要求：

- 验签和解密。
- 幂等去重。
- 失败重试。
- 死信保留。
- 支持按事件重放。

## 8. 字段映射建议

| IAM 字段 | 飞书 | 企业微信 | 钉钉 | 说明 |
| --- | --- | --- | --- | --- |
| `User.Username` | `user_id` / `employee_no` | `userid` | `userid` | 建议生成稳定用户名，避免姓名变更影响登录 |
| `User.RealName` | `name` | `name` | `name` | 可被同步覆盖 |
| `User.Nickname` | `nickname` / `name` | `alias` / `name` | `name` | 展示字段 |
| `User.Email` | `email` | `email` | `email` | 需权限，作为辅助匹配 |
| `User.Phone` | `mobile` | `mobile` | `mobile` | 需权限，作为辅助匹配 |
| `User.Avatar` | `avatar_url` | `avatar` | `avatar` | 可选 |
| `User.Status` | `status` | `enable` / 删除事件 | `active` / 离职信息 | 需要归一化 |
| `Department.Code` | `department_id` | `department_id` | `dept_id` | 建议增加外部部门映射表 |
| `UserDepartment.IsPrimary` | 主部门排序 | 主部门或部门列表 | 主部门或部门列表 | 平台语义不同，需适配 |

## 9. IAM 统一认证服务应具备能力

### 9.1 身份源管理

- 支持多个身份源并存。
- 支持身份源启停、优先级、应用范围。
- 支持自建应用和未来 ISV/套件扩展。
- 支持配置测试和健康检查。

### 9.2 账号生命周期

- 自动建号。
- 自动禁用离职用户。
- 自动恢复重新入职用户。
- 字段覆盖策略。
- 账号合并和冲突处理。
- 唯一登录方式解绑保护。

### 9.3 组织生命周期

- 部门树同步。
- 多部门同步。
- 主部门同步。
- 负责人同步。
- 部门删除/冻结策略。
- 部门路径修复。

### 9.4 权限和访问控制

- 部门到角色映射。
- 岗位/职务到角色映射。
- 标签/用户组到角色映射。
- 应用访问范围自动维护。
- 数据权限范围继承。
- 例外授权和人工覆盖。

### 9.5 协议输出

- OIDC Authorization Code + PKCE。
- OIDC Discovery 和 JWKS。
- OAuth2 Client Credentials。
- SAML / CAS 扩展。
- Token Introspection。
- Token Revocation。
- 单点退出和会话同步。

### 9.6 安全治理

- 外部平台密钥加密存储。
- 外部 access token 缓存，不长期明文落库。
- 回调验签、解密、防重放。
- state、nonce、PKCE。
- 登录限流和异常登录审计。
- IP 白名单。
- 管理员操作审计。

### 9.7 运维可观测

- 同步任务状态。
- 同步差异预览。
- 同步失败明细。
- 重试和重放。
- 身份源 API 调用统计。
- 登录成功率和失败原因。
- 平台限流监控。

## 10. 实施路线

### P0：三平台登录 MVP

目标：支持用户使用飞书、企业微信、钉钉登录 IAM，并通过 IAM 完成下游 OIDC 登录。

任务：

- 新增 ProviderType：`feishu`、`wecom`、`dingtalk`。
- 新增三平台登录适配器。
- 管理端新增三平台配置表单。
- 登录页展示三平台入口。
- 支持账号绑定、自动建号可配置。
- 保留现有 OIDC session 对接方式。

建议验收：

- 已绑定用户可登录。
- 未绑定用户可进入绑定或注册流程。
- 登录后下游 OIDC 应用可拿到 IAM token。
- state 校验有效，回调 URL 安全。

### P1：通讯录全量同步

目标：IAM 可以从三平台同步用户、部门、用户部门关系。

任务：

- 新增 `identity_providers`。
- 新增 `external_identities`。
- 新增 `external_departments`。
- 新增同步任务和同步日志。
- 支持手动触发全量同步。
- 支持字段映射和覆盖策略。
- 支持离职/禁用策略。

建议验收：

- 部门树可同步。
- 用户可同步。
- 用户多部门和主部门可同步。
- 离职用户可按策略禁用。
- 同步失败可查看原因。

### P2：增量事件和权限自动化

目标：身份源变更可实时进入 IAM，并自动影响应用访问控制。

任务：

- 接入平台事件回调。
- 验签、解密、去重、重试、死信。
- 建设角色映射规则。
- 建设应用访问范围自动维护。
- 增加同步差异预览和回滚。
- 增加密钥轮换。

### P3：标准化和生态扩展

目标：IAM 作为企业身份中台，对接更多上游和下游。

任务：

- 支持 SCIM 2.0。
- 支持 LDAP/AD。
- 支持更多 SaaS 应用的出站账号同步。
- 支持多租户隔离。
- 支持更完整的审计与报表。

## 11. 优先级建议

建议优先级：

1. 飞书登录 + 飞书通讯录同步。
2. 企业微信登录。
3. 钉钉登录。
4. 企业微信/钉钉通讯录同步。
5. 三平台事件增量。
6. 角色映射和应用访问范围自动化。

原因：

- 飞书开放平台文档结构更利于首个适配器落地。
- 企业微信和钉钉需要更多后台配置和联调，适合作为第二阶段。
- 通讯录同步比登录复杂，应在登录 MVP 稳定后推进。

## 12. 外部参考

- 飞书：自建应用获取 tenant_access_token  
  <https://open.feishu.cn/document/server-docs/authentication-management/access-token/tenant_access_token_internal.md>
- 飞书：获取用户信息  
  <https://open.feishu.cn/document/deprecated-guide/app-login/api-reference/get-user-info?lang=zh-CN>
- 飞书：批量获取用户信息  
  <https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/contact-v3/user/batch>
- 企业微信：获取访问用户身份  
  <https://developer.work.weixin.qq.com/document/path/91023>
- 企业微信：部门与成员接口参考入口  
  <https://developer.work.weixin.qq.com/document/path/90208>
- 钉钉：通讯录概述  
  <https://open.dingtalk.com/document/development/contacts-overview>
- 钉钉：获取用户通讯录个人信息  
  <https://open.dingtalk.com/document/isvapp-server/dingtalk-retrieve-user-information>

