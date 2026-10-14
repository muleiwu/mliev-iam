# OIDC 角色与 Scope 授权

IAM 支持全局自定义权限授权，以及按应用配置标准 scope、角色和部门输出。Audience 仍为应用的客户端 UUID。自定义权限使用独立数据库表，配置及升级步骤见 [全局自定义权限与授权有效期](custom-permissions.zh-CN.md)。角色与部门输出仍使用现有 JSON 字段。

## 配置入口

在「应用管理 → 创建/编辑 OIDC 应用 → 授权范围与角色」中配置。应用详情显示同一份只读配置。该入口仅编辑标准 scope 策略，并选择全局自定义权限。自定义权限定义和授权移至「访问控制 → 自定义权限」；旧应用内自定义配置不再生效，需要重建后重新选择。标准 scope 保留原兼容模式，新业务权限始终执行全局授权校验。

应用访问范围与 scope 规则分别生效：用户首先必须能访问应用，再满足请求的每一个标准 scope 规则或全局自定义权限授权。指定人员、部门和角色之间为“任一条件匹配”；部门可包含子部门。限制模式下所有条件为空代表拒绝用户访问。申请任一无权 scope 会拒绝整次授权，不静默缩减。

通过人员或部门条件获得 scope 不会自动赋予角色。角色来自已启用、已审批、未过期的全局角色分配；非全局、部门/组织限定的分配不参与外部角色输出或 scope 的角色条件。角色层级不自动继承。

## 管理 API 配置

沿用应用创建和更新 API 的 `protocolConfig`，支持以下字段：

- `customPermissionIds`：应用接入的全局自定义权限 ID。
- `scopeAuthorization`：仅用于标准 scope 的 `enabled` 开关及 `rules`；不影响全局自定义权限校验。每条包含说明、用户范围和机器开关。
- `roleClaims`：`enabled`、`claimName`（`roles` 或 `groups`）、输出角色白名单 `roleIds`、输出位置 `targets`（`access_token`、`id_token`、`userinfo`）。省略名称及位置时分别默认 `roles` 和 `access_token`；默认不开启。
- `clientRoleIds`：单独分配给机密客户端的机器角色。
- `departmentClaims`：用户部门输出开关、部门白名单 `departmentIds` 和输出位置 `targets`，见下节。

下面是 Pagnode 示例中的相关配置。`42` 是角色 ID，`7` 是事先在全局目录创建的 `pagnode:admin` 权限 ID；请先在全局授权管理中授予角色 42 此权限，再替换为实际 ID。更新既有应用时可仅提交需要修改的 `protocolConfig` 顶层字段；未提交字段会保留。提交 `scopeAuthorization` 或 `roleClaims` 时应提供该对象的完整配置。

```json
{
  "scopes": ["openid", "profile"],
  "customPermissionIds": [7],
  "tokenLifetime": 300,
  "roleClaims": {
    "enabled": true,
    "claimName": "roles",
    "roleIds": [42],
    "targets": ["access_token"]
  },
  "scopeAuthorization": {
    "enabled": true,
    "rules": {
      "openid": {
        "description": "验证您的身份",
        "userAccess": {"isPublic": true},
        "allowClientCredentials": false
      },
      "profile": {
        "description": "访问您的基本资料",
        "userAccess": {"isPublic": true},
        "allowClientCredentials": false
      }
    }
  },
  "clientRoleIds": []
}
```

开启标准范围策略后，`scopes` 中的每个标准范围都必须有规则；规则不能引用允许列表之外的标准 scope。自定义权限始终由 `customPermissionIds` 解析和校验。`userAccess.isPublic` 必须明确设置，引用的人员、部门、角色必须存在。角色输出只包含主体实际拥有且在输出白名单中的角色代码，去重排序；开启输出但未匹配角色时返回空数组。

`groups` 是兼容性字段映射，其值仍为所选角色代码字符串数组。它不代表新建了组织用户组。首期不支持 `realm_access.roles` 等嵌套路径、任意 Claim 或脚本映射。

## 部门字段输出

在应用的「授权范围与角色 → 部门输出」中开启。字段固定为 `departments`，内容是部门编码字符串数组。部门配置与角色独立，默认关闭；开启后省略 `targets` 默认仅输出到 Access Token。开启状态下显式传入空输出位置会被拒绝。

```json
{
  "departmentClaims": {
    "enabled": true,
    "departmentIds": [10, 12],
    "targets": ["access_token", "id_token", "userinfo"]
  }
}
```

`10`、`12` 应替换为实际部门 ID。例如用户直接属于 `sre`，上级为 `platform`，应用同时允许这两个部门，则输出：

```json
{
  "departments": ["platform", "sre"]
}
```

部门范围按以下规则计算：

- 从用户已激活、未离开、未删除的部门成员关系开始；直属部门须处于启用状态。
- 按 `ParentID` 收集直属部门及上级，再与应用白名单取交集，去重并按编码排序。部门负责人或角色上的部门限制不等于成员关系。
- 遇到禁用、冻结、删除或不存在的节点，停止这条链，不跨过失效节点继续追溯。已经收集的有效下级部门保留。
- 父子节点独立选择。白名单只包含 `platform` 时，仅输出 `["platform"]`；不会因为选择父部门就公开所有子部门。
- 白名单为空或没有匹配部门时，选定输出位置返回 `departments: []`；关闭或未选择该位置时省略字段。
- 层级出现循环或查询失败时，不签发不完整声明，返回 `server_error`；失败的刷新事务不会消费原刷新令牌。

机器的 `client_credentials` 令牌完全省略该字段，不继承应用创建者的部门，也没有单独的机器部门配置。无需申请新的 `departments` scope；现有 UserInfo 访问仍要求 `openid`。字段输出不改变原有应用、角色和 scope 的授权规则。

同次签发共用一份部门结果。UserInfo 在令牌仍有效时返回签发快照；旧令牌没有部门快照时，不按当前配置补算。部门加入、离开、移动、禁用、编码修改或白名单调整影响后续签发和刷新，旧 JWT 的字段值不会被改写。刷新及注销沿用原有令牌撤销机制。

仅当该应用将部门输出到 ID Token 或 UserInfo 时，Discovery 的 `claims_supported` 才声明 `departments`。此能力使用现有应用 JSON 配置和令牌属性存储，无需数据库迁移；旧管理客户端未提交 `departmentClaims` 时保留原值。

## 用户和机器调用

用户使用授权码流程申请 `scope=openid profile pagnode:admin`。IAM 检查应用访问权限和各 scope 的主体规则，在签发前再次检查。通过后，Access Token 中包含：

```json
{
  "aud": "<客户端 UUID>",
  "scope": "openid profile pagnode:admin",
  "roles": ["pagnode-admin"]
}
```

机器调用须另外完成以下配置：

1. 使用机密客户端，允许 `client_credentials` grant。
2. 在全局授权管理中为该应用创建 `pagnode:admin` 的机器客户端授权，并设置有效期。
3. 如资源端还要求角色，将角色 `42` 加入 `clientRoleIds`，并保留在角色输出白名单中。
4. 向令牌端点显式请求 `grant_type=client_credentials&scope=pagnode:admin`，使用该客户端自己的凭据。

机器不继承创建者角色，也不使用用户范围规则。接入自定义权限或启用标准策略时省略 scope 会失败；机器令牌没有 ID Token、Refresh Token 或 UserInfo。

## Pagnode 接入

浏览器跨域要求和发布后验证见 [OIDC CORS 接入与验收](oidc-cors.zh-CN.md)。

使用 RS256 签名密钥，以 HTTPS 暴露 IAM，反向代理正确设置外部协议和主机名。配置示例中的占位符均需替换：

```dotenv
PAGNODE_OIDC_ISSUER=https://iam.example.com/oidc/<客户端UUID>
PAGNODE_OIDC_AUDIENCE=<客户端UUID>
PAGNODE_OIDC_ROLES_CLAIM=roles
PAGNODE_OIDC_ADMIN_ROLES=pagnode-admin
PAGNODE_OIDC_REQUIRED_SCOPE=pagnode:admin
PAGNODE_ALLOW_INSECURE_OIDC=false
```

Pagnode 用该 issuer 的 Discovery/JWKS 验证 Access Token 的签名、issuer、audience、有效期，再同时检查角色与 scope。ID Token 表示登录身份，不用作管理 API 的访问令牌。HS256 对称密钥不会出现在公开 JWKS 中。

## 撤权、错误和上线

角色与部门关系、应用标准规则和全局自定义授权会在签发、授权码兑换和刷新时重新检查；访问令牌期限同时受所需自定义授权截止时间约束。刷新可以显式缩小原 scope，但不能扩大；仍申请已失去权限的范围会失败，失败事务不会消费原刷新令牌。旧 JWT 是签发时的快照；Pagnode 离线验签时，它通常可使用到到期。管理 API 建议设置 300 秒有效期。

UserInfo 的角色来自该 Access Token 签发时保存的快照，后续新增角色不会扩充旧令牌。已签发令牌的输出字段和位置也保持原配置，下一次签发才应用配置变化。

| 场景 | 错误 |
| --- | --- |
| 授权请求中未知或格式错误的 scope | `invalid_scope` |
| 用户无权申请所请求的 scope | `access_denied` |
| 机器未获准申请的 scope，或启用策略后省略 scope | `invalid_scope` |
| 兑换授权码或刷新时授权已失效 | `invalid_grant` |
| 刷新请求扩大原 scope | `invalid_scope` |

Access Token 的 JWT 头改为 `typ: at+jwt`，ID Token 保持 `typ: JWT`；接入方不应将 Access Token 的类型固定为 `JWT`。先升级所有签发节点，再为应用开启策略，避免旧节点忽略授权规则；启用后不要回退到不理解这些配置的版本。

规范依据：[OIDC Additional Claims](https://openid.net/specs/openid-connect-core-1_0.html#AdditionalClaims)、[OAuth 2.0 Scope](https://www.rfc-editor.org/rfc/rfc6749.html#section-3.3)、[RFC 9068 JWT Access Token](https://www.rfc-editor.org/rfc/rfc9068.html)。

本地测试覆盖模拟 Pagnode 的 RS256/JWKS、issuer、audience、角色与 scope 校验；实际 Pagnode 部署联调需另外验证。
