# OIDC 浏览器跨域接入与验收

## 端点策略

| 端点 | CORS 来源 | 方法 | 浏览器凭证 |
| --- | --- | --- | --- |
| `/oidc/{client_id}/.well-known/openid-configuration`、`/jwks` | 任意来源，`Access-Control-Allow-Origin: *` | GET、OPTIONS | `credentials: "omit"` |
| `/token`、`/revoke`、`/introspect` | 当前启用应用的 HTTP/HTTPS 回调地址 Origin | POST、OPTIONS | 客户端认证或 Bearer Token，不依赖 IAM Cookie |
| `/userinfo` | 当前启用应用的 HTTP/HTTPS 回调地址 Origin | GET、POST、OPTIONS | Bearer Token，不依赖 IAM Cookie |

OIDC 端点不返回 `Access-Control-Allow-Credentials`。协议 API 仅在匹配来源后返回具体 Origin，不回退使用全局管理端白名单。无 Origin 的服务端请求继续执行原 OAuth 鉴权。Discovery/JWKS 的错误响应同样可供浏览器读取。

应用来源按协议、主机和端口精确匹配，规范化域名和默认端口。例如注册 `https://pagnode.mliev.com/callback` 会允许 `https://pagnode.mliev.com`；不会允许其子域、相似域或非默认端口。带用户名密码的 URL、相对 URL、自定义协议和片段不会生成可信来源；协议 API 不接受 `Origin: null`。原生应用回调的回环随机端口规则不适用于浏览器 CORS。

来源配置从当前应用记录读取，回调或启用状态变更会在后续请求生效。预检缓存 300 秒；动态来源响应包含 `Vary: Origin`，预检额外包含 `Access-Control-Request-Method` 和 `Access-Control-Request-Headers`，保留已有缓存差异字段。

预检允许 `Content-Type`、`Authorization`、`Accept`、`Origin`、`X-Requested-With`、`X-Trace-Id`；未获准的来源、方法或请求头返回 403，不执行端点处理。普通未获准来源请求不附加 CORS 许可头，原 OAuth 鉴权仍执行。

## Pagnode 配置核对

在 IAM 应用中检查 `redirectUris`：必须登记 Pagnode 实际使用的完整回调 URI，其 Origin 应为 `https://pagnode.mliev.com`。不要只登记域名或猜测回调路径，也无需把该域名添加到全局管理 API 白名单。

前端读取公开元数据时显式不携带 Cookie：

```js
const response = await fetch(`${issuer}/.well-known/openid-configuration`, {
  credentials: 'omit',
});
const metadata = await response.json();
```

Token 请求使用授权码与 PKCE（公开客户端），或客户端身份认证（机密客户端）；UserInfo 请求通过 `Authorization: Bearer ...` 传递 Access Token。不要把机密客户端的密钥放进公开浏览器代码。IAM 的 `/authorize` 仍使用正常浏览器导航和登录会话，不改为携带 Cookie 的跨域 fetch。

Token、Revoke、Introspect 的认证客户端 ID 必须与 URL 中的客户端 ID 一致，否则返回 `invalid_client`。UserInfo 不接受其他应用的令牌，返回 `invalid_token`。携带 HTTP Basic 认证时同样检查绑定；URL 客户端与认证客户端不匹配的请求会在消费授权码之前被拒绝；授权码仍须通过原有客户端、回调地址与 PKCE 校验。

## 全局配置与兼容性

管理 API、`/api/oidc/` 登录页面接口、安装流程和 `/authorize` 继续使用原有策略。全局配置的有效键为 `app.cors.allowed_origins`、`allowed_methods`、`allowed_headers`、`expose_headers`、`allow_credentials`、`max_age`；旧示例的顶层 `cors.allow_origins` 不会被代码读取。`config.yaml.example` 已修正，修改全局 CORS 配置后应重启实例。

此前依赖全局白名单调用 OIDC 协议 API 的前端，需确认其 Origin 出现在该应用的 HTTP/HTTPS 回调地址中。仅有自定义协议回调的原生客户端不因此获得浏览器来源；无 Origin 的原生或后端调用继续可用。

## 本地回归

常规测试无需浏览器环境：

```sh
go test ./app/middleware ./app/controller ./app/service ./config/autoload -count=1
```

真实浏览器验收使用临时数据库、两个不同端口的页面来源和真实 IAM HTTP 路由；不连接生产账号、不模拟 Fetch/CORS 返回值。测试完成 Discovery → JWKS → 授权码与 PKCE 换 Token → UserInfo，并用 WebCrypto 验证 RS256 签名，同时验证未登记来源与携带 Cookie 的元数据请求被浏览器拒绝。

```sh
# 使用已安装 Playwright 和 Chromium 的 Python 环境。
OIDC_BROWSER_PYTHON=/path/to/venv/bin/python \
  go test ./app/controller -run '^TestOIDCCORSBrowserFlow$' -count=1 -v
```

如需指定现有 Chromium，可额外设置 `OIDC_BROWSER_EXECUTABLE=/path/to/chrome-headless-shell`。

## Kubernetes 手动发布后验收

构建并通过现有手动流程更新 IAM 镜像，确认全部副本完成滚动更新。Ingress/Nginx/CDN 应保留上游 CORS 响应头，不重复添加允许来源或凭证头。若代理缓存了旧响应，先清除相关缓存。此修复无需数据库迁移或新增应用配置字段。

运行只读线上检查（仅 GET 和 OPTIONS，不签发或撤销令牌）：

```sh
python3 scripts/check_oidc_cors.py \
  --issuer 'https://mliev.login.wang/oidc/01a11af1-ce42-794b-b2aa-0e6e089ed9dd' \
  --origin 'https://pagnode.mliev.com'
```

脚本检查 Discovery/JWKS、四个协议 API 的预检、响应头重复、缓存差异字段以及拒绝来源/方法/请求头。所有检查应为 PASS；若公开元数据通过而应用预检仍被拒绝，优先核对目标应用的实际回调 Origin 与启用状态。

最后从 Pagnode 页面走完真实登录和管理 API 授权，确认开发者工具中 Discovery、JWKS、Token、UserInfo 无 CORS 错误。代码和本地测试通过不表示线上已更新，只有新版本部署后的检查与真实页面验证通过，才算线上问题关闭。
