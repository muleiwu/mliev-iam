package protocol

import (
	"context"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/identity"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/social"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

type AuthToken struct{ value any }

type ExternalProfileIdentifier struct {
	Namespace model.IdentifierNamespace
	Value     string
	Primary   bool
}

type ExternalProfile struct {
	DisplayName string
	Avatar      string
	Email       string
	Phone       string
	Identifiers []ExternalProfileIdentifier
	Raw         map[string]any
}

type AuthenticationAdapter interface {
	ValidateConfig() error
	AuthorizationURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*AuthToken, error)
	FetchProfile(ctx context.Context, token *AuthToken) (*ExternalProfile, error)
}

type CodeAuthenticationAdapter interface {
	AuthenticationAdapter
	LoginWithCode(ctx context.Context, code string) (*ExternalProfile, error)
}

func NewAuthenticationAdapter(connection *model.IdentityConnection, credentials map[string]any, redisClient *redis.Client) (AuthenticationAdapter, error) {
	if connection == nil {
		return nil, fmt.Errorf("identity connection 不能为空")
	}
	if !connection.HasCapability(model.ConnectionCapabilityAuthentication) {
		return nil, fmt.Errorf("连接 %s 未启用 authentication 能力", connection.Identifier)
	}
	switch connection.Category {
	case model.ConnectionCategorySocial:
		provider := socialProviderFromConnection(connection, credentials)
		adapter, err := NewSocialProviderAdapter(provider, redisClient)
		if err != nil {
			return nil, err
		}
		if browser, ok := adapter.(BrowserFlowAdapter); ok {
			return &socialBrowserAuthenticationAdapter{adapter: browser}, nil
		}
		if code, ok := adapter.(CodeFlowAdapter); ok {
			return &socialCodeAuthenticationAdapter{adapter: code}, nil
		}
		return nil, fmt.Errorf("社交连接 %s 没有可用认证流程", connection.Identifier)
	case model.ConnectionCategoryEnterprise:
		provider := identityProviderFromConnection(connection, credentials)
		adapter, err := NewIdentityProviderAdapter(provider)
		if err != nil {
			return nil, err
		}
		return &enterpriseAuthenticationAdapter{adapter: adapter, providerType: connection.ProviderType}, nil
	default:
		return nil, fmt.Errorf("不支持的连接类别 %q", connection.Category)
	}
}

type socialBrowserAuthenticationAdapter struct{ adapter BrowserFlowAdapter }

func (a *socialBrowserAuthenticationAdapter) ValidateConfig() error {
	return a.adapter.ValidateConfig()
}
func (a *socialBrowserAuthenticationAdapter) AuthorizationURL(state string) string {
	return a.adapter.GetAuthURL(state)
}
func (a *socialBrowserAuthenticationAdapter) ExchangeCode(ctx context.Context, code string) (*AuthToken, error) {
	token, err := a.adapter.ExchangeToken(ctx, code)
	if err != nil {
		return nil, err
	}
	return &AuthToken{value: token}, nil
}
func (a *socialBrowserAuthenticationAdapter) FetchProfile(ctx context.Context, token *AuthToken) (*ExternalProfile, error) {
	oauthToken, ok := tokenValue[*oauth2.Token](token)
	if !ok {
		return nil, fmt.Errorf("社交认证令牌类型无效")
	}
	user, err := a.adapter.GetUserInfo(ctx, oauthToken)
	if err != nil {
		return nil, err
	}
	return externalProfileFromSocialUser(user), nil
}

type socialCodeAuthenticationAdapter struct{ adapter CodeFlowAdapter }

func (a *socialCodeAuthenticationAdapter) ValidateConfig() error        { return a.adapter.ValidateConfig() }
func (*socialCodeAuthenticationAdapter) AuthorizationURL(string) string { return "" }
func (*socialCodeAuthenticationAdapter) ExchangeCode(context.Context, string) (*AuthToken, error) {
	return nil, fmt.Errorf("该连接不支持浏览器授权码交换")
}
func (*socialCodeAuthenticationAdapter) FetchProfile(context.Context, *AuthToken) (*ExternalProfile, error) {
	return nil, fmt.Errorf("该连接不支持浏览器资料读取")
}
func (a *socialCodeAuthenticationAdapter) LoginWithCode(ctx context.Context, code string) (*ExternalProfile, error) {
	user, err := a.adapter.LoginWithCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return externalProfileFromSocialUser(user), nil
}

type enterpriseAuthenticationAdapter struct {
	adapter      IdentityProviderAdapter
	providerType string
}

func (a *enterpriseAuthenticationAdapter) ValidateConfig() error { return a.adapter.ValidateConfig() }
func (a *enterpriseAuthenticationAdapter) AuthorizationURL(state string) string {
	return a.adapter.GetAuthURL(state)
}
func (a *enterpriseAuthenticationAdapter) ExchangeCode(ctx context.Context, code string) (*AuthToken, error) {
	token, err := a.adapter.ExchangeToken(ctx, code)
	if err != nil {
		return nil, err
	}
	return &AuthToken{value: token}, nil
}
func (a *enterpriseAuthenticationAdapter) FetchProfile(ctx context.Context, token *AuthToken) (*ExternalProfile, error) {
	identityToken, ok := tokenValue[*IdentityToken](token)
	if !ok {
		return nil, fmt.Errorf("企业认证令牌类型无效")
	}
	user, err := a.adapter.GetUserInfo(ctx, identityToken)
	if err != nil {
		return nil, err
	}
	identifiers := []ExternalProfileIdentifier{{Namespace: model.IdentifierNamespaceLoginSubject, Value: user.ExternalUserID, Primary: true}}
	appendIdentifier := func(namespace model.IdentifierNamespace, value string) {
		if value == "" {
			return
		}
		identifiers = append(identifiers, ExternalProfileIdentifier{Namespace: namespace, Value: value})
	}
	switch a.providerType {
	case "feishu", "lark":
		appendIdentifier(model.IdentifierNamespaceOpenID, nestedRawString(user.RawUserInfo, "open_id"))
		appendIdentifier(model.IdentifierNamespaceLoginSubject, nestedRawString(user.RawUserInfo, "user_id"))
	case "wecom", "wechat_work":
		appendIdentifier(model.IdentifierNamespaceOpenUserID, nestedRawString(user.RawUserInfo, "open_userid", "OpenId"))
		appendIdentifier(model.IdentifierNamespaceLoginSubject, nestedRawString(user.RawUserInfo, "userid", "UserId"))
	case "dingtalk":
		appendIdentifier(model.IdentifierNamespaceOpenID, nestedRawString(user.RawUserInfo, "openId", "openid"))
		appendIdentifier(model.IdentifierNamespaceLoginSubject, nestedRawString(user.RawUserInfo, "userid", "userId"))
	}
	if user.UnionID != "" {
		identifiers = append(identifiers, ExternalProfileIdentifier{Namespace: model.IdentifierNamespaceUnionID, Value: user.UnionID})
	}
	return &ExternalProfile{
		DisplayName: user.Nickname, Avatar: user.Avatar, Email: user.Email, Phone: user.Phone,
		Identifiers: identifiers, Raw: user.RawUserInfo,
	}, nil
}

func nestedRawString(raw map[string]any, keys ...string) string {
	for _, source := range []map[string]any{raw, nestedRawMap(raw, "data"), nestedRawMap(raw, "result")} {
		for _, key := range keys {
			if value, ok := source[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return ""
}

func nestedRawMap(raw map[string]any, key string) map[string]any {
	if raw == nil {
		return nil
	}
	value, _ := raw[key].(map[string]any)
	return value
}

func tokenValue[T any](token *AuthToken) (T, bool) {
	var zero T
	if token == nil {
		return zero, false
	}
	value, ok := token.value.(T)
	return value, ok
}

func externalProfileFromSocialUser(user *SocialUser) *ExternalProfile {
	identifiers := []ExternalProfileIdentifier{
		{Namespace: model.IdentifierNamespaceLoginSubject, Value: user.OpenID, Primary: true},
		{Namespace: model.IdentifierNamespaceOpenID, Value: user.OpenID},
	}
	if user.UnionID != "" {
		identifiers = append(identifiers, ExternalProfileIdentifier{Namespace: model.IdentifierNamespaceUnionID, Value: user.UnionID})
	}
	return &ExternalProfile{
		DisplayName: user.Nickname, Avatar: user.Avatar, Email: user.Email, Phone: user.Phone,
		Identifiers: identifiers, Raw: user.RawUserInfo,
	}
}

func socialProviderFromConnection(connection *model.IdentityConnection, credentials map[string]any) *social.Provider {
	config := connection.Config
	return &social.Provider{
		ID: connection.ID, Name: connection.Name, Identifier: connection.Identifier,
		Type: social.ProviderType(connection.ProviderType), Description: connection.Description,
		ClientID: valueString(credentials, config, "clientId"), ClientSecret: valueString(credentials, config, "clientSecret"),
		AuthURL: valueString(nil, config, "authUrl"), TokenURL: valueString(nil, config, "tokenUrl"),
		UserInfoURL: valueString(nil, config, "userInfoUrl"), RedirectURL: valueString(nil, config, "redirectUrl"),
		Scopes: valueString(nil, config, "scopes"), Icon: connection.Icon, SortOrder: connection.SortOrder,
		IsDefault: connection.IsDefault, Attributes: cloneConfig(config),
		Remark: connection.Remark,
	}
}

func identityProviderFromConnection(connection *model.IdentityConnection, credentials map[string]any) *identity.Provider {
	config := connection.Config
	return &identity.Provider{
		ID: connection.ID, Name: connection.Name, Identifier: connection.Identifier,
		Type: identity.ProviderType(connection.ProviderType), Description: connection.Description,
		ClientID: valueString(credentials, config, "clientId"), ClientSecret: valueString(credentials, config, "clientSecret"),
		AuthURL: valueString(nil, config, "authUrl"), TokenURL: valueString(nil, config, "tokenUrl"),
		UserInfoURL: valueString(nil, config, "userInfoUrl"), RedirectURL: valueString(nil, config, "redirectUrl"),
		Scopes: valueString(nil, config, "scopes"), Icon: connection.Icon, SortOrder: connection.SortOrder,
		IsDefault: connection.IsDefault, Attributes: cloneConfig(config),
		Remark: connection.Remark,
	}
}

func valueString(primary map[string]any, fallback model.JSONObject, key string) string {
	if value, ok := primary[key].(string); ok && value != "" {
		return value
	}
	if value, ok := fallback[key].(string); ok {
		return value
	}
	return ""
}

func cloneConfig(config model.JSONObject) model.JSONObject {
	result := make(model.JSONObject, len(config))
	for key, value := range config {
		result[key] = value
	}
	return result
}
