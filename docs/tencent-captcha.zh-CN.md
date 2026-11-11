# 腾讯验证码 AppId 加密与一次一密

Web 腾讯验证码默认启用 AppId 加密，适用于 OIDC 页面和管理端验证码接入，覆盖登录、注册、找回密码、发送短信与邮件场景。无需新增 IAM 配置开关或迁移数据库。

## 腾讯控制台与 IAM 配置

1. 在腾讯验证码控制台创建 Web/App 类型的验证码，配置实际使用的域名。
2. 保留「CaptchaAppId 强制校验」和「一次一密校验」开启。
3. 在 IAM 中填写对应的 `captchaAppId`、`appSecretKey`、云 API `secretId` 和 `secretKey`，并绑定业务场景。AppSecretKey 与云 API SecretKey 是不同的密钥。

IAM 在后端使用 AppSecretKey 加密；密钥不会下发到浏览器。算法与格式遵循[腾讯 CaptchaAppId 鉴权指引](https://cloud.tencent.com/document/product/1110/128489)：

- AES-256-CBC，PKCS#7 填充；短密钥按字节循环补足 32 字节，空密钥及超过 32 字节的密钥报错。
- 明文：`CaptchaAppId&当前 Unix 秒数&300`。服务器时钟需准确，腾讯不接受未来的签发时间。
- 每次签发生成新的安全随机 16 字节 IV，返回 `Base64(IV + 密文)`。
- 腾讯负责拒绝重复 IV；IAM 的验证码会话与票据防重放校验继续执行。

微信小程序仍使用 `tencent_miniprogram` 提供商及原有 `aidEncryptionEnabled` 配置；未开启时不生成密文。它与 Web 共用加密实现，不改变小程序的启用方式。

## 参数接口与客户端

`POST /api/captcha/params` 请求结构和业务接口提交的 `CaptchaTicket` 保持不变。启用腾讯 Web 验证码时，响应数据示意：

```json
{
  "enabled": true,
  "provider": "tencent",
  "expiresIn": 300,
  "params": {
    "captchaAppId": "123456789",
    "aidEncrypted": "<本次生成的密文>",
    "aidEncryptedType": "cbc"
  },
  "sessionUuid": "<本次验证码会话>"
}
```

响应带有 `Cache-Control: no-store`。不要在代理、CDN、本地存储或多次验证之间复用参数响应。参数生成失败时接口返回错误，不创建 Redis 会话。

客户端继续动态加载官方 `TCaptcha.js`，将 `aidEncrypted` 和 `aidEncryptedType` 传入 `TencentCaptcha` 构造参数。

OIDC 页面提前准备一组参数、会话与 SDK 实例，缓存上限为 240 秒（按接口有效期预留 60 秒）。开始验证时独占领取该组状态；成功、取消、容灾失败后均清理实例。再次验证重新获取参数并创建实例。管理端每次触发都获取新参数。

同场景并发触发返回「验证码正在验证中」。旧实例的回调不会完成新实例的请求。加载异常、缺少密文和 `trerror_` 容灾票据不会作为成功结果提交业务接口；用户再次操作时重新申请参数。普通滑块交互由腾讯 SDK 处理，业务侧不自动循环重试。

## 发布与验收

先发布后端，再发布 OIDC 前端和管理端。旧前端不会自动使用新增密文；只有两端都更新后，才能完整支持已开启的两个腾讯安全开关。新前端遇到旧后端没有密文的响应会提示配置异常。

在两个安全开关保持开启的测试环境执行以下验收：

1. 检查参数响应包含密文、`cbc`、300 秒有效期及会话标识，且不包含任何密钥。连续两次响应的密文与会话标识不同。
2. 完成一次验证并提交登录；再次触发、取消后重试及前一次业务提交失败后重试，都能使用新的密文与实例完成验证。
3. 页面预加载后等待超过 240 秒再触发，确认重新获取参数；分别检查登录、注册、找回密码、短信和邮件场景。
4. 在浏览器 Network 中确认正常验证码初始化不再出现因缺失加密参数导致的 `403`；后端仍校验腾讯票据。
5. 在隔离测试页面尝试缺失密文及复用已成功使用的密文，确认腾讯拒绝；恢复获取新参数后能够验证。
6. 模拟腾讯 SDK 加载失败或容灾回调，确认页面提示失败并允许重新操作，没有继续提交登录或发送验证码请求。

本地测试使用模拟 SDK 和测试数据库，不能替代腾讯控制台实际开启一次一密后的联调。

## 本地检查

- 后端：`go test ./app/service/captcha`；`go test ./app/controller -run TestCaptchaParamsHTTPFreshEncryptionAndFailureBeforeSession`。
- OIDC 子仓库：`pnpm exec vitest run src/composables/__tests__/useCaptcha.spec.ts`、`pnpm exec vue-tsc -b`、`pnpm build`。
- 管理端子仓库：`pnpm exec vitest run apps/web-antd/src/composables/__tests__/useCaptcha.test.ts`、`pnpm --filter @vben/web-antd typecheck`、`pnpm --filter @vben/web-antd build`。

构建时可通过 `IAM_AUTH_OUT_DIR` 和 `IAM_WEB_OUT_DIR` 分别指定临时产物目录，避免覆盖正在使用的静态资源。
