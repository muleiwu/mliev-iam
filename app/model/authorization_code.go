package model

import (
	"time"

	"gorm.io/gorm"
)

// AuthorizationCode 授权码模型
// 用于存储OAuth 2.0授权码信息
type AuthorizationCode struct {
	// 基础信息
	ID   uint   `gorm:"primaryKey" json:"id"`
	Code string `gorm:"uniqueIndex;size:100;not null" json:"code"`

	// 关联信息
	ClientID string `gorm:"size:100;not null;index" json:"clientId"`
	UserID   uint   `gorm:"not null;index" json:"userId"`
	// Client   *OIDCClient `gorm:"foreignKey:ClientID;references:ClientID" json:"client,omitempty"`
	// User     *User       `gorm:"foreignKey:UserID" json:"user,omitempty"`

	// 授权信息
	RedirectURI string   `gorm:"size:text;not null" json:"redirectUri"`
	Scopes      []string `gorm:"type:json;serializer:json" json:"scopes"`
	State       string   `gorm:"type:text" json:"state"`
	Nonce       string   `gorm:"size:text" json:"nonce"`

	// PKCE 支持
	CodeChallenge       string `gorm:"size:128" json:"codeChallenge"`
	CodeChallengeMethod string `gorm:"size:10" json:"codeChallengeMethod"`

	// 状态管理
	IsUsed    bool      `gorm:"default:false" json:"isUsed"`
	ExpiresAt time.Time `json:"expiresAt"`

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the table name for AuthorizationCode model
func (*AuthorizationCode) TableName() string {
	return "authorization_codes"
}

// BeforeCreate hook to set expiration time before creating
func (ac *AuthorizationCode) BeforeCreate(tx *gorm.DB) error {
	if ac.ExpiresAt.IsZero() {
		// 授权码默认10分钟过期
		ac.ExpiresAt = time.Now().Add(10 * time.Minute)
	}
	return nil
}

// IsExpired checks if authorization code is expired
func (ac *AuthorizationCode) IsExpired() bool {
	return time.Now().After(ac.ExpiresAt)
}

// IsValid checks if authorization code is valid
func (ac *AuthorizationCode) IsValid() bool {
	return !ac.IsUsed && !ac.IsExpired()
}

// MarkAsUsed marks the authorization code as used
func (ac *AuthorizationCode) MarkAsUsed() {
	ac.IsUsed = true
}

// HasScope checks if authorization code has specific scope
func (ac *AuthorizationCode) HasScope(scope string) bool {
	for _, s := range ac.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}
