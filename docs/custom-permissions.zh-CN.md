# 全局自定义权限与授权有效期

在「访问控制 → 自定义权限」维护业务权限，在「授权管理」为用户、部门、角色、全体用户或机器客户端创建授权。权限和授权全局共用；OIDC 应用通过 `protocolConfig.customPermissionIds` 显式接入权限。

应用接入权限并不直接授权：用户必须同时满足应用访问范围和每一个请求权限的全局授权。机器必须使用机密客户端、开启 `client_credentials`，并且有以该应用为对象的机器授权。机器不会继承用户、角色或“全体用户”授权。

## 配置示例

管理接口均要求管理员认证，响应沿用 `{code, message, data}`。

1. `POST /api/admin/custom-permissions` 创建权限：

```json
{"code":"orders:read","name":"查询订单","description":"查看订单列表及详情","enabled":true}
```

code 全局唯一、大小写敏感，遵循现有 scope 的 ASCII 格式规则，最长 100 字符；不能使用 `openid`、`profile`、`email`、`phone`、`address`、`offline_access`。code 创建后不可更改，删除后也不可复用。名称必填且最长 100 字符，说明最长 2000 字符。

2. `POST /api/admin/custom-permission-grants` 创建授权（示例中的 ID 替换为实际值）：

```json
{
  "permissionId":7,
  "subjectType":"user",
  "subjectId":42,
  "includeSubDepartments":false,
  "startsAt":"2026-10-14T09:00:00+08:00",
  "endsAt":"2026-10-21T18:00:00+08:00",
  "remark":"阶段性订单核查"
}
```

`subjectType` 为 `user`、`department`、`role`、`everyone` 或 `client`。`everyone` 的 `subjectId` 必须为 0，其余必须为存在且可用的对象 ID。只有部门授权可设置 `includeSubDepartments`。角色授权仍只匹配有效、已审批的全局角色分配。

不限时授权将 `startsAt`、`endsAt` 同时设为 `null`。限时授权要求两者都有值且开始早于结束，接口时间必须带时区；服务统一转换为 UTC，页面显示浏览器本地时区。判断区间为 `[startsAt, endsAt)`，无需定时任务。

3. 创建或更新应用的 `protocolConfig`：

```json
{
  "scopes":["openid","profile"],
  "customPermissionIds":[7]
}
```

保留应用的其他必要配置。`scopes`、`scopeAuthorization` 管理标准身份范围；自定义权限由 ID 解析成实际 code。客户端的 OAuth 请求和 Access Token 中仍使用 `scope=openid profile orders:read`，不使用数字 ID。关闭 `scopeAuthorization.enabled` 不会关闭全局自定义权限校验。

## 管理接口

| 接口 | 用途 |
| --- | --- |
| `GET /api/admin/custom-permissions` | 分页查询；支持 `keyword`（code/名称）、`enabled` |
| `GET /api/admin/custom-permissions/:id` | 详情及关联应用 |
| `POST /api/admin/custom-permissions` | 新增权限 |
| `PUT /api/admin/custom-permissions/:id` | 更新名称、说明、启用状态，不能修改 code |
| `DELETE /api/admin/custom-permissions/:id` | 删除；存在应用引用或未撤销授权记录时拒绝 |
| `GET /api/admin/custom-permission-grants` | 查询授权；支持 `keyword`、`permissionId`、`subjectType`、`subjectId`、`state` |
| `POST /api/admin/custom-permission-grants` | 新增授权 |
| `PUT /api/admin/custom-permission-grants/:id` | 更新完整授权配置，包括有效期 |
| `DELETE /api/admin/custom-permission-grants/:id` | 撤销授权，保留操作人及历史记录 |

分页参数 `page`、`pageSize`（上限 100），返回 `{items,total,page,size}`。有效期状态 `state` 为 `pending`、`active`、`expired`；状态按时间实时计算，不表示权限或对象一定仍处于启用状态。授权列表同时提供 `subjectAvailable` 和 `permissionEnabled`。名称说明、有效期和备注更新时应提交完整表单。

## 生效与令牌

- 同一权限存在多条匹配授权时取并集：当前有效授权中取最晚的截止时间，永久授权视为无限；通过角色获得权限时还受用户角色分配到期时间约束。
- 申请多个权限时，每项均须获权，访问令牌截止时间取全部所需权限截止时间与应用令牌有效期的最小值。JWT `exp` 与数据库过期时间使用同一个秒级时刻。
- 签发不足一个完整有效秒的令牌会被拒绝，不会回退到默认一小时有效期。
- 授权请求、授权码兑换、刷新、SSO、隐式流程和机器调用均重新检查权限。刷新仍可缩小原 scope，不能扩权；失败不会消费原刷新令牌。
- Discovery 只列出应用接入且已启用的自定义权限；授权确认页展示全局名称、说明和原 code。
- 已签发 JWT 的到期时间不可追溯修改。提前撤销或缩短授权影响后续签发和刷新，旧令牌沿用既有撤销机制；离线验签客户端可继续使用至 JWT 到期。此次截止时间保证针对新签发访问令牌，ID Token 仍用于身份认证。

## 升级

启动迁移新增 `custom_permissions` 与 `custom_permission_grants`，覆盖 MySQL、PostgreSQL、SQLite。不会从旧应用配置导入权限或授权。

旧自定义 scope 和旧授权规则立即停止参与授权。管理员先重建全局权限和授权，再在应用选择对应权限；创建同名 code 不会自动恢复旧关联。应用页面显示重建提示，保存时清理旧自定义字段，保留标准 scope、角色和部门配置。

签发节点统一升级，避免旧节点使用旧授权规则。迁移拒绝直接回退旧版本；需要回退时恢复协调一致的应用和数据库备份。
