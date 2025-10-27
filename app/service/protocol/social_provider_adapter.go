package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"golang.org/x/oauth2"
)

// SocialUser 标准化的社交用户信息
type SocialUser struct {
	OpenID      string         // 用户唯一标识
	UnionID     string         // 联合ID（如微信UnionID）
	Nickname    string         // 昵称
	Avatar      string         // 头像URL
	Email       string         // 邮箱
	Phone       string         // 手机号
	RawUserInfo map[string]any // 原始用户信息
}

// SocialProviderAdapter 社会化登录提供商适配器接口
// 定义了统一的社会化登录流程，支持不同平台的扩展
type SocialProviderAdapter interface {
	// GetAuthURL 获取OAuth2.0授权URL
	// state: 用于防CSRF的随机字符串
	// 返回：授权URL
	GetAuthURL(state string) string

	// ExchangeToken 交换授权码获取访问令牌
	// ctx: 上下文
	// code: 授权码
	// 返回：OAuth2令牌
	ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error)

	// GetUserInfo 获取用户信息
	// ctx: 上下文
	// token: 访问令牌
	// 返回：标准化的用户信息
	GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error)

	// GetProviderType 获取提供商类型
	GetProviderType() model.ProviderType

	// ValidateConfig 验证提供商配置是否有效
	ValidateConfig() error
}

// GenericOAuth2Adapter 通用OAuth2.0适配器
// 适用于标准的OAuth2.0提供商（如GitHub、Google等）
type GenericOAuth2Adapter struct {
	provider *model.SocialProvider
	config   *oauth2.Config
}

// NewGenericOAuth2Adapter 创建通用OAuth2.0适配器
func NewGenericOAuth2Adapter(provider *model.SocialProvider) *GenericOAuth2Adapter {
	config := &oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  provider.AuthURL,
			TokenURL: provider.TokenURL,
		},
		RedirectURL: provider.RedirectURL,
		Scopes:      provider.GetScopes(),
	}

	return &GenericOAuth2Adapter{
		provider: provider,
		config:   config,
	}
}

// GetAuthURL 获取授权URL
func (a *GenericOAuth2Adapter) GetAuthURL(state string) string {
	return a.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

// ExchangeToken 交换授权码获取令牌
func (a *GenericOAuth2Adapter) ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error) {
	return a.config.Exchange(ctx, code)
}

// GetUserInfo 获取用户信息
func (a *GenericOAuth2Adapter) GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error) {
	if a.provider.UserInfoURL == "" {
		return nil, fmt.Errorf("user info URL not configured")
	}

	// 使用访问令牌请求用户信息
	client := a.config.Client(ctx, token)
	resp, err := client.Get(a.provider.UserInfoURL)
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

	// 解析用户信息
	var rawUserInfo map[string]any
	if err := json.Unmarshal(body, &rawUserInfo); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}

	// 标准化用户信息
	socialUser := &SocialUser{
		RawUserInfo: rawUserInfo,
	}

	// 尝试提取常见字段
	if id, ok := rawUserInfo["id"]; ok {
		socialUser.OpenID = fmt.Sprintf("%v", id)
	} else if sub, ok := rawUserInfo["sub"]; ok {
		socialUser.OpenID = fmt.Sprintf("%v", sub)
	}

	if name, ok := rawUserInfo["name"]; ok {
		socialUser.Nickname = fmt.Sprintf("%v", name)
	} else if login, ok := rawUserInfo["login"]; ok {
		socialUser.Nickname = fmt.Sprintf("%v", login)
	}

	if avatar, ok := rawUserInfo["avatar_url"]; ok {
		socialUser.Avatar = fmt.Sprintf("%v", avatar)
	} else if picture, ok := rawUserInfo["picture"]; ok {
		socialUser.Avatar = fmt.Sprintf("%v", picture)
	}

	if email, ok := rawUserInfo["email"]; ok {
		socialUser.Email = fmt.Sprintf("%v", email)
	}

	return socialUser, nil
}

// GetProviderType 获取提供商类型
func (a *GenericOAuth2Adapter) GetProviderType() model.ProviderType {
	return a.provider.Type
}

// ValidateConfig 验证配置
func (a *GenericOAuth2Adapter) ValidateConfig() error {
	if a.provider.ClientID == "" {
		return fmt.Errorf("client ID is required")
	}
	if a.provider.ClientSecret == "" {
		return fmt.Errorf("client secret is required")
	}
	if a.provider.AuthURL == "" {
		return fmt.Errorf("auth URL is required")
	}
	if a.provider.TokenURL == "" {
		return fmt.Errorf("token URL is required")
	}
	if a.provider.RedirectURL == "" {
		return fmt.Errorf("redirect URL is required")
	}
	return nil
}

// ==================== 特定平台适配器（预留） ====================

// WeChatAdapter 微信登录适配器（预留）
type WeChatAdapter struct {
	*GenericOAuth2Adapter
	// 微信特有配置
}

// NewWeChatAdapter 创建微信登录适配器
func NewWeChatAdapter(provider *model.SocialProvider) *WeChatAdapter {
	return &WeChatAdapter{
		GenericOAuth2Adapter: NewGenericOAuth2Adapter(provider),
	}
}

// GetUserInfo 获取微信用户信息（重写以支持UnionID）
func (a *WeChatAdapter) GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error) {
	// TODO: 实现微信用户信息获取逻辑
	// 微信需要特殊处理UnionID等字段
	socialUser, err := a.GenericOAuth2Adapter.GetUserInfo(ctx, token)
	if err != nil {
		return nil, err
	}

	// 提取微信特有字段
	if unionID, ok := socialUser.RawUserInfo["unionid"]; ok {
		socialUser.UnionID = fmt.Sprintf("%v", unionID)
	}

	return socialUser, nil
}

// QQAdapter QQ登录适配器（预留）
type QQAdapter struct {
	*GenericOAuth2Adapter
	// QQ特有配置
}

// NewQQAdapter 创建QQ登录适配器
func NewQQAdapter(provider *model.SocialProvider) *QQAdapter {
	return &QQAdapter{
		GenericOAuth2Adapter: NewGenericOAuth2Adapter(provider),
	}
}

// AlipayAdapter 支付宝登录适配器（预留）
type AlipayAdapter struct {
	*GenericOAuth2Adapter
	// 支付宝特有配置
}

// NewAlipayAdapter 创建支付宝登录适配器
func NewAlipayAdapter(provider *model.SocialProvider) *AlipayAdapter {
	return &AlipayAdapter{
		GenericOAuth2Adapter: NewGenericOAuth2Adapter(provider),
	}
}

// ==================== 适配器工厂 ====================

// NewSocialProviderAdapter 根据提供商类型创建适配器
func NewSocialProviderAdapter(provider *model.SocialProvider) (SocialProviderAdapter, error) {
	switch provider.Type {
	case model.ProviderTypeWeChat:
		return NewWeChatAdapter(provider), nil
	case model.ProviderTypeQQ:
		return NewQQAdapter(provider), nil
	case model.ProviderTypeAlipay:
		return NewAlipayAdapter(provider), nil
	case model.ProviderTypeGeneric, model.ProviderTypeGithub, model.ProviderTypeGoogle:
		return NewGenericOAuth2Adapter(provider), nil
	default:
		return NewGenericOAuth2Adapter(provider), nil
	}
}
