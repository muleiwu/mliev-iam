package identity

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

const (
	dingTalkAuthURL     = "https://login.dingtalk.com/oauth2/auth"
	dingTalkTokenURL    = "https://api.dingtalk.com/v1.0/oauth2/userAccessToken"
	dingTalkUserInfoURL = "https://api.dingtalk.com/v1.0/contact/users/me"
	dingTalkDefaultScp  = "openid corpid"
)

type DingTalkAdapter struct {
	baseAdapter
}

func NewDingTalkAdapter(provider *model.IdentityProvider) *DingTalkAdapter {
	return &DingTalkAdapter{baseAdapter: newBaseAdapter(provider)}
}

func (a *DingTalkAdapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("钉钉 AppKey/ClientID 不能为空")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("钉钉 AppSecret/ClientSecret 不能为空")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("钉钉回调 URL 不能为空")
	}
	return nil
}

func (a *DingTalkAdapter) GetAuthURL(state string) string {
	scopes := a.scopes([]string{dingTalkDefaultScp})
	return buildURL(endpoint(a.Provider.AuthURL, dingTalkAuthURL), url.Values{
		"client_id":     {a.Provider.ClientID},
		"redirect_uri":  {a.Provider.RedirectURL},
		"state":         {state},
		"response_type": {"code"},
		"prompt":        {"consent"},
		"scope":         {joinScopes(scopes, " ")},
	})
}

func (a *DingTalkAdapter) ExchangeToken(ctx context.Context, code string) (*Token, error) {
	raw, err := postJSON(ctx, a.Client, endpoint(a.Provider.TokenURL, dingTalkTokenURL), map[string]any{
		"clientId":     a.Provider.ClientID,
		"clientSecret": a.Provider.ClientSecret,
		"code":         code,
		"grantType":    "authorization_code",
	}, nil)
	if err != nil {
		return nil, err
	}
	if err := ensureSuccess(raw, "钉钉"); err != nil {
		return nil, err
	}
	expiresIn := int64Number(raw["expireIn"])
	token := &Token{
		AccessToken:  getString(raw, "accessToken", "access_token"),
		RefreshToken: getString(raw, "refreshToken", "refresh_token"),
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		Raw:          raw,
	}
	if expiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	if token.AccessToken == "" {
		return nil, fmt.Errorf("钉钉未返回 accessToken")
	}
	return token, nil
}

func (a *DingTalkAdapter) GetUserInfo(ctx context.Context, token *Token) (*User, error) {
	raw, err := getJSON(ctx, a.Client, endpoint(a.Provider.UserInfoURL, dingTalkUserInfoURL), map[string]string{
		"x-acs-dingtalk-access-token": token.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	if err := ensureSuccess(raw, "钉钉"); err != nil {
		return nil, err
	}
	data := flattenData(raw)
	externalID := getString(data, "userid", "userId", "openId", "openid", "unionId", "unionid")
	if externalID == "" {
		return nil, fmt.Errorf("钉钉未返回用户唯一标识")
	}
	return &User{
		ExternalUserID: externalID,
		UnionID:        getString(data, "unionId", "unionid"),
		TenantID:       getString(token.Raw, "corpId", "corp_id"),
		Nickname:       getString(data, "nick", "name"),
		Avatar:         getString(data, "avatarUrl", "avatar"),
		Email:          getString(data, "email"),
		Phone:          getString(data, "mobile"),
		RawUserInfo:    raw,
	}, nil
}
