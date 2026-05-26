-- +goose Up
-- +goose StatementBegin
INSERT INTO social_providers (
  name, identifier, type, description, client_id, client_secret,
  auth_url, token_url, user_info_url, redirect_url, scopes, icon,
  sort_order, status, is_default, attributes, created_at, updated_at, remark
) VALUES
  ('QQ 互联', 'qq', 'qq', 'QQ 互联 OAuth2 登录，填写 AppID/AppKey 后启用', '', '', '', '', '', '', 'get_user_info', '', 10, 0, FALSE, '{}', NOW(), NOW(), '系统预置模板：启用前请配置 QQ 互联应用信息'),
  ('微信网页扫码', 'wechat', 'wechat', '微信开放平台网站应用登录，支持 PC 扫码授权', '', '', '', '', '', '', 'snsapi_login', '', 20, 0, FALSE, '{}', NOW(), NOW(), '系统预置模板：启用前请配置微信开放平台 AppID/AppSecret'),
  ('微信小程序', 'wechat-miniprogram', 'wechat_miniprogram', '微信小程序 wx.login / 扫码登录', '', '', '', '', '', '', '', '', 30, 0, FALSE, '{}', NOW(), NOW(), '系统预置模板：启用前请配置小程序 AppID/AppSecret'),
  ('GitHub', 'github', 'github', 'GitHub OAuth 登录', '', '', '', '', '', '', 'user:email', '', 40, 0, FALSE, '{}', NOW(), NOW(), '系统预置模板：启用前请配置 GitHub OAuth App'),
  ('Google', 'google', 'google', 'Google OAuth / OpenID 登录', '', '', '', '', '', '', 'openid,profile,email', '', 50, 0, FALSE, '{}', NOW(), NOW(), '系统预置模板：启用前请配置 Google OAuth Client'),
  ('OpenID Connect', 'oidc', 'oidc', '标准 OIDC 登录，支持 issuer discovery', '', '', '', '', '', '', 'openid,profile,email', '', 60, 0, FALSE, '{}', NOW(), NOW(), '系统预置模板：启用前请配置 OIDC issuer 和 Client'),
  ('通用 OAuth2', 'generic-oauth2', 'generic', '通用 OAuth2 登录，可配置授权、令牌、用户信息端点和字段映射', '', '', '', '', '', '', '', '', 70, 0, FALSE, '{}', NOW(), NOW(), '系统预置模板：启用前请配置 OAuth2 端点和字段映射')
ON CONFLICT (identifier) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM social_providers sp
WHERE sp.identifier IN ('qq', 'wechat', 'wechat-miniprogram', 'github', 'google', 'oidc', 'generic-oauth2')
  AND sp.status = 0
  AND NOT EXISTS (
    SELECT 1 FROM user_social_accounts usa WHERE usa.provider_id = sp.id
  );
-- +goose StatementEnd
