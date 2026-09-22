# MLiev IAM 架构图 / Architecture Diagrams

使用 [Archify](https://github.com/tt-a1i/archify) 生成的交互式架构图（独立 HTML，支持深/浅主题、缩放、搜索、焦点追踪与 PNG/SVG 导出）。

Interactive architecture diagrams generated with Archify (standalone HTML with dark/light themes, pan/zoom, search, focus tracing, and PNG/SVG export).

## 图表清单 / Diagrams

| 图 / Diagram | 中文版 | English |
|---|---|---|
| 系统总体架构 / System Architecture | [zh-CN/system-architecture.html](zh-CN/system-architecture.html) | [en/system-architecture.html](en/system-architecture.html) |
| OIDC 授权码流程 / OIDC Authorization Code Flow | [zh-CN/oidc-authorization-flow.html](zh-CN/oidc-authorization-flow.html) | [en/oidc-authorization-flow.html](en/oidc-authorization-flow.html) |
| HTTP 请求处理链路 / HTTP Request Lifecycle | [zh-CN/request-lifecycle.html](zh-CN/request-lifecycle.html) | [en/request-lifecycle.html](en/request-lifecycle.html) |
| 启动装配流程 / Boot & Assembly Workflow | [zh-CN/boot-assembly.html](zh-CN/boot-assembly.html) | [en/boot-assembly.html](en/boot-assembly.html) |

## 内容说明 / Notes

- **系统总体架构**：前端两应用（`admin-web/apps/web-antd` → `/admin`、`frontend-oidc` → `/auth`）→ Gin HTTP Server（go-web 全局中间件链）→ Controller/Service/DAO 分层 → MySQL/Redis/S3；协议工厂支持 OIDC/SAML/CAS/OAuth2。
  System overview: two frontend apps (`admin-web/apps/web-antd` → `/admin`, `frontend-oidc` → `/auth`) → Gin HTTP Server (go-web global middleware chain) → Controller/Service/DAO layers → MySQL/Redis/S3; the protocol factory supports OIDC/SAML/CAS/OAuth2.
- **OIDC 授权码流程**：三方应用 ↔ 浏览器 ↔ `/auth` 登录前端 ↔ IAM 后端（authorize → 登录 → consent → code → token → userinfo），含 PKCE 与 Redis 会话（`oidc:session:` · 10 分钟 TTL）。
  The authorization-code flow with PKCE across app, browser, the `/auth` login UI, and the IAM backend (authorize → login → consent → code → token → userinfo), with Redis-backed sessions.
- **HTTP 请求处理链路**：traceId → 访问日志 → Recovery → CORS → 安装门禁 → 鉴权中间件（JWT + DB 会话）→ WrapHandler → Controller → Service → DAO/GORM，含 401 与安装重定向分支。
  The request lifecycle through the global middleware chain, auth middleware, WrapHandler, and the Controller → Service → DAO stack, with 401 and install-redirect branches.
- **启动装配流程**：main.go → RegisterAssemblies（DependsOn 拓扑排序）→ Env → Config → Logger → DB → Redis → Cache → startupChecks（goose 迁移）→ 服务启动；含 SIGHUP / API 软重载支路。
  The boot chain from `main.go` through topologically-sorted assemblies and goose migrations to serving, plus the SIGHUP/API soft-reload branch.

## 维护 / Maintenance

各图的 Archify JSON 规格源在 [spec/](spec/)（`<名称>.<语言>.json`）。修改后重新生成：

The Archify JSON sources live in [spec/](spec/) (`<name>.<lang>.json`). Regenerate after edits:

```bash
cd ~/.claude/skills/archify
node bin/archify.mjs validate <type> docs/archify/spec/<name>.<lang>.json --quality showcase --json
node bin/archify.mjs deliver  <type> docs/archify/spec/<name>.<lang>.json docs/archify/<lang>/<name>.html --quality showcase --json
```

`<type>` 取值：`architecture` / `sequence` / `workflow`。

生成日期 / Generated: 2026-09-22 · 基于代码实际结构（go-web v1.9.0 框架装配、goose 迁移、frontend-oidc 子模块）绘制。
