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

// GitHub OAuth 固定端点。
const (
	githubAuthURL     = "https://github.com/login/oauth/authorize"
	githubTokenURL    = "https://github.com/login/oauth/access_token"
	githubUserInfoURL = "https://api.github.com/user"
	githubEmailsURL   = "https://api.github.com/user/emails"
	githubDefaultScp  = "user:email"
)

type GitHubAdapter struct {
	*GenericOAuth2Adapter
}

func NewGitHubAdapter(provider *model.SocialProvider) *GitHubAdapter {
	provider.AuthURL = githubAuthURL
	provider.TokenURL = githubTokenURL
	provider.UserInfoURL = githubUserInfoURL
	if provider.Scopes == "" {
		provider.Scopes = githubDefaultScp
	}
	return &GitHubAdapter{GenericOAuth2Adapter: NewGenericOAuth2Adapter(provider)}
}

func (a *GitHubAdapter) GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error) {
	user, err := a.GenericOAuth2Adapter.GetUserInfo(ctx, token)
	if err != nil {
		return nil, err
	}
	// GitHub 用户可能把主邮箱设为私密；此时 /user 的 email 为 null，需要额外请求 /user/emails。
	if user.Email == "" {
		if email, err := a.fetchPrimaryEmail(ctx, token); err == nil && email != "" {
			user.Email = email
		}
	}
	return user, nil
}

func (a *GitHubAdapter) fetchPrimaryEmail(ctx context.Context, token *oauth2.Token) (string, error) {
	client := a.Config.Client(ctx, token)
	resp, err := client.Get(githubEmailsURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github emails endpoint status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	return "", nil
}

func (a *GitHubAdapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("GitHub ClientID 不能为空")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("GitHub ClientSecret 不能为空")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("GitHub 回调 URL 不能为空")
	}
	return nil
}
