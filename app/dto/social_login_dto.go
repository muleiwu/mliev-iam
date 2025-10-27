package dto

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// ==================== 提供商配置相关 DTO ====================

// CreateSocialProviderRequest 创建社会化登录提供商请求
type CreateSocialProviderRequest struct {
	Name         string               `json:"name" binding:"required,max=50"`
	Identifier   string               `json:"identifier" binding:"required,max=50"`
	Type         model.ProviderType   `json:"type" binding:"required"`
	Description  string               `json:"description" binding:"max=255"`
	ClientID     string               `json:"clientId" binding:"required,max=255"`
	ClientSecret string               `json:"clientSecret" binding:"required,max=255"`
	AuthURL      string               `json:"authUrl" binding:"max=255"`
	TokenURL     string               `json:"tokenUrl" binding:"max=255"`
	UserInfoURL  string               `json:"userInfoUrl" binding:"max=255"`
	RedirectURL  string               `json:"redirectUrl" binding:"required,max=255"`
	Scopes       string               `json:"scopes"`
	Icon         string               `json:"icon" binding:"max=255"`
	SortOrder    int                  `json:"sortOrder"`
	Status       model.ProviderStatus `json:"status"`
	IsDefault    bool                 `json:"isDefault"`
	Attributes   map[string]any       `json:"attributes,omitempty"`
	Remark       string               `json:"remark"`
}

// UpdateSocialProviderRequest 更新社会化登录提供商请求
type UpdateSocialProviderRequest struct {
	Name         *string               `json:"name" binding:"omitempty,max=50"`
	Identifier   *string               `json:"identifier" binding:"omitempty,max=50"`
	Type         *model.ProviderType   `json:"type" binding:"omitempty"`
	Description  *string               `json:"description" binding:"omitempty,max=255"`
	ClientID     *string               `json:"clientId" binding:"omitempty,max=255"`
	ClientSecret *string               `json:"clientSecret" binding:"omitempty,max=255"`
	AuthURL      *string               `json:"authUrl" binding:"omitempty,max=255"`
	TokenURL     *string               `json:"tokenUrl" binding:"omitempty,max=255"`
	UserInfoURL  *string               `json:"userInfoUrl" binding:"omitempty,max=255"`
	RedirectURL  *string               `json:"redirectUrl" binding:"omitempty,max=255"`
	Scopes       *string               `json:"scopes"`
	Icon         *string               `json:"icon" binding:"omitempty,max=255"`
	SortOrder    *int                  `json:"sortOrder"`
	Status       *model.ProviderStatus `json:"status"`
	IsDefault    *bool                 `json:"isDefault"`
	Attributes   map[string]any        `json:"attributes,omitempty"`
	Remark       *string               `json:"remark"`
}

// SocialProviderResponse 社会化登录提供商响应
type SocialProviderResponse struct {
	ID          uint                 `json:"id"`
	Name        string               `json:"name"`
	Identifier  string               `json:"identifier"`
	Type        model.ProviderType   `json:"type"`
	Description string               `json:"description"`
	ClientID    string               `json:"clientId"`
	AuthURL     string               `json:"authUrl"`
	TokenURL    string               `json:"tokenUrl"`
	UserInfoURL string               `json:"userInfoUrl"`
	RedirectURL string               `json:"redirectUrl"`
	Scopes      string               `json:"scopes"`
	Icon        string               `json:"icon"`
	SortOrder   int                  `json:"sortOrder"`
	Status      model.ProviderStatus `json:"status"`
	IsDefault   bool                 `json:"isDefault"`
	Attributes  map[string]any       `json:"attributes,omitempty"`
	Remark      string               `json:"remark"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

// SocialProviderListItem 提供商列表项（用于前端展示）
type SocialProviderListItem struct {
	ID          uint                 `json:"id"`
	Name        string               `json:"name"`
	Identifier  string               `json:"identifier"`
	Type        model.ProviderType   `json:"type"`
	Icon        string               `json:"icon"`
	Status      model.ProviderStatus `json:"status"`
	IsDefault   bool                 `json:"isDefault"`
	SortOrder   int                  `json:"sortOrder"`
	Description string               `json:"description"`
}

// PublicProviderInfo 公开的提供商信息（用于登录页面展示，不包含敏感信息）
type PublicProviderInfo struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	SortOrder  int    `json:"sortOrder"`
	IsDefault  bool   `json:"isDefault"`
}

// SocialProviderBatchRequest 批量操作提供商请求
type SocialProviderBatchRequest struct {
	IDs       []uint `json:"ids" binding:"required,min=1"`
	Operation string `json:"operation" binding:"required,oneof=enable disable delete"`
}

// ==================== 用户社交账号相关 DTO ====================

// UserSocialAccountResponse 用户社交账号响应
type UserSocialAccountResponse struct {
	ID           uint           `json:"id"`
	UserID       uint           `json:"userId"`
	ProviderID   uint           `json:"providerId"`
	ProviderName string         `json:"providerName"` // 提供商名称
	ProviderIcon string         `json:"providerIcon"` // 提供商图标
	OpenID       string         `json:"openId"`
	UnionID      string         `json:"unionId,omitempty"`
	UserInfo     map[string]any `json:"userInfo,omitempty"`
	LastUsedAt   *time.Time     `json:"lastUsedAt,omitempty"`
	LoginCount   int64          `json:"loginCount"`
	CreatedAt    time.Time      `json:"createdAt"`
}

// ==================== OAuth认证流程相关 DTO ====================

// SocialAuthURLResponse OAuth授权URL响应
type SocialAuthURLResponse struct {
	AuthURL string `json:"authUrl"` // 授权URL
	State   string `json:"state"`   // State参数（用于防CSRF）
}

// SocialAuthCallbackRequest OAuth回调请求
type SocialAuthCallbackRequest struct {
	Code  string `form:"code" binding:"required"`
	State string `form:"state" binding:"required"`
}

// SocialAuthCallbackResponse OAuth回调响应（登录成功）
type SocialAuthCallbackResponse struct {
	Token     string        `json:"token"`               // JWT访问令牌
	User      *UserResponse `json:"user"`                // 用户信息
	IsNewUser bool          `json:"isNewUser"`           // 是否为新注册用户
	NeedsBind bool          `json:"needsBind"`           // 是否需要绑定已有账号
	BindToken string        `json:"bindToken,omitempty"` // 绑定令牌（用于后续绑定操作）
}

// SocialBindWithLoginRequest 用社交账号绑定已有账户
type SocialBindWithLoginRequest struct {
	BindToken string `json:"bindToken" binding:"required"`
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	SessionID string `json:"sessionId,omitempty"` // OIDC会话ID
}

// SocialRegisterRequest 用社交账号注册新账户
type SocialRegisterRequest struct {
	BindToken   string `json:"bindToken" binding:"required"`
	Username    string `json:"username" binding:"required,min=3,max=20"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	RealName    string `json:"realName" binding:"required,min=2,max=20"`
	AgreePolicy bool   `json:"agreePolicy" binding:"required"`
	SessionID   string `json:"sessionId,omitempty"` // OIDC会话ID
}

// SocialBindAccountRequest 绑定社交账号请求（已登录用户）
type SocialBindAccountRequest struct {
	ProviderIdentifier string `json:"providerIdentifier" binding:"required"`
}

// SocialUnbindAccountRequest 解绑社交账号请求
type SocialUnbindAccountRequest struct {
	AccountID uint `json:"accountId" binding:"required"`
}

// ==================== 统计信息 DTO ====================

// SocialProviderStatsResponse 提供商统计信息响应
type SocialProviderStatsResponse struct {
	TotalProviders    int64 `json:"totalProviders"`    // 总提供商数
	EnabledProviders  int64 `json:"enabledProviders"`  // 启用的提供商数
	DisabledProviders int64 `json:"disabledProviders"` // 禁用的提供商数
	TotalBindings     int64 `json:"totalBindings"`     // 总绑定数
}

// SocialProviderUsageStats 提供商使用统计
type SocialProviderUsageStats struct {
	ProviderID   uint   `json:"providerId"`
	ProviderName string `json:"providerName"`
	BindingCount int64  `json:"bindingCount"` // 绑定数
	LoginCount   int64  `json:"loginCount"`   // 登录次数
}
