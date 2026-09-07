# 小程序服务端身份交换

`POST /external-auth/:identifier/miniprogram/identity` 仅供可信业务后端调用。使用 HTTP Basic 提交已有机密 OIDC 应用的 client ID 和 client secret，请求体为 `{ "jsCode": "微信临时 code" }`。

配置 `miniprogram.identity_exchange.enabled`、`client_id` 与 `connection`，默认关闭，示例见 `config/config.yaml.example`。调用应用、路径中的微信连接须与配置完全相同；公开客户端、非 OIDC 应用、错误密钥、禁用应用、停用身份、不可登录账户及无应用访问权限的用户均不能通过。

沿用 `{ code, message, data }` 响应：

- 已绑定且授权通过：`data.state = "authenticated"`，`data.claims` 为既有 OIDC 用户资料构建器的 `openid/profile/phone` 输出，`sub` 为稳定用户 UUID。
- 未绑定：`data.state = "needs_bind"`，没有 claims，不创建用户、不创建绑定 continuation、不签发 IAM token。

响应不包含微信 session_key，不供小程序直接消费。调用方应在 HTTPS 后端调用后匹配自己的业务账户，不接收客户端自报的 UUID、手机号或 userinfo。请求限时 12 秒；Redis 不可用时拒绝交换。应用与调用方 IP 组合每分钟最多 120 次。

原有小程序网页登录扫码协议不变：二维码包含 `h=<scanHandle>`，扫码得到 `confirmToken`，确认或取消必须同时提交二者。客户端不能用二维码句柄替代确认令牌。

部署顺序：IAM → Registry → 小程序。本接口不引入数据库表或迁移。
