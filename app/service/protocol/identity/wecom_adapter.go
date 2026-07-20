package identity

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const (
	wecomAuthURL          = "https://open.weixin.qq.com/connect/oauth2/authorize"
	wecomTokenURL         = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	wecomUserInfoURL      = "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo"
	wecomUserDetailURL    = "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserdetail"
	wecomDefaultScope     = "snsapi_base"
	wecomDefaultExpiresIn = 7200
)

type WeComAdapter struct {
	baseAdapter
}

func NewWeComAdapter(provider *Provider) *WeComAdapter {
	return &WeComAdapter{baseAdapter: newBaseAdapter(provider)}
}

func (a *WeComAdapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("企业微信 CorpID 不能为空")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("企业微信 Secret 不能为空")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("企业微信回调 URL 不能为空")
	}
	if getString(a.Provider.Attributes, "agentId", "agent_id") == "" {
		return fmt.Errorf("企业微信 AgentID 不能为空")
	}
	return nil
}

func (a *WeComAdapter) GetAuthURL(state string) string {
	scope := wecomDefaultScope
	if scopes := a.scopes(nil); len(scopes) > 0 {
		scope = scopes[0]
	}
	authURL := buildURL(endpoint(a.Provider.AuthURL, wecomAuthURL), url.Values{
		"appid":         {a.Provider.ClientID},
		"redirect_uri":  {a.Provider.RedirectURL},
		"response_type": {"code"},
		"scope":         {scope},
		"agentid":       {getString(a.Provider.Attributes, "agentId", "agent_id")},
		"state":         {state},
	})
	return authURL + "#wechat_redirect"
}

func (a *WeComAdapter) ExchangeToken(ctx context.Context, code string) (*Token, error) {
	tokenURL := buildURL(endpoint(a.Provider.TokenURL, wecomTokenURL), url.Values{
		"corpid":     {a.Provider.ClientID},
		"corpsecret": {a.Provider.ClientSecret},
	})
	raw, err := getJSON(ctx, a.Client, tokenURL, nil)
	if err != nil {
		return nil, err
	}
	if err := ensureSuccess(raw, "企业微信"); err != nil {
		return nil, err
	}
	accessToken := getString(raw, "access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("企业微信未返回 access_token")
	}
	expiresIn := int64Number(raw["expires_in"])
	if expiresIn == 0 {
		expiresIn = wecomDefaultExpiresIn
	}
	return &Token{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		Expiry:      time.Now().Add(time.Duration(expiresIn) * time.Second),
		Raw: map[string]any{
			"code":         code,
			"token_result": raw,
		},
	}, nil
}

func (a *WeComAdapter) GetUserInfo(ctx context.Context, token *Token) (*User, error) {
	code := getString(token.Raw, "code")
	if code == "" {
		return nil, fmt.Errorf("企业微信授权码缺失")
	}
	userInfoURL := buildURL(endpoint(a.Provider.UserInfoURL, wecomUserInfoURL), url.Values{
		"access_token": {token.AccessToken},
		"code":         {code},
	})
	raw, err := getJSON(ctx, a.Client, userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	if err := ensureSuccess(raw, "企业微信"); err != nil {
		return nil, err
	}

	detail := map[string]any{}
	if ticket := getString(raw, "user_ticket", "userTicket"); ticket != "" {
		if detailRaw, err := postJSON(ctx, a.Client, buildURL(wecomUserDetailURL, url.Values{
			"access_token": {token.AccessToken},
		}), map[string]any{"user_ticket": ticket}, nil); err == nil && ensureSuccess(detailRaw, "企业微信") == nil {
			detail = detailRaw
		}
	}

	merged := map[string]any{}
	for k, v := range raw {
		merged[k] = v
	}
	for k, v := range detail {
		merged[k] = v
	}

	externalID := getString(merged, "userid", "UserId", "open_userid", "OpenId", "openid")
	if externalID == "" {
		return nil, fmt.Errorf("企业微信未返回用户唯一标识")
	}
	return &User{
		ExternalUserID: externalID,
		UnionID:        getString(merged, "unionid", "union_id"),
		TenantID:       a.Provider.ClientID,
		Nickname:       getString(merged, "name", "nickname"),
		Avatar:         getString(merged, "avatar"),
		Email:          getString(merged, "email"),
		Phone:          getString(merged, "mobile"),
		RawUserInfo:    merged,
	}, nil
}
