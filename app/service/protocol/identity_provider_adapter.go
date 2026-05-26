package protocol

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/identity"
)

type IdentityUser = identity.User
type IdentityToken = identity.Token
type IdentityProviderAdapter = identity.Adapter

func NewIdentityProviderAdapter(provider *model.IdentityProvider) (IdentityProviderAdapter, error) {
	if provider == nil {
		return nil, fmt.Errorf("identity provider 不能为空")
	}

	switch provider.Type {
	case model.IdentityProviderTypeFeishu:
		return identity.NewFeishuAdapter(provider), nil
	case model.IdentityProviderTypeWeCom:
		return identity.NewWeComAdapter(provider), nil
	case model.IdentityProviderTypeDingTalk:
		return identity.NewDingTalkAdapter(provider), nil
	default:
		return nil, fmt.Errorf("不支持的身份源类型: %s", provider.Type)
	}
}
