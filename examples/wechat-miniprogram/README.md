# IAM 原生小程序登录示例

1. 在 IAM 配置微信小程序身份连接和公开客户端，开启 `miniProgram`。
2. 修改 `config.js` 中的 IAM HTTPS 域名和应用 UUID；不要填写 AppSecret。
3. 使用微信开发者工具导入本目录，替换 `project.config.json` 的 `touristappid` 为真实小程序 AppID。
4. 在微信后台添加 IAM request 合法域名并开通实时手机号能力，使用真机调试。

页面支持手机号注册、已有账号绑定、资料补充、协议及授权确认、二次验证、刷新和退出。敏感凭证不显示在页面或日志。未完成登录的事务只放在内存中。

运行请求层测试：`node --test utils/iam.test.cjs`。

完整配置、API、错误处理和验证码扩展见 [对接文档](../../docs/微信小程序登录对接.md)。真实微信 code 及手机号授权不能由游客 AppID 或单元测试生成。
