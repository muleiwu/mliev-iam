package identity

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

const (
	feishuAuthURL     = "https://accounts.feishu.cn/open-apis/authen/v1/authorize"
	feishuTokenURL    = "https://open.feishu.cn/open-apis/authen/v2/oauth/token"
	feishuUserInfoURL = "https://open.feishu.cn/open-apis/authen/v1/user_info"
	feishuDefaultScp  = "contact:contact.base:readonly"
)

type FeishuAdapter struct {
	baseAdapter
}

func NewFeishuAdapter(provider *model.IdentityProvider) *FeishuAdapter {
	return &FeishuAdapter{baseAdapter: newBaseAdapter(provider)}
}

func (a *FeishuAdapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("飞书 App ID 不能为空")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("飞书 App Secret 不能为空")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("飞书回调 URL 不能为空")
	}
	return nil
}

func (a *FeishuAdapter) GetAuthURL(state string) string {
	scopes := a.scopes([]string{feishuDefaultScp})
	return buildURL(endpoint(a.Provider.AuthURL, feishuAuthURL), url.Values{
		"client_id":     {a.Provider.ClientID},
		"response_type": {"code"},
		"redirect_uri":  {a.Provider.RedirectURL},
		"scope":         {joinScopes(scopes, " ")},
		"state":         {state},
	})
}

func (a *FeishuAdapter) ExchangeToken(ctx context.Context, code string) (*Token, error) {
	raw, err := postJSON(ctx, a.Client, endpoint(a.Provider.TokenURL, feishuTokenURL), map[string]any{
		"grant_type":    "authorization_code",
		"client_id":     a.Provider.ClientID,
		"client_secret": a.Provider.ClientSecret,
		"code":          code,
		"redirect_uri":  a.Provider.RedirectURL,
	}, nil)
	if err != nil {
		return nil, err
	}
	if err := ensureSuccess(raw, "飞书"); err != nil {
		return nil, err
	}

	data := flattenData(raw)
	expiresIn := int64Number(data["expires_in"])
	token := &Token{
		AccessToken:  getString(data, "access_token"),
		RefreshToken: getString(data, "refresh_token"),
		TokenType:    getString(data, "token_type"),
		ExpiresIn:    expiresIn,
		Raw:          raw,
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if expiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	if token.AccessToken == "" {
		return nil, fmt.Errorf("飞书未返回 access_token")
	}
	return token, nil
}

func (a *FeishuAdapter) GetUserInfo(ctx context.Context, token *Token) (*User, error) {
	raw, err := getJSON(ctx, a.Client, endpoint(a.Provider.UserInfoURL, feishuUserInfoURL), map[string]string{
		"Authorization": "Bearer " + token.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	if err := ensureSuccess(raw, "飞书"); err != nil {
		return nil, err
	}
	data := flattenData(raw)
	user := &User{
		ExternalUserID: getString(data, "open_id", "sub", "user_id"),
		UnionID:        getString(data, "union_id"),
		TenantID:       getString(data, "tenant_key", "tenant_id"),
		Nickname:       getString(data, "name", "en_name"),
		Avatar:         getString(data, "avatar_url", "picture", "avatar_thumb"),
		Email:          getString(data, "email", "enterprise_email"),
		Phone:          getString(data, "mobile", "phone"),
		RawUserInfo:    raw,
	}
	if user.ExternalUserID == "" {
		return nil, fmt.Errorf("飞书未返回用户唯一标识")
	}
	return user, nil
}
