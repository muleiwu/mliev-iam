# 极验验证 4.0 接入

IAM 已内置 geetest 验证码 Provider，Web 登录端和管理端都支持。极验的 Captcha ID 会下发给浏览器，Captcha Key 只保存在服务端配置中，不会出现在客户端响应或前端代码里。

## 配置

进入管理后台的「系统设置 -> 验证码设置」，新增验证码配置：

1. 验证码提供商选择「极验验证 4.0」
2. 填写极验后台的 Captcha ID
3. 填写极验后台的 Captcha Key
4. 保存后，在「场景绑定」中把需要防护的场景绑定到该配置

支持绑定的场景：login、register、resetPassword、sendSms、sendEmail。

未绑定或配置停用时，验证码会按现有规则直接放行。

## 校验流程

1. 浏览器在页面加载时请求 /api/captcha/params 并提前初始化极验组件。
2. 用户触发高风险操作后，前端调用 showCaptcha()。
3. 极验验证成功后，前端只提交 lot_number、captcha_output、pass_token、gen_time 四个字段，外加设备指纹和 IAM 会话 ID。
4. 服务端用 Captcha Key 对 lot_number 做 HMAC-SHA256，生成 sign_token。
5. 服务端调用 https://gcaptcha4.geetest.com/validate。
6. 只有极验返回 result=success 时才放行业务；result=fail 和 status=error 都会拒绝。

业务接口仍然使用现有的 captchaTicket 字段，不需要单独为极验新增接口：

```json
{
  "captchaTicket": {
    "provider": "geetest",
    "data": {
      "lot_number": "...",
      "captcha_output": "...",
      "pass_token": "...",
      "gen_time": "...",
      "fingerprint": "..."
    },
    "sessionUuid": "..."
  }
}
```

## 安全约束

- sign_token 仅在服务端生成，使用 HMAC-SHA256(captchaKey, lot_number)。
- 前端代码中不能出现 captchaKey。
- 极验服务端接口超时、HTTP 非 200 或 status=error 时，业务请求会被拒绝。
- IAM 的 sessionUuid 仍然一次性使用，并与设备指纹绑定。
- 重复提交同一组极验凭证不会绕过 IAM 会话校验。

## 相关代码

- app/service/captcha/geetest.go：极验 Provider、签名和 validate 调用
- app/service/captcha/manager.go：注册 geetest Provider
- frontend-oidc/src/composables/useCaptcha.ts：登录、注册、找回密码及发码场景适配
- admin-web/apps/web-antd/src/composables/useCaptcha.ts：管理端登录适配

## 自动化测试

### 离线测试（默认，无外部依赖）

```powershell
# 后端：Provider、签名、HTTP 异常、配置错误、注册信息
$env:GOPROXY="https://goproxy.cn,direct"
go test ./app/service/captcha -run TestGeetest -count=1

# OIDC 登录端：Provider 分派、凭证读取顺序、回调错误、并发控制
pnpm --dir frontend-oidc test -- src/composables/__tests__/useCaptcha.spec.ts

# 管理端：极验/腾讯 Provider 分支与场景开关
pnpm --dir admin-web exec vitest run --dom apps/web-antd/src/composables/useCaptcha.test.ts
```

### 真实极验环境测试（手动开启）

极验凭证是一次性的，且 lot_number、pass_token 会过期，因此真实联调测试默认跳过。先在浏览器中完成一次验证，从 getValidate() 拿到最新四个字段，再立即执行：

```powershell
$env:GEETEST_INTEGRATION="1"
$env:GEETEST_CAPTCHA_ID="你的 Captcha ID"
$env:GEETEST_CAPTCHA_KEY="你的 Captcha Key"
$env:GEETEST_LOT_NUMBER="本次验证的 lot_number"
$env:GEETEST_CAPTCHA_OUTPUT="本次验证的 captcha_output"
$env:GEETEST_PASS_TOKEN="本次验证的 pass_token"
$env:GEETEST_GEN_TIME="本次验证的 gen_time"
go test ./app/service/captcha -run TestGeetestProviderLiveValidateEnvironment -count=1 -v
```

真实凭证不要写入仓库、Issue 或日志。测试再次运行前必须重新完成一次浏览器验证并更新四个凭证字段。