# 身份连接 03：自动同步、任务队列与治理

> 状态：已实施并通过后端与管理端验收（2026-07-20）
> 前置：[02 飞书手动同步](./IDENTITY_CONNECTIONS_02_FEISHU_MANUAL_SYNC.md)
> 后继：[04 企业微信与钉钉](./IDENTITY_CONNECTIONS_04_WECOM_DINGTALK.md)

## 1. 目标

把手动同步服务接入可靠的数据库任务队列、定时计划和飞书事件收件箱，提供失败恢复、死信重放、批量删除保护、会话撤销和统一同步中心。

## 2. 非目标

- 不引入 Redis 队列或外部调度中间件。
- 不信任事件载荷直接覆盖本地目录。
- 不实现企业微信、钉钉、SCIM 或出站同步。

## 3. 领域模型

任务类型：`full_sync`、`incremental_sync`、`reconcile`、`event_refresh`、`apply_changes`。

任务状态：`pending`、`leased`、`succeeded`、`retry_wait`、`dead_letter`、`cancelled`。

计划类型：`cron`、`daily_reconcile`；事件状态：`received`、`verified`、`queued`、`processed`、`failed`、`dead_letter`。

`EventAdapter` 只负责验签、解密、事件标识与变更对象提示：

```go
type EventAdapter interface {
    VerifyAndParse(ctx context.Context, headers http.Header, body []byte) (*DirectoryEvent, error)
}
```

## 4. 数据库设计

### 4.1 `sync_jobs`

- `connection_id`、`run_id`、`job_type`、`status`、`payload_json`。
- `available_at`、`lease_owner`、`lease_expires_at`。
- `attempts`、`max_attempts=8`、`last_error`。
- `created_at`、`updated_at`、`finished_at`。

抢占使用条件更新：仅 `pending/retry_wait`、已到 `available_at` 且无有效租约的记录可转为 `leased`。SQLite 固定单 worker；MySQL/PostgreSQL 可配置多 worker/多实例。

### 4.2 `sync_schedules`

- `connection_id`、`schedule_type`、`cron_expression`、`timezone`、`enabled`。
- `next_run_at`、`last_enqueued_at`；同连接同计划类型唯一。

### 4.3 `external_event_inbox`

- `connection_id + provider_event_id` 唯一。
- `event_type`、`object_type`、`external_object_id`。
- 验签/处理状态、加密或脱敏后的原始载荷、错误和尝试次数。
- `received_at`、`processed_at`。

## 5. API

- `GET/POST /api/admin/sync-jobs`
- `POST /api/admin/sync-jobs/:id/retry|cancel`
- `GET/PUT /api/admin/identity-connections/:id/sync-schedules`
- `GET /api/admin/external-events`
- `POST /api/admin/external-events/:id/replay`
- `GET /api/admin/sync-center/summary`
- `POST /external-events/:identifier`

Webhook 在验签、幂等落库和入队成功后立即响应；实际读取与变更由 worker 完成。

## 6. 执行与治理

- worker 通过租约抢占，定期续租；进程退出停止续租，租约到期后其他实例接管。
- 重试采用指数退避并加入抖动，最多 8 次、间隔上限 6 小时，之后死信。
- 飞书事件只提供对象提示；worker 必须重新调用目录 API 读取当前状态，再复用 02 的差异服务。
- 每个启用目录连接默认每日执行一次 reconcile；管理员可增加 cron 全量同步。
- 自动应用低风险的新建、普通资料更新和成员关系变更。
- 账号合并、部门删除、组织根变更、用户最终停用必须审批。
- 单次删除/停用达到 10 人、管理用户的 5%，或涉及组织根时触发批量保护并禁止自动应用。
- principal 被明确停用时立即阻止该身份登录；批准停用本地用户后撤销 IAM 会话和应用访问。若仍有其他 active principal，则不禁用本地用户。

## 7. 管理界面

- 新增“同步中心”：概览、任务、运行历史、待审批差异、事件、死信。
- 概览突出连接健康、正在运行、待审批和死信，不使用装饰性指标。
- 任务详情展示租约、尝试次数、下一次重试、关联 run 和可执行恢复动作。
- 所有批量危险操作显示准确影响范围，要求二次确认并保留审计。

## 8. 安全与可观测性

- 飞书事件凭据使用 `CredentialPurpose=events`。
- 原始事件中的手机号、邮箱等敏感字段在持久化前脱敏或整体加密。
- 日志包含 connection/run/job/event ID，不包含凭据和完整用户档案。
- 暴露任务积压、成功率、处理时延、API 限流、死信数和最近对账时间。

## 9. 迁移与发布

- worker 默认启用但并发为 1；生产可通过配置增加 MySQL/PostgreSQL worker 数。
- 发布时先迁移表，再启动 worker；计划扫描器必须在迁移完成后运行。
- 新事件端点配置完成前不启用连接的 `events` capability。

## 10. 测试与验收

- 覆盖并发抢占、租约续期/过期接管、进程重启、重试抖动、死信和人工重放。
- 同一 provider event 重复投递只产生一个 inbox 和一个有效任务。
- 乱序事件最终以重新读取到的当前对象为准。
- 每日对账能修复漏事件；不能因临时权限范围缩小自动删除用户。
- 批量保护阈值、其他有效身份、会话撤销均有测试。
- SQLite 不发生并发 worker 写冲突。

实施验收记录：

- [x] `sync_jobs`、`sync_schedules`、`external_event_inbox` 和用户会话撤销时间已提供三种数据库迁移。
- [x] 条件更新抢占、租约续期/过期接管、SQLite 单 worker、优雅停机、8 次重试、6 小时上限与死信重放已实现。
- [x] 飞书事件完成验签、解密、加密落库、幂等入队；worker 只使用事件作为提示并复用全量快照/差异服务。
- [x] 定时全量同步、每日 reconciliation 和进程重启后的可执行任务恢复已接入应用生命周期。
- [x] 仅低风险新建/资料更新可自动批准；匹配、停用、删除采用比阈值规则更严格的逐项审批保护。
- [x] 最后一个有效企业身份停用时撤销 OIDC token 并使既有 IAM JWT 失效；仍有其他有效身份时只封禁对应 principal。
- [x] 同步中心提供健康摘要、任务、事件、重试/取消、死信重放和连接详情中的计划配置。
- [x] 并发抢占、租约接管、重试/死信、事件幂等、会话撤销与其他有效身份均有自动测试。

## 11. 阶段依赖

04 的平台事件适配器只接入本阶段 `EventAdapter`、inbox 和 job，不允许自建平台专用队列。
