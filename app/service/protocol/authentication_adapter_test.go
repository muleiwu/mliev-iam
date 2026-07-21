package protocol

import (
	"context"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/identity"
	"github.com/stretchr/testify/require"
)

type staticIdentityAdapter struct{ user *identity.User }

func (*staticIdentityAdapter) GetProviderType() identity.ProviderType { return identity.ProviderTypeFeishu }
func (*staticIdentityAdapter) ValidateConfig() error                  { return nil }
func (*staticIdentityAdapter) GetAuthURL(string) string               { return "" }
func (*staticIdentityAdapter) ExchangeToken(context.Context, string) (*identity.Token, error) {
	return &identity.Token{}, nil
}
func (a *staticIdentityAdapter) GetUserInfo(context.Context, *identity.Token) (*identity.User, error) {
	return a.user, nil
}

func TestEnterpriseAuthenticationProfileIncludesProviderAliases(t *testing.T) {
	tests := []struct {
		providerType string
		user         *identity.User
		namespace    model.IdentifierNamespace
		alias        string
	}{
		{
			providerType: "feishu",
			user: &identity.User{ExternalUserID: "open-a", RawUserInfo: map[string]any{
				"data": map[string]any{"open_id": "open-a", "user_id": "user-a"},
			}},
			namespace: model.IdentifierNamespaceOpenID, alias: "open-a",
		},
		{
			providerType: "wecom",
			user: &identity.User{ExternalUserID: "open-b", RawUserInfo: map[string]any{
				"open_userid": "open-b", "userid": "user-b",
			}},
			namespace: model.IdentifierNamespaceOpenUserID, alias: "open-b",
		},
		{
			providerType: "dingtalk",
			user: &identity.User{ExternalUserID: "user-c", RawUserInfo: map[string]any{
				"openId": "open-c", "userId": "user-c",
			}},
			namespace: model.IdentifierNamespaceOpenID, alias: "open-c",
		},
	}
	for _, test := range tests {
		t.Run(test.providerType, func(t *testing.T) {
			adapter := &enterpriseAuthenticationAdapter{adapter: &staticIdentityAdapter{user: test.user}, providerType: test.providerType}
			profile, err := adapter.FetchProfile(context.Background(), &AuthToken{value: &identity.Token{}})
			require.NoError(t, err)
			require.True(t, hasProfileIdentifier(profile, model.IdentifierNamespaceLoginSubject, test.user.ExternalUserID, true))
			require.True(t, hasProfileIdentifier(profile, test.namespace, test.alias, false))
		})
	}
}

func hasProfileIdentifier(profile *ExternalProfile, namespace model.IdentifierNamespace, value string, primary bool) bool {
	for _, identifier := range profile.Identifiers {
		if identifier.Namespace == namespace && identifier.Value == value && identifier.Primary == primary {
			return true
		}
	}
	return false
}
