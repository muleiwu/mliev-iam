# OIDC 机器令牌升级 / OIDC Machine Token Upgrade

> English readers: see *Upgrade notes* in the [README](../README.md) for a summary. This document keeps the detailed Chinese procedure.

迁移版本 `20260922000001` 将没有刷新令牌的 `client_credentials` 记录改为存储
`NULL`，以支持后台应用反复申请访问令牌；真实刷新令牌仍保持唯一。
升级时先备份数据库并停止旧版本节点，完成迁移后再启动新版本，避免新旧节点混跑。
此迁移无法在已有多条机器令牌后无损恢复旧约束，因此 `goose Down` 会明确失败并保留
迁移版本号。需要回退时，使用升级前的数据库备份及匹配的程序版本。
