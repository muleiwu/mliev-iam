# 首次安装引导 / First-time Installation Guide

> English readers: see the *First-time install* paragraph in the [README](../README.md) for a summary. This document keeps the detailed Chinese procedure.

首次启动不要求数据库和 Redis 已连接，访问 `/install/`。开源版首先展示完整的
[MIT 授权协议](../LICENSE)，主动勾选同意后，按数据库、Redis、管理员和确认步骤完成安装。
协议随程序内嵌，支持离线阅读；默认不勾选，无需滚动到底，刷新页面后需重新确认。
企业版、高校版及其他版本直接进入数据库配置；已安装实例升级后无需补签。
安装页面由 setup-kit v0.1.0 内嵌提供，不再构建独立前端。

通过 API 安装开源版时，先读取 `/install/api/schema` 返回的 `agreements`，
确认协议后在提交的 `values` 中设置 `setupkit.agreements.mit-license` 为当前协议的 `digest`。
缺失、布尔值或过期摘要会被后端拒绝；同意信息不写入配置或安装状态。

支持 MySQL、PostgreSQL、SQLite；Redis 必选。管理员密码至少 12 个字符，
保留姓名与可选手机号，使用系统现有 Argon2id 哈希。

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
