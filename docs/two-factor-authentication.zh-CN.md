# 双因素认证（2FA）

用户在个人中心的「安全设置 → 双因素认证」中自行开启验证器（TOTP）、短信或邮箱。三种方式独立绑定；首次成功绑定会生成 10 组一次性恢复码。恢复码只展示一次，请保存到安全位置。

## 登录与恢复

个人中心、OIDC、管理后台统一登录、企业身份源、社交登录、小程序及扫码登录均执行同一认证规则：完成第一步验证后，已开启 2FA 的用户必须完成二次验证，才会创建登录会话和签发 Cookie、令牌或授权码。外部身份绑定到已有本地账户的操作也延后至二次验证通过。

短信／邮箱免密登录会排除第一步使用的同一渠道。可以使用另一已绑定方式或恢复码；没有可用方式时，改用密码登录，或由管理员核验身份后重置。已完成 2FA 的有效会话可继续用于 SSO，无需每个应用重复验证。机器 `client_credentials` 授权不受影响。

更换联系方式、移除验证方式、关闭 2FA、重新生成恢复码，都需要重新验证已有方式或恢复码。首次开启需要当前密码；无密码用户须在通过原应用重新登录后的 5 分钟内操作。移除最后一种方式需要明确确认关闭 2FA。

配置变更会更新配置版本、撤销旧登录会话和关联应用令牌，并令未完成挑战失效。当前浏览器获得新的个人会话；同一会话上的有效管理员访问也随之轮换。其他浏览器需要重新登录。管理员或目录同步更改联系方式不会自动将新地址绑定为 2FA；不再匹配的旧渠道将不可用，账户仍要求二次验证。

管理员可在用户列表查看 2FA 状态并执行「重置 2FA」。此操作需要重新确认管理员身份，并填写身份核验及重置原因。重置会关闭目标账户的 2FA、清除全部因子及恢复码、撤销全部会话，用户可以重新登录并自行开启。管理员对自己的设置使用个人中心的关闭流程。

## 部署配置

- TOTP 复用 `IDENTITY_CREDENTIALS_KEY`，或 `IDENTITY_CREDENTIALS_KEYS` 与 `IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION`。密钥为 Base64 编码的 32 字节随机值；已有部署应继续使用原有密钥，轮换时保留旧版本以解密现存凭据。所有节点使用相同的密钥环。
- 短信和邮箱复用管理后台「推送设置」的服务凭据、渠道及模板变量绑定，不需要单独消息服务。用户联系方式必须已验证，绑定时还需接收验证码确认。
- Redis 必须可用。登录挑战和操作凭据不降级为本地内存，服务不可用时不会绕过 2FA。
- TOTP 使用 RFC 6238，SHA-1、6 位数字、30 秒周期，接受前后各一个时间窗口；已成功使用的时间步不能再次使用。请保持各节点时间同步。
- 挑战有效期为 5 分钟，每个挑战最多 5 次错误尝试；账号每 15 分钟最多 10 次，IP 每 15 分钟最多 50 次。成功验证释放账号/IP 的本次尝试额度。短信／邮箱重发间隔为同一用户、同一渠道 60 秒。
- 密钥、验证码、恢复码及操作凭据不得写入业务日志。TOTP 密钥以 AES-256-GCM 加密；恢复码使用 128 位随机值并仅保存 SHA-256 哈希；短期验证码以挑战专属哈希保存在 Redis，通过原子比较删除消费。

迁移版本为 `20261008000000`，覆盖 MySQL、PostgreSQL、SQLite。升级前备份数据库与凭据密钥，停止旧节点后统一部署后端、两个前端及内嵌资源。存量用户默认未开启 2FA。该迁移拒绝自动 Down，避免删除已绑定凭据、审计记录并静默取消认证要求；回退需协调恢复应用和数据库备份。

## 接口约定

所有写接口使用 `application/json`。挑战接口必须携带发起浏览器的 `iam_2fa_binding` HttpOnly Cookie，OIDC 挑战还要求对应授权事务的浏览器 Cookie。原生客户端也必须保存并发送 Cookie。挑战 ID 不是登录凭据。

登录接口需要二次验证时，仍使用现有响应外壳 `{code: 0, data: ...}`，其中 `data` 包含：

```json
{
  "requiresTwoFactor": true,
  "twoFactorChallenge": {
    "id": "opaque-challenge-id",
    "purpose": "login",
    "expiresAt": 1791446700,
    "methods": [
      {"method": "totp", "enabled": true, "available": true},
      {"method": "email", "enabled": true, "available": true, "maskedReceiver": "u***r@example.com"},
      {"method": "recovery", "enabled": true, "available": true}
    ]
  }
}
```

该响应不包含登录令牌。客户端应先处理 `requiresTwoFactor`，不能把 HTTP 200 当作登录完成。OIDC `flow-status` 同时返回 `needs_two_factor` 和对应挑战，刷新页面后可继续。

| 方法与路径 | 请求与结果 |
|---|---|
| `GET /api/2fa/challenges/:id` | 读取当前挑战及可用方式 |
| `POST /api/2fa/challenges/:id/send-code` | `{method: "phone"或"email"}`；服务端选择已绑定接收地址，返回 `retryAfter` |
| `POST /api/2fa/challenges/:id/verify` | `{method, code}`；登录挑战返回原入口的登录完成响应，OIDC 返回原 `LoginResponse` |
| `POST /api/2fa/challenges/:id/cancel` | `{}`；撤销挑战 |
| `GET /api/user/2fa` | 返回 `enabled`、`hasPassword`、各方式启用及可用状态、剩余恢复码数量 |
| `POST /api/user/2fa/reauthenticate` | `{password?}`；未开启者验证第一步凭据，已开启者返回 `purpose: "manage"` 挑战 |
| `POST /api/user/2fa/enroll` | `{method, operationToken}`；返回 `purpose: "enroll"` 挑战，TOTP 还返回一次性的 `secret`、`otpAuthUri`、`qrCode` |
| `POST /api/user/2fa/methods/:method/remove` | `{operationToken, confirmDisable?}`；移除最后一种方式必须传 `confirmDisable: true` |
| `POST /api/user/2fa/disable` | `{operationToken, confirmDisable: true}` |
| `POST /api/user/2fa/recovery-codes/regenerate` | `{operationToken}`；返回新恢复码，旧码全部失效 |
| `POST /api/admin/auth/2fa/reauthenticate` | `{password?}`；验证管理员自己的身份，签发管理员操作凭据 |
| `POST /api/admin/users/:id/2fa/reset` | `{operationToken, reason}`；超级管理员权限，原因 3–500 字 |

方式枚举为 `totp`、`phone`、`email`、`recovery`，其中恢复码不可作为独立绑定方式。管理挑战验证成功后返回一次性、有效期 5 分钟的 `operationToken`，绑定用户、会话、配置版本和用户／管理员用途。绑定挑战验证成功后才保存因子，并返回 `twoFactorUpdated: true`；首次开启还返回 `recoveryCodes`。任何配置变更后，前端应丢弃旧应用 access token 并使用新 Cookie 会话。

已有 `PUT /api/user/email`、`PUT /api/user/phone` 在用户已开启 2FA 时新增必需的 `operationToken`，并继续要求发往新地址的验证码。授权流程的补充资料接口不允许替换已绑定的 2FA 联系方式，应通过个人中心完成安全改绑。

## 验证

```sh
go test ./...
go test -race ./app/service ./app/controller -run TestTwoFactor -count=1
go test -tags=integration ./migrations -run TestMigrationDialects -count=1
pnpm --dir frontend-oidc test
pnpm --dir frontend-oidc build
pnpm --dir admin-web test:unit
pnpm --dir admin-web --filter @vben/web-antd exec vue-tsc --noEmit --skipLibCheck
pnpm --dir admin-web build:antd
go build ./...
```

消息发送测试使用注入的模拟发送器和隔离 Redis，不发送真实短信或邮件。`IAM_TWO_FACTOR_BROWSER_TEST=1 go test ./app/controller -run TestTwoFactorHTTP -count=1 -v` 可启动仅用于本地浏览器验证的隔离测试服务，日志给出随机监听地址；服务使用测试账户和内存数据库，最多运行 5 分钟，`POST /__test/stop` 可提前结束。
