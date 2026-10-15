# Passkey 与统一两步验证

用户在个人中心「安全设置 → Passkey」中添加、重命名和删除凭据。一个账户可以绑定多个设备或密码管理器。登录页的「使用 Passkey 登录」直接由浏览器选择账号，无需先填写用户名；个人中心、OIDC 统一登录和管理后台统一登录共用这套认证流程。

## 认证规则

- 绑定 Passkey 或 TOTP 不会自动开启两步验证。用户在安全设置中单独开启；系统要求至少存在一条使用两种不同方式的完整登录路径。
- 密码、Passkey、短信和邮箱可以作为第一步或第二步。已设置密码、已绑定有效 Passkey、已验证且推送服务可用的联系方式自动参与；第二步排除第一步同类方式。
- TOTP 和恢复码只用于第二步；企业身份源和社交登录继续作为第一步。恢复码不计入开启条件。
- Passkey 的指纹、面容或 PIN 解锁仍然只算一种认证方式。先使用 Passkey 后，不能使用另一个 Passkey 充当第二步。
- 关闭两步验证保留 Passkey、TOTP、密码和联系方式，同时作废恢复码；再次开启会生成 10 组新恢复码。
- 管理员重置两步验证会清除 TOTP 和恢复码并退出全部会话，保留 Passkey、密码和联系方式。管理员不能替用户添加或删除 Passkey。
- 自助移除凭据或外部身份不得使账户失去唯一登录方式；两步验证已开启时也不得破坏完整的两步登录路径。请先添加替代方式，或明确关闭两步验证。

## 部署

默认关闭，配置文件示例：

```yaml
passkey:
  enabled: true
  rp_id: "iam.example.com"
  rp_origins:
    - "https://iam.example.com"
  rp_display_name: "MLiev IAM"
```

RP ID 是固定域名，不包含协议、路径或端口。Origin 必须包含协议、域名及非默认端口，不包含路径；必须属于 RP ID。启用时配置非法会阻止应用启动，服务不会根据请求 Host 或转发头自动信任域名。

生产使用 HTTPS。开发可设置 `rp_id: localhost` 和 `rp_origins: ["http://localhost:8080"]`，需要另一个前端端口时显式加入对应 Origin。`localhost` 与 `127.0.0.1` 不可混用。已有凭据绑定原 RP ID，改变 RP ID 后需要重新绑定；跨站 iframe 内不执行 WebAuthn，用户转到顶层完整认证页继续原事务。

Redis 必须可用，各节点必须使用一致的 RP 配置。注册与认证事务均为 5 分钟、单次使用，并绑定浏览器、来源、用途、管理会话或 OIDC 事务。Passkey 请求开始接口按浏览器每分钟 30 次、IP 每分钟 60 次限制；第二步沿用现有挑战、账号和 IP 限流。

服务器只保存公钥凭据，永不接收私钥、指纹或面容数据。注册要求可发现凭据，注册和认证均要求用户验证，不请求设备证明。验证成功后更新计数器和备份状态，支持合法的零计数器；计数器异常时拒绝本次验证。

## API

沿用 `{code, message, data}` 外壳。所有写请求使用 `application/json`，浏览器需携带 Cookie；WebAuthn 接口还需实际页面的 `Origin`。所有事务都必须由开始请求的浏览器完成。

| 接口 | 请求／响应 |
|---|---|
| `GET /api/passkey/config` | `{enabled}` |
| `GET /api/user/passkeys` | `[{id, name, createdAt, lastUsedAt}]`，只返回当前用户的展示信息 |
| `POST /api/user/passkeys/registration/options` | `{name, operationToken}` → `{transactionId, publicKey, expiresAt}` |
| `POST /api/user/passkeys/registration/verify` | `{transactionId, credential}` → `{twoFactorUpdated: true}` |
| `PATCH /api/user/passkeys/:id` | `{name}`，名称为 1–100 字 |
| `DELETE /api/user/passkeys/:id` | `{operationToken}` → `{twoFactorUpdated: true}` |
| `POST /api/passkey/login/options` | `{sessionId?, rememberMe?}` → `{transactionId, publicKey, expiresAt}`；`sessionId` 为 OIDC 授权事务 |
| `POST /api/passkey/login/verify` | `{transactionId, credential}` → 原入口登录响应，或 `requiresTwoFactor` 挑战 |
| `POST /api/2fa/challenges/:id/passkey/options` | `{}` → `{transactionId, publicKey, expiresAt}` |
| `POST /api/2fa/challenges/:id/passkey/verify` | `{transactionId, credential}` → 原挑战的登录完成响应或 `operationToken` |
| `POST /api/user/2fa/enable` | `{operationToken}` → `{twoFactorUpdated: true, recoveryCodes}` |

前端把 `publicKey` 交给 `@simplewebauthn/browser` 的 `startRegistration` 或 `startAuthentication`，把返回值作为 `credential` 提交。不能把 HTTP 200 或第一步成功当作完整登录；需要先处理 `requiresTwoFactor`。

`TwoFactorMethod` 包括 `password`、`passkey`、`totp`、`phone`、`email`、`recovery`。密码通过已有挑战的 `/verify` 接口提交 `{method: "password", password}`，验证码方式继续提交 `{method, code}`；Passkey 使用独立 options／verify 接口。

用户和管理员的 `/2fa/reauthenticate` 提交 `{}` 即可获取可用方式。未开启两步验证的旧客户端仍可提交 `{password}` 换取一次性操作凭据；无任何本地可用方式的外部身份账户，只可在通过原应用登录后 5 分钟内继续。操作凭据绑定用户、会话、用途、凭据状态及配置版本，5 分钟内单次使用。

## 升级与兼容性

迁移 `20261014010000` 覆盖 MySQL、PostgreSQL 和 SQLite。保留存量两步验证开关、TOTP、恢复码及旧渠道因子数据；所有已开启账户改为使用全部已就绪方式。旧的短信／邮箱 2FA 绑定和移除接口返回明确提示，联系方式统一在个人资料中管理。

旧版本可能在管理员编辑或目录同步联系方式时保留旧地址的验证时间。迁移发现旧因子的接收地址与当前地址不同时，会清除该联系方式的验证状态并更新配置版本；用户需要重新验证当前地址，不会静默关闭两步验证。今后的联系方式更改也会清除旧验证状态，管理员显式标记新地址已验证的操作除外。

添加／删除 Passkey、改变两步验证配置或自助解绑外部身份会撤销旧会话及关联令牌，轮换当前浏览器会话。重命名只修改名称。配置变化后客户端需丢弃旧应用 access token，使用轮换后的 Cookie。管理员／目录修改联系方式时撤销受影响用户会话和未完成挑战。

后端和两个前端需协调升级。迁移不支持自动 Down；回退必须协调恢复应用、数据库和既有 TOTP 凭据密钥备份，不能直接删除 Passkey 表。

## 验证

```sh
go test ./...
go test -race ./app/service ./app/controller -run 'TestPasskey|TestTwoFactor|TestAuthentication' -count=1
go test -tags=integration ./migrations -run TestMigrationDialects -count=1
pnpm --dir frontend-oidc test
pnpm --dir frontend-oidc build
pnpm --dir admin-web test:unit
pnpm --dir admin-web --filter @vben/web-antd exec vue-tsc --noEmit --skipLibCheck
pnpm --dir admin-web build:antd
go build ./...
```

浏览器测试使用隔离 SQLite、内存 Redis 和虚拟认证器，不访问生产数据。先编译前端，再运行：

```sh
IAM_PASSKEY_BROWSER_TEST=1 go test ./app/controller -run '^TestPasskeyBrowser$' -count=1 -v
```

终端打印 `PASSKEY_BROWSER_URL`；服务最多存活 5 分钟。在另一个终端打开该地址的 `/auth/#/account/login`（脚本使用 hash 路由构建），然后运行：

```sh
playwright-cli -s=iam-passkey open 'http://localhost:打印的端口/auth/#/account/login' --browser=chrome
playwright-cli -s=iam-passkey run-code --filename=scripts/passkey-browser-check.js
```

脚本验证注册、免用户名登录、双向两步验证、关闭保留、重命名、窄屏、OIDC、跨站 iframe 和删除。测试后向该服务 `/__test/stop` 发送 POST，并执行 `playwright-cli -s=iam-passkey close`。
