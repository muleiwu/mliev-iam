package social

import (
	"context"
	"strings"

	"golang.org/x/oauth2"
)

type ProviderType string

const (
	ProviderTypeGeneric           ProviderType = "generic"
	ProviderTypeWeChat            ProviderType = "wechat"
	ProviderTypeWeChatMiniProgram ProviderType = "wechat_miniprogram"
	ProviderTypeQQ                ProviderType = "qq"
	ProviderTypeAlipay            ProviderType = "alipay"
	ProviderTypeGithub            ProviderType = "github"
	ProviderTypeGoogle            ProviderType = "google"
	ProviderTypeOIDC              ProviderType = "oidc"
)

// Provider is the authentication adapter input. It deliberately has no GORM
// metadata and cannot read the retired social_providers table at runtime.
type Provider struct {
	ID           uint
	Name         string
	Identifier   string
	Type         ProviderType
	Description  string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	RedirectURL  string
	Scopes       string
	Icon         string
	SortOrder    int
	IsDefault    bool
	Attributes   map[string]any
	Remark       string
}

func (p *Provider) GetScopes() []string {
	if p == nil || strings.TrimSpace(p.Scopes) == "" {
		return nil
	}
	parts := strings.Split(p.Scopes, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

// SocialUser 标准化的社交用户信息。
// OpenID 是第三方平台内的用户唯一标识；UnionID 在同主体（如微信开放平台）下跨应用一致。
type SocialUser struct {
	OpenID      string
	UnionID     string
	Nickname    string
	Avatar      string
	Email       string
	Phone       string
	RawUserInfo map[string]any
}

// Adapter 所有社交登录提供商适配器的最小接口。
// 具体的登录能力（浏览器授权 / 后端 code 交换）通过下方的能力接口暴露。
type Adapter interface {
	GetProviderType() ProviderType
	ValidateConfig() error
}

// BrowserFlowAdapter 标准 OAuth2 浏览器授权流适配器。
// 适用：GitHub / Google / OIDC / QQ / 微信网页。
type BrowserFlowAdapter interface {
	Adapter
	GetAuthURL(state string) string
	ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error)
	GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error)
}

// CodeFlowAdapter 非浏览器 code 流适配器。
// 适用：微信小程序（wx.login -> js_code -> jscode2session）。
type CodeFlowAdapter interface {
	Adapter
	LoginWithCode(ctx context.Context, code string) (*SocialUser, error)
}
