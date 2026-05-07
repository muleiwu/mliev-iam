package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"golang.org/x/oauth2"
)

// GenericOAuth2Adapter 标准 OAuth2.0 流程的通用实现，作为其他浏览器流适配器的内嵌基类。
type GenericOAuth2Adapter struct {
	Provider *model.SocialProvider
	Config   *oauth2.Config
}

func NewGenericOAuth2Adapter(provider *model.SocialProvider) *GenericOAuth2Adapter {
	return &GenericOAuth2Adapter{
		Provider: provider,
		Config: &oauth2.Config{
			ClientID:     provider.ClientID,
			ClientSecret: provider.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  provider.AuthURL,
				TokenURL: provider.TokenURL,
			},
			RedirectURL: provider.RedirectURL,
			Scopes:      provider.GetScopes(),
		},
	}
}

func (a *GenericOAuth2Adapter) GetProviderType() model.ProviderType {
	return a.Provider.Type
}

func (a *GenericOAuth2Adapter) GetAuthURL(state string) string {
	return a.Config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (a *GenericOAuth2Adapter) ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error) {
	return a.Config.Exchange(ctx, code)
}

func (a *GenericOAuth2Adapter) GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error) {
	if a.Provider.UserInfoURL == "" {
		return nil, fmt.Errorf("user info URL not configured")
	}

	client := a.Config.Client(ctx, token)
	resp, err := client.Get(a.Provider.UserInfoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}

	return MapCommonUserFields(raw), nil
}

func (a *GenericOAuth2Adapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("client ID is required")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("client secret is required")
	}
	if a.Provider.AuthURL == "" {
		return fmt.Errorf("auth URL is required")
	}
	if a.Provider.TokenURL == "" {
		return fmt.Errorf("token URL is required")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("redirect URL is required")
	}
	return nil
}

// MapCommonUserFields 从原始用户信息里抽取常见字段，各类 OAuth2 适配器共用。
func MapCommonUserFields(raw map[string]any) *SocialUser {
	u := &SocialUser{RawUserInfo: raw}

	if id, ok := raw["id"]; ok {
		u.OpenID = stringifyID(id)
	} else if sub, ok := raw["sub"]; ok {
		u.OpenID = stringifyID(sub)
	}

	if name, ok := raw["name"]; ok {
		u.Nickname = fmt.Sprintf("%v", name)
	} else if login, ok := raw["login"]; ok {
		u.Nickname = fmt.Sprintf("%v", login)
	}

	if avatar, ok := raw["avatar_url"]; ok {
		u.Avatar = fmt.Sprintf("%v", avatar)
	} else if picture, ok := raw["picture"]; ok {
		u.Avatar = fmt.Sprintf("%v", picture)
	}

	if email, ok := raw["email"]; ok {
		u.Email = fmt.Sprintf("%v", email)
	}

	return u
}

func stringifyID(v any) string {
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%.0f", x)
	case int, int64:
		return fmt.Sprintf("%d", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
