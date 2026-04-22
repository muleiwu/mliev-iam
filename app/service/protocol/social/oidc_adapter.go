package social

import (
	"context"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"golang.org/x/oauth2"
)

const oidcDefaultScp = "openid,profile,email"

// OIDCResolver 抽象 OIDC discovery 解析能力，避免 social 子包反向依赖外层 protocol 包。
type OIDCResolver interface {
	Resolve(ctx context.Context, issuer string) (*OIDCDoc, error)
}

// 为了避免从 protocol 包导入造成循环依赖，这里定义一个兼容的结构，
// 外层 protocol.OIDCDiscoveryDoc 的字段需与此一一对应（由 factory 适配层负责转换）。
type OIDCDoc struct {
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserinfoEndpoint      string
	JwksURI               string
}

// OIDCAdapter 标准 OpenID Connect 适配器。
// 构造时若 Attributes["issuer"] 存在且 discovery 可用，端点会被自动填充。
type OIDCAdapter struct {
	*GenericOAuth2Adapter
	Issuer string
}

// NewOIDCAdapter 创建 OIDC 适配器。
// resolver 可为 nil；此时若 provider 已手工填写 Auth/Token/UserInfo URL 则仍可工作。
func NewOIDCAdapter(provider *model.SocialProvider, resolver OIDCResolver) (*OIDCAdapter, error) {
	issuer, _ := getAttrString(provider.Attributes, "issuer")

	if resolver != nil && issuer != "" {
		doc, err := resolver.Resolve(context.Background(), issuer)
		if err == nil && doc != nil {
			if doc.AuthorizationEndpoint != "" {
				provider.AuthURL = doc.AuthorizationEndpoint
			}
			if doc.TokenEndpoint != "" {
				provider.TokenURL = doc.TokenEndpoint
			}
			if doc.UserinfoEndpoint != "" {
				provider.UserInfoURL = doc.UserinfoEndpoint
			}
		}
	}
	if provider.Scopes == "" {
		provider.Scopes = oidcDefaultScp
	}

	return &OIDCAdapter{
		GenericOAuth2Adapter: NewGenericOAuth2Adapter(provider),
		Issuer:               issuer,
	}, nil
}

func (a *OIDCAdapter) GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error) {
	user, err := a.GenericOAuth2Adapter.GetUserInfo(ctx, token)
	if err != nil {
		return nil, err
	}
	// OIDC 的 OpenID 应来自 sub；MapCommonUserFields 已处理 id/sub 的兜底，但这里确保优先 sub。
	if sub, ok := user.RawUserInfo["sub"]; ok {
		user.OpenID = stringifyID(sub)
	}
	return user, nil
}

func (a *OIDCAdapter) ValidateConfig() error {
	if a.Issuer == "" {
		return fmt.Errorf("OIDC issuer 不能为空（请在 Attributes.issuer 填写）")
	}
	if a.Provider.ClientID == "" {
		return fmt.Errorf("OIDC ClientID 不能为空")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("OIDC ClientSecret 不能为空")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("OIDC 回调 URL 不能为空")
	}
	if a.Provider.AuthURL == "" || a.Provider.TokenURL == "" {
		return fmt.Errorf("OIDC 端点缺失：请确认 issuer 的 discovery 文档可访问")
	}
	return nil
}

func getAttrString(attrs map[string]any, key string) (string, bool) {
	if attrs == nil {
		return "", false
	}
	v, ok := attrs[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
