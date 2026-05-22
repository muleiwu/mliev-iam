package model

import (
	"time"

	"gorm.io/gorm"
)

// UserIdentityAccount 用户企业身份源账号绑定。
type UserIdentityAccount struct {
	ID uint `gorm:"primaryKey" json:"id"`

	UserID         uint   `gorm:"not null;index:idx_user_identity_provider" json:"userId"`
	ProviderID     uint   `gorm:"not null;index:idx_user_identity_provider;index:idx_identity_provider" json:"providerId"`
	ExternalUserID string `gorm:"size:255;not null;uniqueIndex:idx_identity_provider_external_user" json:"externalUserId"`
	UnionID        string `gorm:"size:255;index" json:"unionId,omitempty"`
	TenantID       string `gorm:"size:255;index" json:"tenantId,omitempty"`

	UserInfo JSONObject `gorm:"type:json" json:"userInfo,omitempty"`

	AccessToken  string     `gorm:"type:text" json:"-"`
	RefreshToken string     `gorm:"type:text" json:"-"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	TokenType    string     `gorm:"size:50" json:"-"`

	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	LoginCount int64      `gorm:"default:0" json:"loginCount"`

	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Attributes JSONObject `gorm:"type:json" json:"attributes,omitempty"`
	Remark     string     `gorm:"type:text" json:"remark"`

	User     *User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Provider *IdentityProvider `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

func (*UserIdentityAccount) TableName() string {
	return "user_identity_accounts"
}

func (a *UserIdentityAccount) UpdateLastUsed() {
	now := time.Now()
	a.LastUsedAt = &now
	a.LoginCount++
}

func (a *UserIdentityAccount) SetTokenInfo(accessToken, refreshToken, tokenType string, expiresIn int64) {
	a.AccessToken = accessToken
	a.RefreshToken = refreshToken
	a.TokenType = tokenType
	if expiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
		a.ExpiresAt = &expiresAt
	}
}
