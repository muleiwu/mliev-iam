# OIDC 角色与 Scope 授权

IAM 支持按应用配置用户与机器的 scope 授予策略，并将应用相关角色输出到 Access Token、ID Token 或 UserInfo。Audience 仍为应用的客户端 UUID。本功能使用现有应用 `protocolConfig` 和令牌 `Attributes` JSON 字段，无需数据库表迁移。

## 配置入口

在「应用管理 → 创建/编辑 OIDC 应用 → 授权范围与角色」中配置。应用详情显示同一份只读配置。原有应用未开启策略时继续使用应用 scope 允许列表；新建应用的管理界面默认开启策略，标准身份 scope 允许应用内用户，新业务 scope 默认不授予任何用户或机器。

应用访问范围与 scope 规则分别生效：用户首先必须能访问应用，再满足请求的每一个 scope 规则。指定人员、部门和角色之间为“任一条件匹配”；部门可包含子部门。限制模式下所有条件为空代表拒绝用户访问。申请任一无权 scope 会拒绝整次授权，不静默缩减。

通过人员或部门条件获得 scope 不会自动赋予角色。角色来自已启用、已审批、未过期的全局角色分配；非全局、部门/组织限定的分配不参与外部角色输出或 scope 的角色条件。角色层级不自动继承。

## 管理 API 配置

沿用应用创建和更新 API 的 `protocolConfig`，增加三个字段：

- `scopeAuthorization`：`enabled` 开关与按 scope 名索引的 `rules`。每条包含授权说明 `description`、用户范围 `userAccess`、机器授权开关 `allowClientCredentials`。
- `roleClaims`：`enabled`、`claimName`（`roles` 或 `groups`）、输出角色白名单 `roleIds`、输出位置 `targets`（`access_token`、`id_token`、`userinfo`）。省略名称及位置时分别默认 `roles` 和 `access_token`；默认不开启。
- `clientRoleIds`：单独分配给机密客户端的机器角色。

下面是 Pagnode 示例中的相关配置。`42` 代表事先创建的 `pagnode-admin` 角色 ID，需替换为实际值。更新既有应用时可仅提交需要修改的 `protocolConfig` 顶层字段；未提交字段会保留。提交 `scopeAuthorization` 或 `roleClaims` 时应提供该对象的完整配置。

```json
{
  "scopes": ["openid", "profile", "pagnode:admin"],
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
      },
      "pagnode:admin": {
        "description": "管理 Pagnode 控制面",
        "userAccess": {
          "isPublic": false,
          "userIds": [],
          "departmentIds": [],
          "roleIds": [42],
          "includeSubDepartments": false
        },
        "allowClientCredentials": false
      }
    }
  },
  "clientRoleIds": []
}
```

开启策略后，`scopes` 中的每个范围都必须有规则；规则不能引用允许列表之外的 scope。`userAccess.isPublic` 必须明确设置，引用的人员、部门、角色必须存在。角色输出只包含主体实际拥有且在输出白名单中的角色代码，去重排序；开启输出但未匹配角色时返回空数组。

`groups` 是兼容性字段映射，其值仍为所选角色代码字符串数组。它不代表新建了组织用户组。首期不支持 `realm_access.roles` 等嵌套路径、任意 Claim 或脚本映射。

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
2. 将 `pagnode:admin` 规则的 `allowClientCredentials` 设为 `true`。
3. 如资源端还要求角色，将角色 `42` 加入 `clientRoleIds`，并保留在角色输出白名单中。
4. 向令牌端点显式请求 `grant_type=client_credentials&scope=pagnode:admin`，使用该客户端自己的凭据。

机器不继承创建者角色，也不使用用户范围规则。启用策略时省略 scope 会失败；机器令牌没有 ID Token、Refresh Token 或 UserInfo。

## Pagnode 接入

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

角色与部门关系、应用规则变更会在下次签发、授权码兑换和刷新时重新检查。刷新可以显式缩小原 scope，但不能扩大；仍申请已失去权限的范围会失败，失败事务不会消费原刷新令牌。旧 JWT 是签发时的快照；Pagnode 离线验签时，它通常可使用到到期。管理 API 建议设置 300 秒有效期。

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
