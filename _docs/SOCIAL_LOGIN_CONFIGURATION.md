# 社会化登录配置指南

本系统会预置 7 个禁用状态的社会化登录模板：QQ、微信网页扫码、微信小程序、GitHub、Google、OpenID Connect、通用 OAuth2。模板不会写入真实密钥，管理员需要在管理台填写应用信息后再启用。

## 回调地址

浏览器 OAuth/OIDC 登录统一使用：

```text
https://<IAM 域名>/social/callback/<提供商标识符>
```

示例：

```text
https://iam.example.com/social/callback/qq
https://iam.example.com/social/callback/wechat
https://iam.example.com/social/callback/github
```

微信小程序不使用浏览器回调。PC 扫码登录会由 IAM 创建小程序码，小程序端扫码后调用 `/social/miniprogram/qrcode/scan` 和 `/social/miniprogram/qrcode/confirm`。

## 启用前检查

- Provider 必须保持唯一标识符；标识符会出现在回调 URL 中。
- 启用前必须填写对应平台的 Client ID / Secret；禁用状态允许留空。
- 浏览器 OAuth/OIDC 类型必须填写回调 URL，并在第三方平台控制台登记完全一致的地址。
- 通用 OAuth2 必须填写授权 URL、Token URL、用户信息 URL。用户字段映射可留空，后端会按常见字段名兜底：`id/sub/openid/open_id`、`name/login/nickname`、`avatar_url/picture/avatar/headimgurl`、`email/mail`。

## 平台要点

- QQ 互联：创建网站应用，登记回调地址，填写 AppID 和 AppKey。后端会按 QQ 官方流程先获取 openid，再调用 `get_user_info`。
- 微信网页扫码：创建微信开放平台网站应用，scope 默认 `snsapi_login`，填写 AppID、AppSecret 和回调地址。
- 微信小程序：填写小程序 AppID、AppSecret；如使用 PC 小程序码扫码登录，需要小程序中实现确认页并传回 ticket、js_code。
- GitHub：创建 OAuth App，scope 默认 `user:email`，私密邮箱会额外调用邮箱接口兜底。
- Google：创建 OAuth Client，scope 默认 `openid,profile,email`。
- OpenID Connect：填写 Issuer、Client ID、Client Secret、回调 URL；管理台可预览 discovery 文档。
- 通用 OAuth2：用于兼容标准授权码流程的平台。若用户信息字段不是常见字段名，请填写字段映射。

## 官方参考

- [QQ 授权码换取 access_token](https://wiki.connect.qq.com/%E4%BD%BF%E7%94%A8authorization_code%E8%8E%B7%E5%8F%96access_token)
- [QQ get_user_info](https://wiki.connect.qq.com/get_user_info)
- [微信网站应用登录](https://developers.weixin.qq.com/doc/oplatform/Website_App/WeChat_Login/Wechat_Login.html)
- [微信小程序 code2Session](https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/user-login/code2Session.html)
