# MLiev IAM

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev)

[English](README.md) | 简体中文

企业级身份认证与访问管理（IAM）系统，提供单点登录（SSO）——基于 Go 构建的完整
OIDC / OAuth2 服务提供方，内嵌管理控制台与登录界面、细粒度 RBAC 权限以及企业身份目录集成，
单二进制即可部署。

## 特性

- **OIDC / OAuth2 服务提供方** — 支持 PKCE 的授权码流程、令牌内省与撤销、JWKS 与发现文档；应用也可注册为 SAML、CAS 或纯 OAuth2 客户端
- **单点登录** — 一个身份中枢服务所有应用，端点统一在 `/oidc/:client_id/*` 之下
- **社交登录** — Google、GitHub、QQ、微信、微信小程序及通用 OAuth2/OIDC 身份源
- **企业目录同步** — 飞书、企业微信、钉钉目录同步，含事件回调与同步自动化
- **带数据范围的 RBAC** — 角色、菜单/按钮/API 权限与四级行级数据范围控制
- **内嵌前端** — Vben + Ant Design Vue 管理控制台位于 `/admin`，独立的 Vue 3 登录/授权界面位于 `/auth`，均通过 `go:embed` 打入二进制
- **灵活存储** — MySQL（默认）/ PostgreSQL / SQLite，goose 版本化 SQL 迁移随启动自动执行；Redis 承载会话与缓存；S3 兼容对象存储承载附件
- **配置热重载** — 通过 `SIGHUP` 信号或管理 API 在不重启进程的情况下重载配置
- **引导式安装** — 内置安装向导（协议 → 数据库 → Redis → 管理员），无需单独构建安装前端
- **单二进制部署** — 模板与静态资源全部内嵌；debug 模式从磁盘读取以支持热更新

## 架构

交互式架构图由 [Archify](https://github.com/tt-a1i/archify) 生成，提供中英文双语版本，
支持深/浅主题、缩放、搜索与 PNG/SVG 导出。索引见 [docs/archify](docs/archify/README.md)。

| 图表 | 中文 | English |
|---|---|---|
| 系统总体架构 | [中文](docs/archify/zh-CN/system-architecture.html) | [English](docs/archify/en/system-architecture.html) |
| OIDC 授权码流程 | [中文](docs/archify/zh-CN/oidc-authorization-flow.html) | [English](docs/archify/en/oidc-authorization-flow.html) |
| HTTP 请求处理链路 | [中文](docs/archify/zh-CN/request-lifecycle.html) | [English](docs/archify/en/request-lifecycle.html) |
| 启动装配流程 | [中文](docs/archify/zh-CN/boot-assembly.html) | [English](docs/archify/en/boot-assembly.html) |

概览：两个前端应用（`admin-web` → `/admin`、`frontend-oidc` → `/auth`）由基于
[go-web](https://cnb.cool/mliev/open/go-web) 框架构建的 Gin 服务承载。全局中间件链
（traceId → 访问日志 → Recovery → CORS → 安装门禁）覆盖所有由 JWT 鉴权中间件保护的路由组；
请求随后经 Controller → Service → DAO 分层，通过 GORM 访问数据库，Redis 则支撑
OIDC 会话与设置缓存。OIDC/SAML/CAS/OAuth2 协议处理器及目录、社交、事件适配器位于
`app/service/protocol/`。协议细节见 [OIDC_README.md](OIDC_README.md)。

## 快速开始

### 前置要求

- Go 1.26+
- MySQL 8+ / PostgreSQL 13+ / SQLite 3+
- Redis 6+

### 克隆、配置与运行

```bash
# 前端为 git 子模块
git clone --recurse-submodules https://cnb.cool/mliev/iam/mliev-iam.git
cd mliev-iam

cp config.yaml.example config.yaml    # 编辑数据库 / Redis 配置

# OIDC 签名密钥（如以其他方式管理密钥可跳过）
openssl genrsa -out keys/oidc_private_key.pem 2048
openssl rsa -in keys/oidc_private_key.pem -pubout -out keys/oidc_public_key.pem

go run .                              # 仅启动后端（debug 模式）
```

其他常用工作流（完整清单见 `Makefile`）：

```bash
make dev        # 前端 watch + Go 后端（debug 模式）
make all        # 构建前端 + Go 后端
make build-go   # 仅构建 Go 后端
```

### Docker

```bash
docker build -t mliev-iam .
docker run -p 8080:8080 mliev-iam
```

### 首次安装

首次启动不要求数据库和 Redis 已配置就绪：访问 `/install/`，按内嵌向导
（协议同意 → 数据库 → Redis → 管理员）完成安装。向导由二进制内的 setup-kit 提供，
无需单独的安装前端。API 安装方式、TLS 选项与备份说明详见
[docs/installation-guide.zh-CN.md](docs/installation-guide.zh-CN.md)。

## 配置与热重载

运行时重载配置，无需重启进程：

```bash
kill -HUP <PID>              # 通过信号
./scripts/reload-config.sh   # 通过管理 API（需要管理员令牌）
```

详见[配置重载指南](_docs/CONFIG_RELOAD_GUIDE.md)。

## 文档

- [交互式架构图](docs/archify/README.md) — 双语
- [OIDC 协议说明](OIDC_README.md)
- [OAuth2 系列教程](docs/oauth2/)
- [开发规范](_docs/PROJECT_SPECIFICATION.md)
- [JWT 认证指南](_docs/JWT_AUTHENTICATION_GUIDE.md)
- [RBAC 技术实现](_docs/RBAC_TECHNICAL_IMPLEMENTATION.md)
- [测试指南](_docs/TESTING_GUIDE.md)
- [Docker 部署说明](_docs/docker/)
- [首次安装指南](docs/installation-guide.zh-CN.md)

## 项目结构

```
mliev-iam/
├── app/            # 控制器、服务、DAO、模型、DTO、中间件
├── config/         # 应用装配与 autoload 配置提供者
├── internal/       # JWT 密钥管理、凭证加密、密码哈希
├── pkg/            # 扩展模块、安装状态、凭证
├── migrations/     # goose 版本化 SQL 迁移（mysql / postgresql / sqlite）
├── admin-web/      # 管理台前端（子模块 → static/admin → /admin）
├── frontend-oidc/  # OIDC 登录前端（子模块 → static/auth → /auth）
├── static/         # 内嵌静态资源（由前端构建产物填充）
├── templates/      # 内嵌 HTML 模板
├── docs/           # 架构图与指南
└── _docs/          # 工程文档（规范、指南、报告）
```

## 贡献指南

1. Fork 本仓库
2. 创建功能分支
3. 提交变更
4. 发起 Pull Request

请遵循[开发规范](_docs/PROJECT_SPECIFICATION.md)。

## 升级说明

迁移版本 `20260922000001` 允许 `client_credentials` 应用反复申请访问令牌
（机器令牌的刷新令牌列存储为 `NULL`）。升级前请备份数据库并停止旧节点——
该迁移无法通过 `goose Down` 回滚。详见
[docs/oidc-machine-token-upgrade.zh-CN.md](docs/oidc-machine-token-upgrade.zh-CN.md)。

## 许可证

基于 [MIT License](LICENSE) 发布。

---

© 2026 MLiev IAM