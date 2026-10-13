# 首次安装引导 / First-time Installation Guide

> English readers: see the *First-time install* paragraph in the [README](../README.md) for a summary. This document keeps the detailed Chinese procedure.

首次启动不要求数据库和 Redis 已连接，访问 `/install/`。开源版首先展示完整的
[MIT 授权协议](../LICENSE)，主动勾选同意后，按数据库、Redis、管理员和确认步骤完成安装。
协议随程序内嵌，支持离线阅读；默认不勾选，无需滚动到底，刷新页面后需重新确认。
企业版、高校版及其他版本直接进入数据库配置；已安装实例升级后无需补签。
安装页面由 setup-kit v0.2.0 内嵌提供，不再构建独立前端。

通过 API 安装开源版时，先读取 `/install/api/schema` 返回的 `agreements`，
确认协议后在提交的 `values` 中设置 `setupkit.agreements.mit-license` 为当前协议的 `digest`。
缺失、布尔值或过期摘要会被后端拒绝；同意信息不写入配置或安装状态。

支持独立选择 MySQL、MariaDB、PostgreSQL、SQLite，默认 MySQL；Redis 必选。管理员密码至少 12 个字符，
保留姓名与可选手机号，使用系统现有 Argon2id 哈希。

选择 MariaDB 时，安装 API 使用 `database.driver: mariadb` 和 `database.mariadb.*`
连接字段。保存后的配置保留 `database.driver: mariadb`，连接参数写入 `database.host`、
`database.port` 等通用字段；连接检查、安装迁移和运行时均复用 MySQL 驱动。
环境变量 `DB_DRIVER=mariadb` 同样支持，运行时映射不会改写配置文件。

安装检查与正式运行均支持 `database.tls`、`database.charset`、`redis.username`、
`redis.tls`。TLS 使用系统 CA 验证证书与主机名，不支持跳过验证。
默认 SQLite 路径位于配置目录；自定义路径的父目录须已存在。

配置及完成标记原子保存到实际读取的配置文件，首次默认 `config/config.yaml`，
权限为 0600。进度记录在配置文件旁的 `.setup-state.json` 文件中，不记录密码。
旧 `installation-id` 会被沿用，失败重试不会重复创建管理员或重置密码。
请同时备份配置、数据库和 `identity-credentials.key`；已有环境变量密钥继续优先。

保存成功后自动重启；在完成页点击“进入应用”，入口等待服务就绪后进入后台。
旧安装提交和测试接口已经移除；自动化改用 `/install/api/schema`、`status`、
`checks/:id`、`submit`，JSON 请求格式为 `{"values":{...}}`，并携带
`X-Setup-Kit: 1`。只读 `/api/install/check` 继续提供 installed/ready 状态。
代理应保留 Host 和正确的协议，必要时通过 `install.allowed_origins` 配置明确的外部来源。

衍生发行版可以通过 `config.App.SetupName` 设置安装器名称，通过 `SetupEntryPath`
设置安装完成后的站内入口。留空时保留原发行版名称及 `/admin/`；完成页的
`/install/enter` 在等待就绪和直接跳转时都使用相同的入口配置。

## 安装变量预填

社区版、企业版和高校版共用以下 `IAM_SETUP_*` 安装专用环境变量。在首次启动时读取，
预填值可以在安装页面修改或清空；不会自动提交安装，社区版仍需主动确认协议。

| 环境变量 | 预填内容 |
|---|---|
| `IAM_SETUP_DB_DRIVER` | 数据库类型：`mysql`、`mariadb`、`postgresql`、`sqlite` |
| `IAM_SETUP_DB_HOST` / `IAM_SETUP_DB_PORT` | 网络数据库主机、端口 |
| `IAM_SETUP_DB_USERNAME` / `IAM_SETUP_DB_PASSWORD` | 网络数据库用户名、密码 |
| `IAM_SETUP_DB_DATABASE` | 网络数据库名称 |
| `IAM_SETUP_DB_TLS` | 网络数据库 TLS 开关 |
| `IAM_SETUP_DB_CHARSET` | MySQL、MariaDB 字符集：`utf8mb4` 或 `utf8` |
| `IAM_SETUP_DB_FILEPATH` | SQLite 文件路径，父目录须已存在 |
| `IAM_SETUP_REDIS_HOST` / `IAM_SETUP_REDIS_PORT` | Redis 主机、端口 |
| `IAM_SETUP_REDIS_USERNAME` / `IAM_SETUP_REDIS_PASSWORD` | Redis ACL 用户名、密码 |
| `IAM_SETUP_REDIS_DB` / `IAM_SETUP_REDIS_TLS` | Redis 数据库编号、TLS 开关 |
| `IAM_SETUP_ADMIN_USERNAME` / `IAM_SETUP_ADMIN_EMAIL` | 管理员账号、邮箱 |
| `IAM_SETUP_ADMIN_REAL_NAME` / `IAM_SETUP_ADMIN_PHONE` | 管理员姓名、手机号 |
| `IAM_SETUP_ADMIN_PASSWORD` | 管理员密码，同时预填确认密码；至少 12 个字符 |

端口必须为 1–65535 的整数，Redis 数据库编号为非负整数；布尔值支持 `true`、`false`、
`1`、`0` 等 Go `strconv.ParseBool` 格式。空字符串或未设置的变量沿用原默认值。
未安装实例启动时校验所有已绑定的非空变量，包括未选择的数据库类型；非法值会使安装器初始化失败，
错误指出字段和变量名，不包含变量实际值。

优先级为：**用户提交值 → 环境预填值 → 原默认值**。用户主动清空字段后不会再次预填，
必填字段仍会报错；API 省略字段时使用预填值。变量只在安装器创建时读取，运行期间修改需重启才能生效。
安装完成后，重启会忽略全部 `IAM_SETUP_*` 变量，残留的无效值也不会阻止启动。
这些变量不参与运行配置覆盖；现有 `DB_*`、`REDIS_*` 运行变量仍按原有规则生效。

密码预填会通过安装前的 `/install/api/schema` 返回浏览器，页面以密码控件显示，确认页掩码展示。
管理员密码仅用于创建账号，不写入配置或进度文件；数据库、Redis 密码按原有映射写入权限为 0600 的配置文件。
安装完成后 Schema 不再返回敏感默认值，错误及安装进度也不会记录密码。

### 命令行

```bash
IAM_SETUP_DB_DRIVER=sqlite \
IAM_SETUP_DB_FILEPATH=./config/iam.db \
IAM_SETUP_REDIS_HOST=127.0.0.1 \
IAM_SETUP_REDIS_PORT=6379 \
IAM_SETUP_ADMIN_USERNAME=admin \
IAM_SETUP_ADMIN_EMAIL=admin@example.com \
IAM_SETUP_ADMIN_REAL_NAME='管理员' \
./iam start
```

### Docker / Compose

Docker 使用 `-e IAM_SETUP_DB_DRIVER=sqlite` 等选项传入变量；例如：

```bash
docker run --rm --network iam-net -p 8080:8080 -v iam-config:/app/config \
  -e IAM_SETUP_DB_DRIVER=sqlite \
  -e IAM_SETUP_DB_FILEPATH=/app/config/iam.db \
  -e IAM_SETUP_REDIS_HOST=redis \
  your-iam-image:tag
```

上例的 `iam-net` 为示例网络名，替换为已有网络，确保其中有可访问的 `redis` 服务。
`your-iam-image:tag` 替换为已构建的 IAM 镜像。
Compose 可在现有 `services.iam` 中合并以下片段：

```yaml
services:
  iam:
    environment:
      IAM_SETUP_DB_DRIVER: sqlite
      IAM_SETUP_DB_FILEPATH: /app/config/iam.db
      IAM_SETUP_REDIS_HOST: redis
      IAM_SETUP_REDIS_PORT: "6379"
      IAM_SETUP_ADMIN_USERNAME: "${IAM_SETUP_ADMIN_USERNAME:-}"
      IAM_SETUP_ADMIN_EMAIL: "${IAM_SETUP_ADMIN_EMAIL:-}"
      IAM_SETUP_ADMIN_REAL_NAME: "${IAM_SETUP_ADMIN_REAL_NAME:-}"
      IAM_SETUP_ADMIN_PASSWORD: "${IAM_SETUP_ADMIN_PASSWORD:-}"
```

`redis` 必须是该 Compose 网络内可访问的服务名。安装器只读取进程环境，不加载 `.env`；
Compose 的 `.env` 插值需通过 `environment`（如上）或 `env_file` 明确传入容器。
