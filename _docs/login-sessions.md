# 登录会话管理

一个浏览器登录对应一个持久化会话。个人中心、从该登录进入的管理后台和关联 OIDC 用户令牌共享会话 ID，角色权限仍独立验证。真实重新登录替换旧会话；SSO 和刷新继续使用原会话。

## 使用

- 个人中心 → 安全设置 → 登录会话：查看设备、IP、有效期和关联应用，退出指定会话或其他会话。
- 管理后台 → 组织与用户 → 用户管理 → 登录会话：分页查询全部用户，按用户 ID、状态、IP、应用 Client ID 筛选。用户列表和详情均有快捷入口。
- 列表的“有效”表示仍可通过该会话认证，不表示设备当前在线。最近活跃时间的写入间隔为一分钟。后台访问标记表示该会话曾进入后台。
- 手动退出会话会结束该会话的个人中心、后台登录和所有关联的 OIDC 用户令牌。历史保留 90 天；后台任务启动时和每 24 小时分批清理。
- 本人修改/设置密码保留当前会话并退出其他会话；管理员重置和验证码找回密码退出全部会话。禁用、删除及目录同步停用也会撤销会话。

## API

所有列表响应沿用 `{code,message,data}`，其中 `data` 为 `{items,total,page,size}`。查询参数为 `page`、`pageSize`、`status`，默认第一页、20 条、`active`，每页最多 100 条。状态筛选支持 `active/history/logged_out/revoked/expired/all`。管理员额外支持 `userId/ip/clientId`；个人接口始终限定当前用户。

| 方法与路径 | 行为 |
| --- | --- |
| GET `/api/user/sessions` | 本人会话及历史 |
| POST `/api/user/sessions/:sid/revoke` | 撤销本人的指定会话 |
| POST `/api/user/sessions/revoke-others` | 保留当前会话，撤销其他会话 |
| GET `/api/admin/sessions` | 超级管理员查询会话 |
| POST `/api/admin/sessions/:sid/revoke` | 超级管理员撤销指定会话 |
| POST `/api/admin/users/:id/sessions/revoke-all` | 超级管理员清退用户全部会话 |

写请求必须使用 `Content-Type: application/json`，无业务参数时提交 `{}`。指定会话和管理员全部清退返回 `{currentRevoked:boolean}`；当前会话被撤销后，客户端应清除本地状态，不应再发送一次登出请求。

个人接口显式 Bearer 头优先，否则先验证个人 Cookie、再验证后台 Cookie。管理接口显式 Bearer 头优先，否则只验证后台 Cookie，并要求有效的超级管理员会话；后台 Cookie 缺失或失效返回 401，不回退到个人 Cookie。显式提交普通用户 Bearer 或已失去超级管理员角色时仍返回 403。列表不包含访问令牌、刷新令牌或原始 Cookie。

门户切换账号后，浏览器本地可能仍保存后台登录标记。后台首次加载时会重新验证服务端管理员会话；失效时进入统一认证，使用当前账号重新验证超级管理员身份后再加载菜单和页面。

## 升级

迁移版本 `20260907000000` 支持 MySQL、PostgreSQL、SQLite。升级后所有无会话标识的 IAM JWT、旧用户 OIDC 令牌、旧授权码失效，用户需重新登录。`client_credentials` 机器令牌不受影响；不回填旧会话设备和历史信息。

发布时同步 Go 后端、`frontend-oidc` 与 `admin-web` 两个子模块构建产物。先停止旧版本节点，完成迁移后整体切换，避免新旧认证节点混跑。迁移完成前不能启用依赖会话表的新后端。回退程序不会恢复已经撤销的凭据。

首版在 IAM 鉴权、授权码兑换、刷新、userinfo、内省与 SSO 入口即时检查会话状态。接入应用自行缓存的登录态或仅离线验证 JWT 的服务不会自动收到退出通知；这里没有实现 OIDC Front/Back-Channel Logout。现有凭据有效期不变，会话有效期覆盖关联凭据的最晚到期时间。
