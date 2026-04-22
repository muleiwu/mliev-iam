package social

import (
	"context"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"golang.org/x/oauth2"
)

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
	GetProviderType() model.ProviderType
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
