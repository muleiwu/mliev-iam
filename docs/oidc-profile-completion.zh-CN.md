# OIDC 资料补全后的会话换发

## 修复内容

原接口调用通用 `UpdateUser`：邮箱/手机验证成功后触发 `contact_changed`，撤销当前登录，下一次 `flow-status` 返回 `session_expired`。现在由专用服务完成资料补全和会话换发，管理员及目录同步修改联系方式的全量撤销语义保持不变。

1. 校验浏览器绑定、当前登录、应用权限、所需缺失字段及联系方式唯一性。
2. 按用户、登录会话顺序加锁，原子校验并消费本次全部验证码；任一验证码失败不消费其他验证码。
3. 数据库事务保存已验证资料、提升认证版本、撤销旧会话与凭证，创建待激活的新 SID。
4. Redis 条件更新只替换当前授权事务的 SID，保持原 TTL、认证时间、PKCE、state、nonce、CSRF 和授权参数；事务变更或删除时拒绝覆盖。
5. 再次核验会话和认证版本，事务性激活新 SID 并签发凭证。保留原登录方式及两步验证证据，不继承管理员权限，不通过本次换发延长登录期限。
6. 同步交付 IAM 用户 Cookie、OIDC Refresh Cookie 和前端 OIDC Access Token。前端先替换缓存凭证再推进授权。

待激活会话不能鉴权、签发 Token 或出现在正常会话列表。接续失败时保留已提交资料、撤销替代会话，返回“资料已保存，请重新登录后继续授权”。数据库回滚而验证码已消费时要求重新取码。并发提交至多成功一次，失败请求不覆盖成功请求的 Cookie。

## 接口及迁移

`POST /api/oidc/complete-profile` 新增请求字段 `sessionRotationSupported: true`。旧页面会在消费验证码前收到刷新提示。成功响应增加 `accessToken`、`expiresIn`、`rememberMe`，原 `success/message/nextUrl` 保留。

| 结果 | HTTP | 行为 |
|---|---|---|
| 成功 | 200 | `success: true`，新凭证就绪 |
| 旧页面、错误验证码、字段不合法 | 400 | 留在表单，不换发会话 |
| 重复提交、会话状态变更 | 409 | 不写 Cookie、不清除其他请求的新凭证 |
| 应用权限不足 | 403 | 不修改资料 |
| 数据库保存失败 | 500 | 事务回滚，提示重新获取验证码 |
| 资料已提交但接续失败 | 200 | `success: false`、`error.code: login_required`，提供已验证应用的返回地址 |

迁移版本 `20261112000000`，覆盖 SQLite、MySQL、PostgreSQL：

- `login_sessions.remember_me`：可空布尔值；历史未知值换发时按非持久登录处理。
- `login_sessions.activation_pending`：默认 false，仅此次流程的替代会话在交付前设为 true。

部署时前后端需同步发布。首次启动执行迁移，旧标签页刷新后继续。构建前端后再打包后端，确保嵌入的静态页面支持新接口。换发日志使用 `[oidc-profile]`，记录阶段和关联会话标识，不记录验证码或完整 Token。

回滚前应先停止新换发请求并撤销所有待激活会话，再回滚前后端；保留新增列，不执行生产 Down，不恢复已撤销的凭证。旧版本不认识待激活标志，不能在待激活记录仍有效时直接切回旧版本。

## 自动化验证

在 `mliev-iam` 仓库运行：

```bash
go test ./...
go test -race ./app/service ./app/controller -run 'TestOIDCProfileCompletion|TestAuthentication|TestTwoFactor|TestFirstParty|TestAccountAuth' -count=1
go test -tags=integration ./migrations -run TestMigrationDialects -count=1
pnpm --dir frontend-oidc test
pnpm --dir frontend-oidc build
```

2026-11-12 本地结果：上述检查全部通过；前端 32 个测试文件、507 项测试通过。真实数据库集成验证使用 MySQL 8.4、PostgreSQL 16，验证新建、升级、回滚及再次升级；SQLite 同样验证实际迁移。

HTTP 回归覆盖注册后补邮箱、补手机、同时补全、自动及手动授权、授权码交换、记住我、历史未知持久性、两步验证继承、旧凭证失效、错误/过期/占用/已有联系方式/无浏览器绑定/撤销会话/停用应用、并发提交、Redis 不可用、授权事务变更/删除、并发撤销、签发失败和数据库回滚。故障后重新登录可使用已保存资料。

## 本地浏览器冒烟

可重复启动仅监听本机的隔离服务，不使用真实数据库、邮箱或短信供应商：

```bash
IAM_AUTH_OUT_DIR=/tmp/iam-profile-auth-build pnpm --dir frontend-oidc build
IAM_PROFILE_BROWSER_TEST=1 IAM_AUTH_OUT_DIR=/tmp/iam-profile-auth-build \
  go test ./app/controller -run '^TestOIDCProfileCompletionBrowser$' -count=1 -v
```

服务输出 `PROFILE_BROWSER_URL`，15 分钟后超时。以下 `__test` 路由只存在于显式启用的测试二进制，不注册到生产应用。

1. 请求 `GET /__test/start?fields=email&prompt=create`，读取 `data.url`、用户名和测试密码，打开 URL 实际完成注册。
2. 输入测试邮箱前，通过 `POST /__test/code` 写入隔离验证码，JSON 为 `{"sessionId":"...","type":"email","receiver":"..."}`；验证码固定 `123456`。手机对应 `type: phone`，receiver 使用 E.164。
3. 在页面填写邮箱和验证码，点击“验证并继续”；应返回测试应用并显示“授权完成”，回调真实交换授权码。
4. `fields=email%20phone&prompt=consent` 测试双字段和手动授权；此模式会创建一个可用于页面登录的测试账号。各场景使用不同邮箱/手机号。
5. 登录之后调用 `POST /__test/fail-issuance` 可注入签发失败；应显示资料已保存和重新登录提示，并可返回应用。下次 `/__test/start` 自动清除故障注入。
6. `POST /__test/stop` 正常结束服务。

| 浏览器/场景 | 本地结果 |
|---|---|
| 桌面 Chrome：创建账号、补邮箱、自动授权、授权码交换 | 通过 |
| 桌面 Chrome：回调后刷新 Token、个人中心访问 | 通过 |
| 桌面 Chrome：双字段、错误验证码重试、手动授权 | 通过 |
| 桌面 Chrome：签发故障后的提示、清除旧缓存及返回应用 | 通过 |
| 移动 WebKit：390×844、触屏及 iPhone 微信 UA 模拟；注册、邮箱补全、回调、刷新和个人中心 | 通过；不等同真机 |
| iPhone 微信真机、测试环境真实收码 | 未验证，需要可用设备和测试环境 |

浏览器模拟不等同于微信真机。发布验收仍需在测试环境使用受控邮箱/手机号，在 iPhone 微信内置浏览器完成一次原始注册路径，并核对 Cookie 持久性与真实收码；本次未部署线上。
