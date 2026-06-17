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
	ClientSecret string               `json:"clientSecret" binding:"max=255"`
	AuthURL      string               `json:"authUrl" binding:"max=255"`
	TokenURL     string               `json:"tokenUrl" binding:"max=255"`
	UserInfoURL  string               `json:"userInfoUrl" binding:"max=255"`
	RedirectURL  string               `json:"redirectUrl" binding:"max=255"`
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
	Identifier string             `json:"identifier"`
	Name       string             `json:"name"`
	Icon       string             `json:"icon"`
	SortOrder  int                `json:"sortOrder"`
	IsDefault  bool               `json:"isDefault"`
	Type       model.ProviderType `json:"type"`
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
	SessionID string        `json:"sessionId,omitempty"` // OIDC会话ID（用于OIDC流程）
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
	BindToken             string   `json:"bindToken" binding:"required"`
	Username              string   `json:"username" binding:"required,min=3,max=20"`
	Email                 string   `json:"email" binding:"required,email"`
	Password              string   `json:"password" binding:"required,min=6"`
	RealName              string   `json:"realName" binding:"required,min=2,max=20"`
	AgreePolicy           bool     `json:"agreePolicy"`
	AcceptedAgreementKeys []string `json:"acceptedAgreementKeys,omitempty"`
	SessionID             string   `json:"sessionId,omitempty"` // OIDC会话ID
}

// SocialBindAccountRequest 绑定社交账号请求（已登录用户）
type SocialBindAccountRequest struct {
	ProviderIdentifier string `json:"providerIdentifier" binding:"required"`
}

// SocialUnbindAccountRequest 解绑社交账号请求
type SocialUnbindAccountRequest struct {
	AccountID uint `json:"accountId" binding:"required"`
}

// SocialMiniProgramLoginRequest 小程序登录请求
type SocialMiniProgramLoginRequest struct {
	JsCode    string `json:"jsCode" binding:"required"`
	SessionID string `json:"sessionId,omitempty"`
}

// ==================== 小程序扫码登录（PC 端展示小程序码） ====================

// MPQRCodeCreateRequest PC 创建扫码登录票据
type MPQRCodeCreateRequest struct {
	SessionID  string `json:"sessionId,omitempty"`  // 可选：OIDC 会话 ID
	Page       string `json:"page,omitempty"`       // 可选：小程序打开路径，默认由后端设置
	EnvVersion string `json:"envVersion,omitempty"` // 可选：小程序版本 release/trial/develop，默认 release
}

// MPQRCodeCreateResponse 创建扫码登录票据响应
type MPQRCodeCreateResponse struct {
	Ticket    string `json:"ticket"`           // 本次扫码会话唯一标识
	Scene     string `json:"scene"`            // 传给小程序的 scene 字符串
	QRCode    string `json:"qrcode,omitempty"` // data:image/png;base64,... 形式的小程序码（可能为空）
	ExpiresIn int    `json:"expiresIn"`        // 有效期（秒）
}

// MPQRCodeStatusResponse 扫码状态查询响应
// state 可取值：pending / scanned / confirmed / needs_bind / canceled / expired
type MPQRCodeStatusResponse struct {
	State      string         `json:"state"`
	Token      string         `json:"token,omitempty"`      // state == confirmed 时返回 JWT
	User       *UserResponse  `json:"user,omitempty"`       // state == confirmed 时返回用户信息
	IsNewUser  bool           `json:"isNewUser,omitempty"`  // 对齐已有 callback 语义
	BindToken  string         `json:"bindToken,omitempty"`  // state == needs_bind 时返回
	SocialInfo *MPSocialBrief `json:"socialInfo,omitempty"` // state == needs_bind 时的社交资料片段
	SessionID  string         `json:"sessionId,omitempty"`  // OIDC 会话（透传）
}

// MPSocialBrief 供 PC 在"注册/绑定"抉择页展示的最小社交资料
type MPSocialBrief struct {
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}

// MPQRCodeScanRequest 小程序扫码后上报
type MPQRCodeScanRequest struct {
	Ticket   string         `json:"ticket" binding:"required"`
	JsCode   string         `json:"jsCode" binding:"required"`
	UserInfo *MPSocialBrief `json:"userInfo,omitempty"` // 小程序 wx.getUserProfile 获取到的昵称/头像
}

// MPQRCodeScanResponse 扫码上报响应（小程序端使用）
type MPQRCodeScanResponse struct {
	State         string `json:"state"`                   // scanned / already_bound / not_bound
	Nickname      string `json:"nickname,omitempty"`      // 后台根据 openid 找到的用户昵称（若已绑定）
	Avatar        string `json:"avatar,omitempty"`        // 同上
	PendingAction string `json:"pendingAction,omitempty"` // login / bind
}

// MPQRCodeConfirmRequest 小程序端确认或取消
type MPQRCodeConfirmRequest struct {
	Ticket string `json:"ticket" binding:"required"`
	Action string `json:"action" binding:"required,oneof=confirm cancel"`
}

// OIDCDiscoverRequest OIDC 发现请求
type OIDCDiscoverRequest struct {
	Issuer string `json:"issuer" binding:"required"`
}

// OIDCDiscoverResponse OIDC 发现响应
type OIDCDiscoverResponse struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorizationEndpoint"`
	TokenEndpoint         string   `json:"tokenEndpoint"`
	UserinfoEndpoint      string   `json:"userinfoEndpoint"`
	JwksURI               string   `json:"jwksUri"`
	ScopesSupported       []string `json:"scopesSupported,omitempty"`
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
