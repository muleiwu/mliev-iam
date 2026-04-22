package protocol

import (
	"context"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/social"
	"github.com/redis/go-redis/v9"
)

// 为保持向后兼容，这里把 social 子包里的类型重导出为当前包内的别名，
// 让外部调用方既能继续用 protocol.SocialUser / protocol.BrowserFlowAdapter 等名字，
// 也能直接从 social 子包引入。
type SocialUser = social.SocialUser

type SocialProviderAdapter = social.Adapter

type BrowserFlowAdapter = social.BrowserFlowAdapter

type CodeFlowAdapter = social.CodeFlowAdapter

// oidcResolverAdapter 把外层 OIDCDiscoveryCache 包装成 social.OIDCResolver 形状，
// 避免 social 子包反向依赖 protocol 包造成循环。
type oidcResolverAdapter struct {
	cache *OIDCDiscoveryCache
}

func (r *oidcResolverAdapter) Resolve(ctx context.Context, issuer string) (*social_OIDCDoc, error) {
	doc, err := r.cache.Resolve(ctx, issuer)
	if err != nil {
		return nil, err
	}
	return &social_OIDCDoc{
		Issuer:                doc.Issuer,
		AuthorizationEndpoint: doc.AuthorizationEndpoint,
		TokenEndpoint:         doc.TokenEndpoint,
		UserinfoEndpoint:      doc.UserinfoEndpoint,
		JwksURI:               doc.JwksURI,
	}, nil
}

// 用 type alias 让 social 子包里定义的 struct_OIDCDoc 可以在这里用一个可读名字。
type social_OIDCDoc = social.OIDCDoc //nolint:revive

// NewSocialProviderAdapter 按 provider.Type 返回对应适配器。
// redisClient 用于 OIDC discovery 缓存；传 nil 时 OIDC 依赖 provider 已手填的端点。
func NewSocialProviderAdapter(provider *model.SocialProvider, redisClient *redis.Client) (SocialProviderAdapter, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider 不能为空")
	}

	switch provider.Type {
	case model.ProviderTypeGithub:
		return social.NewGitHubAdapter(provider), nil
	case model.ProviderTypeGoogle:
		return social.NewGoogleAdapter(provider), nil
	case model.ProviderTypeQQ:
		return social.NewQQAdapter(provider), nil
	case model.ProviderTypeWeChat:
		return social.NewWeChatAdapter(provider), nil
	case model.ProviderTypeWeChatMiniProgram:
		return social.NewWeChatMiniProgramAdapter(provider), nil
	case model.ProviderTypeOIDC:
		var resolver social.OIDCResolver
		if redisClient != nil {
			resolver = &oidcResolverAdapter{cache: NewOIDCDiscoveryCache(redisClient)}
		}
		return social.NewOIDCAdapter(provider, resolver)
	case model.ProviderTypeGeneric, model.ProviderTypeAlipay:
		// 历史枚举：沿用通用 OAuth2 实现（Alipay 作为简单 OAuth2 占位）。
		return social.NewGenericOAuth2Adapter(provider), nil
	default:
		return social.NewGenericOAuth2Adapter(provider), nil
	}
}
