# 对接第三方 OIDC 身份认证源

IAM 可以作为 OIDC 客户端，使用第三方身份提供方登录和绑定现有账号。这与 IAM 自身向业务应用提供 OIDC 服务是两个独立方向。

## 配置步骤

1. 在第三方身份提供方创建 OIDC 客户端，启用 **Authorization Code** 流程和 **PKCE S256**。
2. 在管理后台「身份连接」中新建连接。企业连接、社交连接均可选择「标准 OIDC」。
3. 填写连接名称、唯一标识、第三方提供的完整 **Issuer**、**Client ID** 和 **Client Secret**。Issuer 可以包含租户或 realm 路径，末尾斜杠必须与提供方公布的值一致。
4. 点击「发现配置」，检查授权端点、Token 端点及 JWKS 签名公钥端点。系统请求 `<Issuer 去掉末尾斜杠>/.well-known/openid-configuration`；登录和启用校验也会自动发现，无需手填端点。
5. 按第三方要求选择 Token 端点认证方式：默认 `auto` 根据 Discovery 优先选择 `client_secret_basic`，其次 `client_secret_post`；未声明支持方式时自动尝试。也可显式指定上述方式。公共客户端必须选择 `none`，无需密钥，且不会向第三方发送此前保存的密钥。
6. 首次可先保存为禁用状态。重新打开连接，复制后端生成的「平台登记的登录回调地址」到第三方客户端的允许回调列表，必须完全一致。回调路径为 `/external-auth/<连接标识>/callback`。
7. 启用连接，点击「检查」查看结果。启用「全局展示」，或在具体应用的登录方式设置中选择该连接。

Scopes 默认是 `openid profile email`，可用逗号或空白分隔；系统会确保包含 `openid`。根据第三方授权策略调整 `profile`、`email` 或其他 scope。

OIDC 连接只提供登录认证能力，不提供目录同步或事件回调。凭据通过现有连接凭据存储加密保存；编辑时 Client ID、Client Secret 留空保留原值。

「检查」和启用时的校验会检查配置及 Discovery，不会模拟用户登录，也不能确认客户端密钥正确。完成第三方回调地址登记后，请实际登录一次以验证凭据、PKCE 和 ID Token。

## 登录与账号绑定

- 登录页使用现有外部身份登录流程。首次登录的未绑定身份进入现有账号绑定流程；已绑定身份按原有规则登录。
- 账号中心使用同一 OIDC 协议完成绑定。
- 认证请求发送独立随机 nonce 和 PKCE S256 challenge。nonce、verifier 与连接、登录会话保存在一次性 state 中，5 分钟过期。
- 服务端交换授权码后，根据 discovery 的 JWKS 校验 ID Token 签名，并检查 Issuer、受众、有效期、签发时间、nonce，以及适用的 `azp` 和 `at_hash`。
- 用户唯一标识固定来自验证后的 `sub`，按身份连接隔离。有 UserInfo 端点时补充资料，并要求其 `sub` 与 ID Token 一致；未提供 UserInfo 端点时使用 ID Token 中的资料。
- 已存在外部身份的连接不能切换 Issuer 或提供商类型。对接另一个身份提供方时应新建连接，避免不同 Issuer 的相同 `sub` 被视为同一个人。
- 尚未绑定账号的连接修改 Issuer 后，旧的待绑定资料失效，用户需重新认证。
- 不按邮箱自动合并账号或自动创建本地用户；本地用户或外部身份被停用后，第三方认证成功也不能登录。

## 对接要求与排查

- Issuer、授权端点、Token、JWKS 和可选 UserInfo 必须是 HTTPS；仅 `localhost` 和回环 IP 允许 HTTP 本机开发。请求使用系统可信证书，后端请求超时 10 秒，不跟随端点重定向。
- Discovery 必须返回匹配的 `issuer`，并包含 `authorization_endpoint`、`token_endpoint`、`jwks_uri`。发现失败直接报错，不回退到旧端点。
- 支持 JWKS 非对称签名的 ID Token，包括 RSA、ECDSA、RSA-PSS 和 EdDSA；不接受无签名或共享密钥 HMAC ID Token。未声明签名算法时默认 RS256。
- Discovery 使用 Redis 缓存 24 小时；没有 Redis 缓存时仍可检查配置。「发现配置」会重新获取并刷新缓存。浏览器登录本身仍依赖现有 Redis state 存储。
- `invalid_client`：核对客户端 ID、密钥和 Token 端点认证方式。
- `invalid_grant`：核对第三方登记的回调地址及 PKCE S256 支持，重新发起登录。
- Issuer 不一致：使用第三方 Discovery 文档中的准确 Issuer，保留租户路径和末尾斜杠。
- 缺少 `id_token`：确认第三方客户端为 OIDC 类型并允许 `openid` scope。
- nonce 或 UserInfo sub 不一致：该响应会被拒绝，需检查第三方的 OIDC 实现。

## 从旧版 OIDC 连接升级

无需数据库迁移。原有社交 OIDC 连接继续使用相同连接标识、凭据和账号绑定；企业连接现在也可以选择 OIDC。

旧版仅依赖 OAuth2/UserInfo 或手工端点的配置，需要补齐准确的 Issuer，确保 Discovery 提供 JWKS、第三方启用 PKCE S256，并在 Token 响应中返回带正确 nonce 的签名 ID Token。系统不再使用 Discovery 失败后的旧端点，也不再接受未验证的 UserInfo 身份。升级时正在进行的旧 OIDC 登录事务缺少 nonce/verifier，用户需重新发起登录。

本功能使用授权码流程，不包含 Implicit/Hybrid、第三方单点登出、目录同步或自动开户。

协议依据：[OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation)、[OpenID Connect Discovery](https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderConfigurationValidation)。
