package social

import (
	"fmt"
)

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
	googleDefaultScp  = "openid,profile,email"
)

type GoogleAdapter struct {
	*GenericOAuth2Adapter
}

func NewGoogleAdapter(provider *Provider) *GoogleAdapter {
	provider.AuthURL = googleAuthURL
	provider.TokenURL = googleTokenURL
	provider.UserInfoURL = googleUserInfoURL
	if provider.Scopes == "" {
		provider.Scopes = googleDefaultScp
	}
	return &GoogleAdapter{GenericOAuth2Adapter: NewGenericOAuth2Adapter(provider)}
}

func (a *GoogleAdapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("Google ClientID 不能为空")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("Google ClientSecret 不能为空")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("Google 回调 URL 不能为空")
	}
	return nil
}
