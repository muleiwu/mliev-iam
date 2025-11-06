package model

import (
	"time"

	"gorm.io/gorm"
)

// UserSocialAccount represents a user's social account binding.
// 用户社交账号绑定模型，记录用户与第三方平台账号的关联关系
type UserSocialAccount struct {
	// 基础信息
	ID uint `gorm:"primaryKey" json:"id"`

	// 关联信息
	UserID     uint   `gorm:"not null;index:idx_user_provider" json:"userId"`                        // 本地用户ID
	ProviderID uint   `gorm:"not null;index:idx_user_provider;index:idx_provider" json:"providerId"` // 提供商ID
	OpenID     string `gorm:"size:255;not null;uniqueIndex:idx_provider_openid" json:"openId"`       // 第三方平台用户唯一标识
	UnionID    string `gorm:"size:255;index" json:"unionId,omitempty"`                               // 联合ID（如微信UnionID）

	// 第三方用户信息（JSON存储）
	// 包含：昵称、头像、邮箱等第三方平台返回的用户信息
	UserInfo JSONObject `gorm:"type:json" json:"userInfo,omitempty"`

	// 令牌信息（加密存储，用于可能的令牌刷新）
	AccessToken  string     `gorm:"type:text" json:"-"`  // 访问令牌（不返回给前端）
	RefreshToken string     `gorm:"type:text" json:"-"`  // 刷新令牌（不返回给前端）
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"` // 令牌过期时间
	TokenType    string     `gorm:"size:50" json:"-"`    // 令牌类型（如：Bearer）

	// 使用统计
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`        // 最后使用时间
	LoginCount int64      `gorm:"default:0" json:"loginCount"` // 使用此账号登录次数

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"` // 绑定时间
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 扩展属性
	Attributes JSONObject `gorm:"type:json" json:"attributes,omitempty"`
	Remark     string     `gorm:"type:text" json:"remark"`

	// 关联对象（用于查询时预加载）
	User     *User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Provider *SocialProvider `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

// TableName returns the table name for UserSocialAccount model.
func (*UserSocialAccount) TableName() string {
	return "user_social_accounts"
}

// BeforeCreate hook
func (usa *UserSocialAccount) BeforeCreate(tx *gorm.DB) error {
	return nil
}

// UpdateLastUsed updates the last used timestamp and increments login count
func (usa *UserSocialAccount) UpdateLastUsed() {
	now := time.Now()
	usa.LastUsedAt = &now
	usa.LoginCount++
}

// IsTokenValid checks if the access token is still valid
func (usa *UserSocialAccount) IsTokenValid() bool {
	if usa.ExpiresAt == nil {
		return false
	}
	return time.Now().Before(*usa.ExpiresAt)
}

// SetTokenInfo sets the token information
func (usa *UserSocialAccount) SetTokenInfo(accessToken, refreshToken, tokenType string, expiresIn int64) {
	usa.AccessToken = accessToken
	usa.RefreshToken = refreshToken
	usa.TokenType = tokenType
	if expiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
		usa.ExpiresAt = &expiresAt
	}
}
