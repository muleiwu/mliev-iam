# 身份连接 02：飞书手动与半自动目录同步

> 状态：代码整改完成，真实飞书测试企业验收中
> 前置：[01 统一连接](./IDENTITY_CONNECTIONS_01_UNIFICATION.md)
> 后继：[03 自动化与治理](./IDENTITY_CONNECTIONS_03_AUTOMATION_GOVERNANCE.md)
> 修复计划：[企业身份连接修复与生产验收计划](./IDENTITY_CONNECTIONS_REMEDIATION_PLAN.md)

## 1. 目标

在统一企业连接上实现飞书通讯录全量读取、标准化快照、差异预览和人工确认应用，完成“企业目录 → IAM 用户/部门/关系”的首个可用闭环。

## 2. 非目标

- 不实现定时调度、实时事件、后台 worker 或失败重放。
- 不实现企业微信、钉钉、SCIM 或双向写回。
- 不因一次全量结果中缺失对象而自动删除或停用本地用户。

## 3. 领域模型

新增统一目录接口：

```go
type DirectoryAdapter interface {
    ValidateDirectoryConfig(ctx context.Context) (*DirectoryDiagnostic, error)
    ListOrgUnits(ctx context.Context, cursor string) (OrgUnitPage, error)
    ListUsers(ctx context.Context, cursor string) (DirectoryUserPage, error)
}
```

标准对象包含：

- `DirectoryUser`：稳定 ID、租户 ID、标识别名、姓名、邮箱、手机、工号、头像、状态和原始数据。
- `OrgUnit`：稳定 ID、父 ID、名称、排序、负责人 ID、状态和原始数据。
- `Membership`：用户 ID、部门 ID、是否主部门、是否负责人、职位和排序。

## 4. 数据库设计

### 4.1 `directory_sync_policies`

- `connection_id` 唯一，且引用 enterprise 连接。
- `root_department_id` 在启用策略中全局唯一，保证多源按组织根隔离。
- `sync_scope_json`、`field_mapping_json`、`field_ownership_json`。
- `match_policy`：`manual` 或 `create_unmatched`，默认 `manual`。
- `deprovision_policy`：首期固定 `block_identity_require_user_approval`。
- `enabled`、`created_at`、`updated_at`。

### 4.2 `external_org_units`

- `connection_id + external_id` 唯一。
- 可空 `department_id`、`external_parent_id`、名称、排序、状态、负责人外部 ID。
- `snapshot_version`、`last_seen_at`、`raw_data`。

### 4.3 `external_memberships`

- `(connection_id, principal_id, external_org_unit_id)` 唯一。
- 可空 `user_id`、`department_id`、`is_primary`、`is_leader`、`position`、`status`。
- `snapshot_version`、`last_seen_at`、`raw_data`。

### 4.4 `directory_sync_runs` 与 `directory_sync_changes`

Run 状态：`pending`、`fetching`、`preview_ready`、`applying`、`succeeded`、`failed`、`cancelled`、`superseded`。

Change 类型：`create`、`update`、`move`、`link`、`disable`、`delete`、`conflict`；对象类型为 `user`、`org_unit`、`membership`。Change 状态为 `pending`、`approved`、`rejected`、`applied`、`failed`。

每条 change 保存 before/after JSON、风险级别、匹配理由和错误；生成预览时不得改写本地用户、部门或关系。

## 5. API

- `GET/PUT /api/admin/identity-connections/:id/directory-policy`
- `POST /api/admin/identity-connections/:id/directory-diagnostic`
- `POST /api/admin/identity-connections/:id/sync-runs`
- `GET /api/admin/identity-connections/:id/sync-runs`
- `GET /api/admin/directory-sync-runs/:runId`
- `GET /api/admin/directory-sync-runs/:runId/changes`
- `POST /api/admin/directory-sync-runs/:runId/approve`
- `POST /api/admin/directory-sync-runs/:runId/apply`
- `POST /api/admin/directory-sync-changes/:changeId/approve|reject`

创建 run 在当前请求中执行拉取与差异计算；应用阶段按批次事务执行，并持续更新 run 统计。

## 6. 数据流与匹配

1. 检查连接能力、目录凭据、飞书权限和通讯录可见范围。
2. 获取 tenant access token，分页拉取部门、用户和关系。
3. 使用新的 snapshot version upsert 外部快照。
4. 按稳定标识匹配 principal；`tenant_key + user_id` 映射为 `directory_user_id`，保存 `open_id`、`union_id` 别名。
5. 已确认历史映射优先；唯一邮箱/手机号只生成 conflict/link 建议，必须审批。
6. 未匹配对象按策略生成本地创建建议或 conflict。
7. 比对外部快照与本地目录，生成 changes 并进入 `preview_ready`。
8. 管理员审批后才能 apply；应用必须幂等。

明确返回离职/禁用状态时立即把对应 principal 标记 `blocked`。仅 `last_seen_at` 未更新表示“本次不可见”，只能生成高风险待审批 change。

## 7. 管理界面

- 身份连接详情新增“目录同步”页签。
- 首屏显示连接身份、组织根、目录权限诊断和最后一次同步，而不是先展示协议字段。
- 配置区包含组织根、同步范围、字段映射、匹配策略和停用策略。
- “生成同步预览”与“应用已审批变更”使用不同层级按钮。
- 预览按新增、更新、移动、冲突、停用分类，支持逐项批准/拒绝和风险筛选。
- 空状态解释需要先配置目录凭据和组织根，并提供直接操作。

## 8. 安全策略

- 飞书目录凭据使用 `CredentialPurpose=directory`，不得复用或暴露用户登录 token。
- 敏感字段按最小权限申请；诊断结果明确区分令牌有效与字段权限完整性。
- 所有应用变更记录管理员、run、来源连接和匹配理由。
- 组织根隔离在服务层与数据库唯一约束双重执行。

## 9. 迁移与发布

- 新表按 MySQL/PostgreSQL/SQLite 分别提供 goose 迁移。
- 现有企业连接默认没有 `directory_sync` capability；管理员保存有效 policy 和目录凭据后才可启用。
- 不自动对现有飞书连接执行同步。

## 10. 测试与验收

- 飞书分页、限流、权限不足、空部门、循环/缺失父部门均有测试。
- 重复执行同一快照不产生重复用户、部门和关系。
- 覆盖多部门、主部门、负责人、职位、邮箱/手机冲突和未匹配用户。
- 两个连接不能绑定同一本地组织根，且不能跨根更新部门。
- 预览阶段本地用户、部门、关系零变化。
- 应用失败可重试，已成功 change 不重复执行。
- 明确离职封禁企业 principal，但本地用户停用保持待审批。

实施验收记录：

- [x] 飞书目录使用独立 directory 凭据，标准化稳定员工 ID、别名、部门、主部门、负责人、职位和状态。
- [x] 外部快照与差异计算在本地目录变更之前完成；测试确认预览阶段本地用户、部门和关系零变化。
- [x] 唯一邮箱/手机号只生成待确认 link；未匹配策略、明确离职封禁和快照缺失保护已实现。
- [x] 组织根排他、根内更新校验、审批、幂等应用和重复快照无重复结果测试通过。
- [x] 管理端目录同步页包含独立凭据、权限诊断、组织根、字段所有权、预览、历史和逐项审批；typecheck/build 通过。

## 11. 阶段依赖

03 的任务和事件只能调用本阶段公开的“生成预览”和“应用已批准变更”服务，不得复制同步逻辑。

## 12. 2026-07-20 修复复验

- [x] 飞书 Contact v3 从配置根读取根详情、遍历子部门并逐部门分页读取直属用户，按 `tenant_key:user_id` 去重合并多部门关系。
- [x] 快照记录遍历范围、分页完整性、字段权限和 scope hash；不完整 run 失败且不生成删除类差异。
- [x] `directory_sync_snapshot_objects` 保存不可变标准 JSON/hash；审批应用只读取 change 引用的快照，不读取后续 run 覆盖的映射表。
- [x] 同连接单一可审批 run、显式 supersede、`directory_operation_leases` 续租/过期接管/owner 校验均有回归测试。
- [x] 部门父子拓扑、move、来源所有权删除保护，membership 撤销、主部门/负责人重算和幂等应用已实现。
- [x] scope、fieldMapping、fieldOwnership 实际参与诊断和差异；本地拥有的邮箱/手机号不会触发上游权限阻断或无意义更新。
- [x] 重叠组织根、重复联系方式、活跃用户唯一约束、failed change retry 与服务端分页已实现。
- [ ] 真实飞书测试企业尚未注入 `FEISHU_SANDBOX_*` 凭据，诊断、完整预览、审批、二次幂等、权限缩小与离职链路未取得真实验收结论。
