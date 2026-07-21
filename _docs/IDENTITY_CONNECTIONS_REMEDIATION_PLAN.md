# 企业身份连接修复与生产验收计划

> 建立日期：2026-07-20
> 执行顺序：严格按 01 → 02 → 03 → 04；前一阶段的自动化测试与验收清单未关闭时，后一阶段不得恢复为“已完成”。
> 审计依据：[实现审计报告](./IDENTITY_CONNECTIONS_IMPLEMENTATION_AUDIT_2026-07-20.md)

## 1. 最初目标与最终目标

最初目标是把企业登录和社交登录收敛到统一 `identity_connections`，用飞书打通企业目录到 IAM 用户、部门和成员关系，再增加任务、事件与治理能力，最后复用模型扩展企业微信和钉钉。

最终目标是交付可生产运行的企业身份生命周期系统：认证和目录共享完整外部标识；认证票据不可被固定、窃取或重放；目录预览绑定不可变快照；部门、用户及成员关系能够幂等同步；多实例 worker 能安全接管；三个平台和三个数据库方言都通过真实验收。

本轮继续保持非目标：不增加 SCIM、LDAP、AD、HRIS、ISV 套件授权、双向写回、下游账号供给，也不长期保存上游用户令牌。

## 2. 阶段 01：认证安全、统一 principal 与连接管理

- OIDC session 使用浏览器绑定 Cookie；`init-session` 只发一次性登录票据，由登录浏览器原子兑换。
- 外部 OAuth state 绑定连接、OIDC session、模式、用户、可信 opener origin 和浏览器事务，并原子消费。
- 待绑定资料只保存在 HttpOnly continuation Cookie 对应的服务端记录中；客户端仅获得一次性 CSRF。
- 二维码拆分 `scanHandle`、`pollToken`、`confirmToken`，任何状态接口都不返回 JWT 或绑定票据。
- 所有登录方式签发会话前统一执行 `CanLogin()`。
- 认证和目录共用 connection 范围的 principal resolver；歧义写入 `external_principal_conflicts`，由管理员审批。
- 连接更新改为字段级合并，补齐 provider schema、capability 健康检查和凭据 keyring。
- 删除旧 social/identity 运行时代码；迁移遇到任何不可完整复制的别名时必须中止。

阶段门槛：会话固定、恶意 opener、XSS、state mix-up、并发回调、二维码旁观者、爆破、未激活用户、登录/同步先后顺序和离职封禁测试全部通过。

## 3. 阶段 02：飞书真实闭环与目录一致性

- 按 Contact v3 从配置根遍历子部门并分页读取直属用户，合并多部门关系。
- 每次运行保存完整性、scope hash 和不可变 `directory_sync_snapshot_objects`，审批与应用必须读取同一快照。
- 同一连接只允许一个可审批 run；显式 supersede 后旧 run 才进入 `superseded`。
- `directory_operation_leases` 串行化连接同步和组织根分配，并支持续租、过期接管与 owner 校验。
- 部门按拓扑创建/移动/软删除；成员关系生成撤销差异并重算主部门、负责人。
- scope、字段映射和字段所有权均实际生效；范围变化产生的破坏性差异强制审批。
- 拒绝重叠组织根和重复联系方式；补齐活跃用户唯一约束、change 重试和服务端分页。

阶段门槛：真实飞书完成诊断、预览、审批、二次幂等、多部门、移动、撤销、主部门切换、负责人、冲突、权限缩小、离职和组织根隔离。

## 4. 阶段 03：队列、自动化和治理

- claim 严格遵守最大尝试次数；到达上限的过期租约原子转入死信，管理员重试显式清零。
- 停机按 scheduler、claim、排空、取消顺序执行，最终状态使用独立 context 落库。
- worker 和应用流程全程传播 context，并在每个 change 事务前复查 job/operation lease。
- 连接或策略禁用、能力移除时取消待执行任务；入队、领取和执行前重复校验。
- 正确处理 `Retry-After` 与错误分类；同步中心展示 superseded、重试、冲突、健康、租约和重放结果。
- 所有回调 URL 由后端根据公开 API 基址生成。

阶段门槛：多实例抢占、九次崩溃、租约丢失、停机、重复/乱序事件、连接禁用、死信重放和每日 reconciliation 仅产生一次一致效果。

## 5. 阶段 04：企微、钉钉与生产验收

- 企微、钉钉输出与飞书相同的完整性、scope、字段权限和稳定标识语义。
- 修复三平台分页、限流、诊断和事件重读，并用统一 fixture 校验标准化结果。
- 三个平台分别在真实测试企业完成诊断、全量、审批、定时、事件、离职、死信和重放。
- 未完成真实联调的平台保持 directory/events capability 关闭；阶段 04 不标记完成。

## 6. 数据库与接口变更

新增 `external_principal_conflicts`、`identity_connection_health_checks`、`directory_sync_snapshot_objects`、`directory_operation_leases`。`directory_sync_runs` 增加 `scope_hash`、`is_complete`、`superseded_at`；`directory_sync_changes` 增加 `snapshot_object_id`、`attempt_count`、`last_attempt_at`。

API 采用硬切：绑定/注册使用 continuation Cookie；二维码使用三类票据；OIDC 初始化使用一次性 login ticket；连接更新为字段合并；诊断返回 capability 结果；同步运行支持 `supersede`；新增 change retry 和 principal conflict API；回调 URL 从后端 DTO 返回。

## 7. 测试、迁移与发布门槛

- 数据库：SQLite、MySQL 8.4、PostgreSQL 16 覆盖空库、存量硬切、冲突阻断、坏密钥、唯一约束和备份恢复。
- 代码：`go test ./...`、关键服务 race、`go vet ./...`、管理端 typecheck/build/lint、OIDC 前端 build 全部通过。
- 平台：带 `sandbox` build tag 的真实测试企业结果只记录 request ID 和结论，不提交凭据或通讯录数据。
- 发布：备份 → 暂停入口 → 预检/迁移 → keyring 验证 → 三端同步部署 → 冒烟 → 按连接恢复 capability。
- 回滚依赖数据库备份、旧程序和旧客户端；新 schema 与安全票据协议不提供向后兼容。

## 8. 当前执行状态

| 阶段 | 状态 | 关闭条件 |
| --- | --- | --- |
| 01 | 代码整改完成，生产验收中 | 真实 Redis/浏览器安全对抗与 MySQL/PostgreSQL 硬切演练通过 |
| 02 | 代码整改完成，真实飞书待验收 | 飞书测试企业完整目录闭环通过 |
| 03 | 代码整改完成，真实多实例待验收 | MySQL/PostgreSQL 多实例、连续崩溃和停机演练通过 |
| 04 | 契约测试完成，真实联调阻塞 | 三平台 `sandbox` 验收全部通过并记录 request ID |

已新增两类硬门槛测试：`-tags=integration` 固定运行 `mysql:8.4`、`postgres:16`；`-tags=sandbox` 使用三家测试企业环境变量运行真实目录验收。2026-07-20 本机复验时前者因 Docker 守护进程未启动、后者因凭据未注入而失败，因此没有把任一阶段恢复为“已完成”。
