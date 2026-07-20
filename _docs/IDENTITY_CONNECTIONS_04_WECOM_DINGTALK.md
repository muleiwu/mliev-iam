# 身份连接 04：企业微信与钉钉目录扩展

> 状态：已实现，待真实测试企业联调
> 前置：[03 自动化与治理](./IDENTITY_CONNECTIONS_03_AUTOMATION_GOVERNANCE.md)

## 1. 目标

在不改变统一目录模型和同步引擎的前提下，增加企业微信、钉钉的目录读取、事件接入、诊断和管理配置，使飞书/企微/钉钉对上层产生一致结果。

## 2. 非目标

- 不支持企微第三方/服务商代开发、钉钉 ISV 套件授权。
- 不实现 SCIM、LDAP、AD、HRIS、双向写回或下游账号供给。
- 不在平台适配器中直接修改 IAM 用户、部门、角色或应用权限。

## 3. 统一契约

企业微信与钉钉实现 02 的 `DirectoryAdapter` 和 03 的 `EventAdapter`。所有分页、限流和厂商字段差异在适配器内归一化为 `DirectoryUser`、`OrgUnit`、`Membership`、`DirectoryEvent`。

可重试错误：网络超时、429、平台 5xx、令牌临时失效。不可重试错误：凭据无效、权限不足、应用类型不支持、数据违反稳定标识要求。

## 4. 企业微信设计

### 4.1 支持范围与标识

- 仅企业自建应用/通讯录同步。
- 连接外部租户为 `corp_id`，稳定用户键为 `corp_id + userid`。
- `directory_user_id=corp_id + ":" + userid`，保存 `open_userid`、`unionid` 等可用别名。
- 部门稳定键为 `corp_id + department_id`。

### 4.2 凭据

- 认证用途：`corp_id`、`agent_id`、登录应用 secret。
- 目录用途：`corp_id`、通讯录 secret、可见根部门。
- 事件用途：`token`、`encoding_aes_key`、回调配置。

### 4.3 数据与事件

- 同步部门、用户、多部门、主部门、负责人、职位和启用状态。
- 支持成员新增/变更/删除、部门新增/变更/删除、成员部门移动等通讯录事件。
- 回调必须完成 URL 验证、签名校验和 AES 解密后才可进入 inbox。

## 5. 钉钉设计

### 5.1 支持范围与标识

- 仅企业内部应用。
- 连接外部租户为 `corp_id`，稳定用户键为 `corp_id + userid`。
- `directory_user_id=corp_id + ":" + userid`，保存 `openid`、`unionid` 等可用别名。
- 部门稳定键为 `corp_id + dept_id`。

### 5.2 凭据

- 认证用途：`client_id/app_key`、`client_secret`、`corp_id`。
- 目录用途：`app_key`、`app_secret`、`corp_id`、可见根部门。
- 事件用途：事件订阅 secret/token 及平台要求的回调配置。

### 5.3 数据与事件

- 同步部门、用户、成员关系、主部门、负责人、职位和 active 状态。
- 支持用户与部门新增、变更、删除等组织事件；事件只触发重新读取。
- 针对钉钉新旧 API 差异固定使用当前企业内部应用服务端 API，不在运行时混用版本。

## 6. API 与管理界面

不新增平台专用同步 API；继续使用统一连接、诊断、run、job、schedule、event API。

连接创建/编辑表单按 provider type 展示厂商配置，并明确区分认证、目录、事件三套凭据。诊断输出：令牌、通讯录范围、用户字段权限、部门权限、事件配置和限流状态。

## 7. 安全策略

- 三种凭据按 purpose 分开加密、更新和轮换。
- 回调验签失败返回拒绝且不落任务；挑战验证只返回平台要求的响应。
- 通讯录 API 使用最小读取权限；首版不存在写权限。
- provider 原始错误对管理员给出可操作提示，对日志保留 request ID，不输出 secret/token。

## 8. 迁移与发布

- 不新增平台专用业务表；仅在统一连接配置 schema 中登记 provider 字段。
- 现有企微/钉钉认证连接升级后默认只有 `authentication`；目录与事件必须分别完成诊断后启用。
- 按企微、钉钉分别进行沙箱/测试企业联调，未联调通过的平台 capability 保持关闭。

## 9. 测试与验收

- 使用 `httptest` 覆盖令牌、分页、部门、用户、成员、限流、过期凭据和平台错误映射。
- 覆盖企微 URL 验证、签名、AES 解密、重复与乱序事件。
- 覆盖钉钉事件验签、重复与乱序事件。
- 使用同一标准目录 fixture 分别经过飞书、企微、钉钉适配器，最终产生相同的本地用户、部门和 membership。
- 三个平台均通过手动预览、审批应用、定时同步、事件刷新、死信重放和每日对账。

实施验收记录：

- [x] 企业微信目录适配器覆盖 token、递归成员、部门、多部门、负责人、状态和稳定标识归一化。
- [x] 钉钉目录适配器固定使用内部应用 TopAPI v2，覆盖递归部门、用户分页、去重、多部门、负责人和状态。
- [x] 企业微信 GET URL 验证、SHA-1 验签、AES-CBC 解密和事件确认响应已实现并有契约测试。
- [x] 钉钉加密回调验签、AES-CBC 解密、`check_url` 和加密 `success` 响应已实现并有契约测试。
- [x] 两个平台复用统一快照、差异、审批、任务、对账和死信能力；没有平台适配器直接写本地目录。
- [x] 同一标准组织 fixture 经过飞书、企业微信、钉钉适配器后，部门、用户状态、职位、主部门和负责人语义投影一致。
- [x] 管理端按 provider type 展示认证、目录和事件的独立配置字段与统一诊断入口。
- [ ] 企业微信与钉钉真实测试企业的权限诊断、全量同步和事件回调联调（需要外部测试企业与凭据）。

实现依据：[钉钉部门 API](https://open.dingtalk.com/document/orgapp-server/obtain-a-sub-department-id-list-v2)、[钉钉回调 URL 验证](https://open.dingtalk.com/document/orgapp-server/validating-and-updating-callback-url-events)、[企业微信开发者中心](https://developer.work.weixin.qq.com/document/)。

## 10. 验收标准

- 企业微信、钉钉各至少一个真实测试企业完成权限诊断、全量同步和事件联调。
- 多源组织根隔离有效；不存在平台适配器直接写本地目录的路径。
- 管理端无需理解厂商原始字段即可查看统一差异、任务和错误。
- 全量后端测试与前端构建通过，实施状态文档更新。

## 11. 后续路线

SCIM、LDAP/AD、HRIS、企业应用出站供给、角色/应用范围自动映射和双向同步另行立项，不扩张本阶段范围。
