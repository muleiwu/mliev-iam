# 企业身份连接与目录同步实现审计报告

> 审计日期：2026-07-20
> 审计范围：`IDENTITY_CONNECTIONS_01` 至 `IDENTITY_CONNECTIONS_04` 的后端、数据库迁移、管理端和 OIDC 前端实现
> 审计方式：按阶段分组的三个只读子审计、主线程跨阶段安全复核、静态检查、单元测试、竞态测试和前端构建
> 原审计结论：**审计时实现不满足生产发布条件。**
> 2026-07-20 修复复验结论：**原报告中的阻断、高风险和中风险代码项已整改并通过本地自动化验证；真实数据库、浏览器/Redis 和三平台测试企业门槛未完成，因此仍不能发布或把 01–04 标记为全部完成。**

## 0. 修复复验附录（2026-07-20）

本节是原只读审计之后的实施复验；第 1–10 节保留原始发现和证据，便于追踪问题来源，不再代表当前代码状态。

### 0.1 原审计问题关闭情况

| 原问题组 | 修复结论 |
| --- | --- |
| OIDC 会话固定、state mix-up、恶意 opener、回调 XSS/票据泄露 | 已加入浏览器绑定、一次性 login ticket、原子 OAuth state、可信 opener origin、安全模板/CSP nonce 和 HttpOnly continuation。 |
| 小程序公开 ticket 可领取结果 | 已拆分 `scanHandle`、`pollToken`、`confirmToken`，轮询不返回 JWT/绑定票据。 |
| `bind-with-login` 防护与未激活用户登录 | 已复用 captcha、账号失败计数、IP/账号/continuation 限速，并在所有签发入口调用 `CanLogin()`。 |
| 目录/登录 principal 分裂与离职绕过 | 已统一完整标识 resolver；blocked principal 拒绝登录，多 principal 命中进入管理员 conflict 审批。 |
| 飞书 API、可变快照、成员撤销、部门移动/删除 | 已改为 Contact v3 根/子部门 + 直属用户遍历；新增不可变快照、supersede、operation lease、拓扑移动和来源所有权撤销。 |
| 连接编辑破坏能力、坏密钥覆盖 | 已实现字段级 merge、provider schema、capability 健康和 versioned keyring 全量解密验证。 |
| worker 无限接管、停机重放、连接禁用仍执行 | 已限制最大尝试、到限死信、独立 final context、租约复查、禁用取消与执行前重复校验。 |
| 三平台语义不一致 | 三平台完整性/scope/字段权限/稳定标识已统一，并通过同一 fixture 契约测试。 |

### 0.2 本轮通过的命令

```text
GOCACHE=/tmp/mliev-iam-go-cache GOWORK=off go test ./...
GOCACHE=/tmp/mliev-iam-go-cache GOWORK=off go test -race ./app/service ./app/service/protocol ./app/service/protocol/directory ./internal/pkg/credentialcrypto ./migrations
GOCACHE=/tmp/mliev-iam-go-cache GOWORK=off go vet ./...

cd admin-web
./node_modules/.bin/eslint <本轮 identity connection / directory sync / sync center 文件>
./node_modules/.bin/vue-tsc --noEmit --skipLibCheck -p apps/web-antd/tsconfig.json
cd apps/web-antd && ../../node_modules/.bin/vite build --mode production

cd frontend-oidc
./node_modules/.bin/vue-tsc -b
./node_modules/.bin/vite build

GOCACHE=/tmp/mliev-iam-go-cache GOWORK=off go test -tags=integration ./migrations -run '^$'
GOCACHE=/tmp/mliev-iam-go-cache GOWORK=off go test -tags=sandbox ./app/service/protocol/directory -run '^$'
```

### 0.3 未通过的外部验收及原因

- `go test -tags=integration ./migrations -run TestMigrationDialects -count=1`：测试代码编译成功，实际启动 `mysql:8.4` / `postgres:16` 时失败，原因是本机无可用 Docker provider。未取得真实 DDL、硬切和备份恢复结论。
- `go test -tags=sandbox ./app/service/protocol/directory -run Sandbox -count=1`：测试代码编译成功，实际运行因缺少 `FEISHU_SANDBOX_*`、`WECOM_SANDBOX_*`、`DINGTALK_SANDBOX_*` 环境变量失败。
- 真实 Redis + 浏览器会话固定、并发回调、二维码旁观者和进程级 worker 崩溃演练尚未在受控 E2E 环境执行。

因此当前准确结论是“代码整改完成、生产验收阻塞”，而不是“生产可用”。

## 1. 执行摘要

统一连接、加密凭据、外部 principal、目录快照、持久化任务、事件 inbox 以及企微/钉钉适配器等基础结构已经建立，现有后端测试和两个目标前端构建可以通过。

但审计发现以下主链路阻断问题：

1. 外部登录可被利用把受害者身份写入攻击者持有的 OIDC session。
2. 飞书目录适配器没有调用官方 Contact v3 的部门遍历和部门直属用户接口，真实飞书环境无法完成全量同步。
3. 目录产生的 principal 与登录产生的 principal 没有按 `open_id`、`union_id` 等别名合流，既会导致绑定失败，也可能绕过离职封禁。
4. 小程序二维码把公开 scene ticket 同时用作浏览器取 JWT/bind token 的凭证。
5. 成员关系删除、部门移动、并发 run 和审批快照一致性没有正确处理，可能遗留组织权限或重复创建本地用户。

因此，阶段 01–03 应回到“已实现、审计未通过”状态；阶段 04 应保持“代码完成、待真实企业联调且依赖前序整改”。

## 2. 阶段结论

| 阶段 | 审计状态 | 结论 |
| --- | --- | --- |
| 01 统一身份连接 | 审计未通过 | 模型、加密迁移和硬切路由基本完成；外部认证事务、回调页面和小程序票据存在严重安全问题。 |
| 02 飞书手动同步 | 审计未通过 | 标准目录模型和预览框架已建立；真实飞书 API、身份合流、组织移动和关系撤销未闭环。 |
| 03 自动化与治理 | 审计未通过 | 队列、计划、事件 inbox 和 worker 已实现；租约接管、优雅停机、并发同步和不可变审批证据存在缺陷。 |
| 04 企微与钉钉 | 待整改及外部联调 | 模拟契约测试已完成；真实测试企业权限、全量同步和事件回调尚未验收，且复用的同步引擎仍有前述缺陷。 |

## 3. 上线阻断问题

### 3.1 外部登录存在 OIDC 会话固定和身份冒用

相关代码：

- `app/controller/external_auth_controller.go:26-33`
- `app/controller/external_auth_controller.go:66-88`
- `app/controller/oidc_auth_flow_controller.go:1539-1548`
- `app/controller/oidc_auth_flow_controller.go:1624-1633`

攻击者可先创建自己持有的 OIDC session，再诱导受害者访问带该 `session_id` 的外部登录地址。外部回调没有校验该 session 与发起浏览器的关系，会直接调用 `MarkAsAuthenticated` 把受害者 User ID 写入攻击者的 session。公开的 `flow-status` 随后会向仅持有 session ID 的调用方返回 consent CSRF token。

整改要求：

- 使用 HttpOnly、SameSite 的一次性事务 Cookie 或等价浏览器证明。
- 将 connection、OIDC session、发起浏览器、redirect intent 和 state 绑定为同一认证事务。
- `flow-status` 不得仅凭 session ID 返回可完成授权的敏感令牌。

### 3.2 飞书目录适配器调用了错误的 API

相关代码：

- `app/service/protocol/directory/feishu.go:75-118`
- `app/service/protocol/directory/feishu.go:121-165`
- `app/service/protocol/directory/feishu_test.go:19-27`

当前实现调用：

- `GET /open-apis/contact/v3/departments`
- `GET /open-apis/contact/v3/users`

飞书官方的完整组织遍历场景需要组合“获取子部门列表”和“获取部门直属用户列表”，逐部门分页读取，再按稳定用户 ID 合并多部门成员关系。现有 mock 复刻了错误路径，因此测试通过但不能代表真实飞书可用。

官方依据：

- [获取子部门列表](https://open.feishu.cn/document/server-docs/contact-v3/department/children)
- [获取部门直属用户列表](https://open.feishu.cn/document/server-docs/contact-v3/user/find_by_department)
- [Go SDK 场景示例：获取部门下所有用户](https://open.feishu.cn/document/server-side-sdk/golang-sdk-guide/demo?lang=zh-CN)

整改要求：从配置的可见根部门开始拓扑遍历，对每个部门分页读取直属用户，按 `tenant_key + user_id` 去重，并加入严格遵循官方 URL、参数和响应结构的契约测试。

### 3.3 目录身份与企业登录身份没有合流

相关代码：

- `app/service/directory_sync_service.go:354-402`
- `app/service/external_auth_service.go:352-364`
- `app/service/external_auth_service.go:367-405`
- `app/service/protocol/authentication_adapter.go:132-148`

目录同步仅按 `directory_user_id` 查找 principal；外部登录仅按 `login_subject` 查找 principal，没有使用同一连接内的 `open_id`、`union_id` 等标识做无歧义合并。

影响：

- 先同步再登录时找不到已有 principal，后续绑定可能撞 `(connection_id, user_id)` 唯一约束。
- 先登录再同步时可能创建第二个 principal。
- 上游离职时可能只 block 目录 principal，真正用于登录的 principal 仍为 active。

整改要求：认证和目录统一通过完整标识集合解析 principal；稳定目录 ID 优先，其次匹配可信别名；多 principal 命中必须进入冲突审批，禁止静默迁移标识。

### 3.4 小程序二维码公开 ticket 可用于领取 JWT 或 bind token

相关代码：

- `app/service/social_qrcode_service.go:101-118`
- `app/service/social_qrcode_service.go:146-197`
- `app/service/social_qrcode_service.go:337-365`
- `config/autoload/router.go:329-332`

同一个 ticket 既被放入公开二维码 scene，又被浏览器状态接口用来返回 JWT/bind token；确认接口没有要求扫码设备产生的独立一次性证明。

整改要求：拆分公开 scan handle、不可见 browser polling secret 和移动端确认凭证；公开 scene 不得直接兑换登录结果。

## 4. 高风险问题

### 4.1 回调页面泄露 bind token，且存在同源脚本注入

相关代码：

- `app/controller/external_auth_controller.go:71-79`
- `app/controller/external_auth_controller.go:240-242`
- `app/controller/social_auth_controller.go:538-545`

问题包括：

- `postMessage(..., '*')` 会把 bind token 发送给任意恶意 `window.opener`。
- 外部昵称、头像和邮箱被直接拼入内联 `<script>`；`escapeJS` 没有处理 `</script>`、`<`、`>`、`&`、U+2028 和 U+2029。

整改要求：不再拼接 JavaScript；使用 HTML 安全的 JSON 序列化和模板输出，限制精确 target origin，敏感绑定事务优先放入 HttpOnly Cookie。

### 4.2 成员移出部门后旧关系不会撤销

相关代码：

- `app/service/directory_sync_service.go:282-305`
- `app/service/directory_sync_service.go:307-315`
- `app/service/directory_sync_service.go:812-839`

当前只 upsert 本次快照内的 membership，没有对上次存在、本次缺失的关系生成 disable/delete change。员工调岗、取消兼任或离职后，旧 `external_memberships` 和 `user_departments` 可能继续生效。

整改要求：为缺失关系生成受保护的撤销差异，只停用属于当前连接的本地关系，并覆盖调岗、取消兼任、主部门变化和负责人变化测试。

### 4.3 审批 run A 时可能应用 run B 覆盖后的快照

相关代码：

- `app/service/directory_sync_service.go:232-266`
- `app/service/directory_sync_service.go:686-706`
- `app/service/directory_sync_service.go:812-829`

外部快照表是跨 run 覆盖写；change 保存的主要是可变记录 ID；应用阶段又重新读取当前记录。因此 run A 获批后，run B 可以先覆盖快照，随后应用 A 实际写入未经 A 审批的数据。

整改要求：每个 run 保存不可变快照，或只从 change 的不可变 `AfterData` 应用；同一连接只能存在一个可应用 run，新 run 产生时旧 run 应过期。

### 4.4 并发同步或租约重放可能重复创建本地用户

相关代码：

- `app/service/directory_sync_service.go:154-175`
- `app/service/directory_sync_service.go:763-809`

连接没有 run/apply 互斥；应用 create change 时先创建用户，之后才复用 `principal.UserID`。两个 run 可以分别创建用户，principal 最终指向后创建者，先创建者成为孤儿。

整改要求：按 connection 序列化快照和应用；应用前锁定并重读 principal；已有 User ID 时必须复用；增加双 run 并发和租约重放测试。

### 4.5 部门创建依赖上游顺序，部门移动不会同步

相关代码：

- `app/service/directory_sync_service.go:333-351`
- `app/service/directory_sync_service.go:686-710`

父部门尚未映射时，子部门会回退到本地组织根；后续差异只比较名称和排序，不比较父节点，因此无法修复错误层级，也不会生成 `move` change。

整改要求：校验父节点缺失和循环，按拓扑顺序生成/应用部门变更，并正确更新 `parent_id`、`path`、`level` 以及子树路径。

### 4.6 禁用身份连接不会停止目录同步

相关代码：

- `app/service/directory_sync_service.go:842-852`
- `app/service/directory_sync_service.go:875-883`
- `app/service/sync_schedule_service.go:93`

同步仅检查 policy enabled，没有检查连接 enabled 状态和 `directory_sync` capability。管理员禁用连接后，计划任务仍可能继续访问上游并修改本地目录。

整改要求：计划入队、worker 执行和手动 run 均复查连接状态及 capability；禁用连接时取消未领取任务。

### 4.7 管理端编辑连接会破坏 capabilities 和 provider 配置

相关代码：

- `admin-web/apps/web-antd/src/views/identity-connection/index.vue:127-180`
- `app/service/protocol/social/oidc_adapter.go:76-90`
- `app/service/protocol/identity/wecom_adapter.go:29-40`

问题包括：

- 编辑时 capabilities 被固定提交为 `['authentication']`，会删除 `directory_sync` 和 `events`。
- OIDC 表单缺少必需的 `issuer`。
- 企业微信认证表单缺少必需的 `agentId`。
- payload 只重建固定 config 字段，迁移来的未知 provider 配置会丢失。

整改要求：使用 provider-specific schema 和表单；普通编辑采用字段级 PATCH；未知 config 和 capabilities 必须原样保留。

### 4.8 错误主密钥不会阻止启动，凭据编辑可能永久覆盖密文

相关代码：

- `migrations/runner.go:94-99`
- `app/service/identity_connection_service.go:280-303`
- `app/service/identity_connection_service.go:319-359`

启动只校验 Base64 和长度，不验证现有 credential envelope 能否被当前 key 解密。保存时又静默忽略读取/解密错误，并使用此次部分字段覆盖旧密文。

整改要求：启动时验证全部凭据可解密；使用 key version 对应的 keyring；除“记录不存在”外必须传播所有凭据读取错误，禁止在解密失败后覆盖。

### 4.9 任务租约接管不受最大重试次数限制

相关代码：

- `app/service/sync_job_service.go:56-94`
- `app/service/sync_job_service.go:125-140`

`Claim` 没有 `attempts < max_attempts` 条件。只有 worker 正常捕获错误并调用 `Fail` 时才会进入 dead letter；若进程在领取后反复崩溃，任务可以无限接管。

整改要求：原子地把超过上限的过期 leased 任务转入 dead letter，claim 只允许未超限任务。

### 4.10 优雅停机可能导致数据已修改但任务未完成

相关代码：

- `app/service/sync_worker.go:35-69`
- `app/service/sync_worker.go:179-184`

停机会取消公共 context；如果本地变更已经完成，但 `Complete` 使用已取消的 context 落库，任务会保持 leased，租约到期后重复执行。

整改要求：先停止领取新任务，再限时排空在途任务；最终状态用独立短超时 context 落库；租约丢失后禁止继续应用。

### 4.11 `bind-with-login` 绕过正常登录防护

相关代码：

- `config/autoload/router.go:326`
- `app/controller/external_auth_controller.go:94-105`
- `app/service/external_auth_service.go:197-210`

该公开接口没有复用正常登录的 captcha 和速率限制；失败时 bind token 不消费，可以被反复用于用户名/密码尝试。

整改要求：复用完整登录防护，按 bind token、账号和 IP 限速；失败达到阈值后冻结或消费 token；成功消费必须原子化。

### 4.12 外部登录签发令牌前没有校验 `CanLogin()`

相关代码：

- `app/service/external_auth_service.go:442-456`
- `app/service/user_service.go:191-200`
- `app/service/user_service.go:589-592`
- `app/middleware/auth_middleware.go:64-77`

统一外部登录路径通过 `GetUserByID` 读取用户后直接签发 JWT，没有执行本地密码登录使用的 `CanLogin()`。中间件只检查 status 和 session revoke 时间，没有检查 `IsActivated`，因此 active 但未激活的用户可能绕过激活限制。

整改要求：所有登录方式统一调用同一账户可登录策略，并在建立 OIDC session 或签发任何令牌前执行。

## 5. 中风险与需求缺口

### 5.1 OAuth state 未绑定 connection，且消费不是原子的

相关代码：

- `app/service/external_auth_service.go:57-95`
- `app/controller/external_auth_controller.go:50-66`

state 没有保存创建它的 connection，回调路径中的 identifier 可以替换；`GET` 后 `DEL` 允许并发回调同时读取同一 state。

整改要求：state 保存 connection ID、发起 origin 和 redirect intent，并使用 Redis `GETDEL` 或 Lua 原子消费。

### 5.2 部门 delete change 是 no-op，却被标记为 applied

相关代码：

- `app/service/directory_sync_service.go:662-683`
- `app/service/directory_sync_service.go:686-694`

管理员批准部门删除后，`applyOrgChange` 直接返回 nil，通用路径随后把 change 标记为 applied，但本地部门没有任何变化。

整改要求：未实现删除前禁止把 change 标为 applied；或者实现受保护的停用/软删并校验组织根、子部门和成员关系。

### 5.3 `syncScope` 和 `fieldMapping` 没有参与同步

策略能够存储这两个字段，但管理端固定提交空对象，目录拉取和差异计算也没有使用它们。当前行为始终同步应用全部可见通讯录，可能超出管理员预期范围。

整改要求：让 sync scope 决定上游遍历根和过滤规则，让 field mapping 参与标准字段投影，并在预览中显示实际范围。

### 5.4 组织根隔离允许父子根重叠

当前服务检查和数据库唯一约束只阻止两个连接选择完全相同的 root ID；连接 A 选择 X、连接 B 选择 X 的子部门 Y 时仍会通过，两个连接可以写入同一子树。

整改要求：保存策略时拒绝所有祖先/后代重叠根，并为本地目录对象增加明确来源所有权。

### 5.5 连接健康检查只检查 authentication

相关代码：

- `app/service/identity_connection_service.go:174-178`
- `app/service/identity_connection_service.go:247-266`

任何 enabled 连接更新后都会构建 authentication adapter。目录-only 或事件-only 连接会校验失败并被强制 disabled；多 capability 连接的“健康”状态也不能反映目录和事件能力。

整改要求：按 capability 分别诊断并记录健康状态，只在相关 capability 的显式启用操作失败时拒绝或回滚。

### 5.6 限流等待单位错误

相关代码：

- `app/service/protocol/directory/http_client.go:43-55`
- `app/service/protocol/directory/feishu.go:296-308`

`Retry-After` 的秒数被乘以 `100ms`，例如 `Retry-After: 1` 实际只等待 100ms，会加剧上游限流。

整改要求：按 HTTP 规范解析秒数或 HTTP-date，执行真实秒级等待并设置合理上限。

### 5.7 管理端事件回调 URL 在前后端分域时错误

相关代码：

- `admin-web/apps/web-antd/src/views/identity-connection/directory-sync.vue:105-109`
- `admin-web/apps/web-antd/src/api/request.ts:22`

页面使用 `window.location.origin` 生成回调 URL，但 API 可以使用独立基址。管理端和 IAM API 分域部署时，复制出的 URL 会指向管理前端。

整改要求：由后端根据公开服务基址返回回调 URL，或显式使用绝对 API 基址。

### 5.8 大型差异和失败变更缺少可恢复操作

- 管理页固定只读取最多 500 条 change，超过部分无法逐项审批。
- change 应用失败后状态变为 `failed`，重跑只选择 `approved`，审核接口又只允许 `pending`，失败项无法恢复。

整改要求：加入服务端分页、筛选、选择审批，以及明确的 failed → approved 重试操作。

### 5.9 旧实现源码和前端 API 仍残留

旧路由已经不再注册，但下列旧代码仍存在并被部分导出：

- `app/controller/admin/social_provider_controller.go`
- `app/controller/admin/identity_provider_controller.go`
- `app/service/social_auth_service.go`
- `app/service/identity_auth_service.go`
- `admin-web/apps/web-antd/src/api/core/social-login.ts`
- `admin-web/apps/web-antd/src/api/core/identity-provider.ts`
- `admin-web/apps/web-antd/src/views/social-login/`
- `admin-web/apps/web-antd/src/views/identity-provider/`

这不会恢复旧路由，但违反硬切后的代码清理目标，增加误用和回归风险。

### 5.10 迁移对重复 union ID 的校验不足

`migrations/identity_connection_data.go` 在 union ID 冲突时可能跳过第二个别名，但仍继续完成迁移并删除旧表。当前数量核对主要检查 principal 总数，没有校验每个旧账号的完整标识集合。

整改要求：预检 union/open 标识冲突并停止切换，或通过明确的 principal 合并流程处理。

## 6. 已验证通过的实现

- AES-256-GCM 使用随机 nonce 和 AAD；凭据 DTO 只返回配置摘要，不返回密文或明文。
- 新 principal/identifier 模型没有 access token 和 refresh token 字段；登录获得的上游 token 没有长期保存。
- SQLite 硬切测试覆盖连接和绑定复制、凭据加密、旧 token 不迁移、重复 identifier 阻断以及旧表删除。
- 新管理、公共认证、绑定、注册、小程序和外部身份路由已注册；旧 social/identity 路由未注册。
- `features.enterpriseLogin` 已贯通后端配置和 OIDC 前端；登录页按企业和社交类别分组展示。
- 目录标准模型覆盖稳定 ID、别名、多部门、主部门、负责人、职位和状态。
- 企微与钉钉模拟适配器测试覆盖 token、部门递归、用户分页、多部门、负责人、状态和加密事件回执。
- 危险用户停用保持人工审批；存在其他 active principal 时不会停用整个用户，最后身份失效时会撤销 IAM/OIDC 会话。
- 管理端页面视觉层级、窄屏横向表格和深色模式方向基本合理，未发现阻断性的界面设计问题。

## 7. 本次执行的验证命令

### 7.1 通过

```text
GOCACHE=/private/tmp/mliev-iam-gocache go test ./... -count=1
GOCACHE=/private/tmp/mliev-iam-gocache go test -race ./app/service ./app/service/protocol/directory ./app/service/protocol/event
GOCACHE=/private/tmp/mliev-iam-gocache go vet ./...

cd admin-web/apps/web-antd
pnpm run typecheck
pnpm run build

cd frontend-oidc
pnpm run build
```

### 7.2 未通过或未覆盖

- `admin-web` 全 monorepo `pnpm run check:type`：`web-install` 缺少 `@vben/types/global`，但本次目标 `web-antd` 单独 typecheck 通过。
- `admin-web` 全量 lint：仓库已有 stylelint 错误导致失败；本次新增文件另有 Prettier 格式警告。
- MySQL/PostgreSQL：只有等价迁移 SQL，没有在本次审计中启动真实数据库执行不可逆硬切测试。
- 飞书、企微、钉钉：没有真实测试企业凭据，未执行权限诊断、全量同步和事件回调联调。

## 8. 建议整改顺序

### 第一批：安全止血

1. 修复 OIDC session 固定和外部认证浏览器事务绑定。
2. 修复 `postMessage('*')`、回调 XSS 和 bind token 传递方式。
3. 重构小程序二维码的 scan handle、polling secret 和确认凭证。
4. 为 `bind-with-login` 增加登录防护；所有登录方式统一执行 `CanLogin()`。
5. 原子消费 OAuth state，并绑定 connection 和发起 origin。

### 第二批：完成飞书真实闭环

1. 按官方 Contact v3 API 重写部门与用户遍历。
2. 实现目录/登录多标识 principal 合流和冲突审批。
3. 实现不可变 run 快照、成员关系撤销、部门移动和拓扑应用。
4. 增加连接级同步互斥和创建用户前的事务性重检。

### 第三批：自动化治理

1. 修复租约接管上限、优雅停机和重复执行。
2. 连接禁用时停止计划、事件和 worker 写入。
3. 实现失败 change 重试、分页审批和真实部门删除语义。
4. 让 sync scope、field mapping、字段权限诊断真正参与同步。

### 第四批：发布验收

1. 修复管理端 provider-specific 表单和 capabilities 保留。
2. 在真实 MySQL、PostgreSQL、SQLite 上执行完整迁移演练。
3. 完成飞书真实企业闭环后，再分别进行企微和钉钉沙箱联调。
4. 所有阻断和高风险问题增加回归测试后，重新执行本报告中的全套命令。

## 9. 发布门槛

只有满足以下条件后才可恢复“已完成”状态：

- 本报告所有上线阻断问题关闭并有自动化回归测试。
- 高风险的数据一致性、凭据和队列问题关闭。
- 飞书真实测试企业完成诊断、全量预览、审批应用、定时同步、事件刷新和离职验证。
- MySQL、PostgreSQL、SQLite 完成空库及存量硬切演练并验证备份恢复。
- 企微和钉钉在各自真实测试企业完成目录与事件联调。
- 后端测试、竞态测试、管理端 typecheck/build、OIDC 前端 build 全部通过。

## 10. 原审计边界

- 原始审计为只读代码审计；其后的代码修复与复验结果见第 0 节。
- 未修改用户已有的 `frontend-oidc/src/main.ts` 工作区变更和 `outputs/` 未跟踪目录。
- 平台 API 判断以 2026-07-20 可访问的官方文档为依据；后续实施时应再次核对厂商最新版本和权限要求。
