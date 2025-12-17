package model

import (
	"time"

	"gorm.io/gorm"
)

// TokenType 令牌类型枚举
type TokenType string

const (
	TokenTypeBearer TokenType = "Bearer"
)

// OIDCToken OIDC令牌模型
// 用于存储访问令牌、刷新令牌和ID令牌
type OIDCToken struct {
	// 基础信息
	ID          uint      `gorm:"primaryKey" json:"id"`
	AccessToken string    `gorm:"uniqueIndex;type:text;not null" json:"accessToken"`
	TokenType   TokenType `gorm:"size:20;default:'Bearer'" json:"tokenType"`

	// 关联信息
	ClientID string `gorm:"size:100;not null;index" json:"clientId"`
	UserID   uint   `gorm:"not null;index" json:"userId"`
	// Client   *OIDCClient `gorm:"foreignKey:ClientID;references:ClientID" json:"client,omitempty"`
	// User     *User       `gorm:"foreignKey:UserID" json:"user,omitempty"`

	// 令牌信息
	Scopes       []string `gorm:"type:json;serializer:json" json:"scopes"`
	RefreshToken string   `gorm:"uniqueIndex;size:255" json:"refreshToken"`
	IDToken      string   `gorm:"type:text" json:"idToken"`

	// 时间管理
	ExpiresAt        time.Time  `json:"expiresAt"`
	RefreshExpiresAt *time.Time `json:"refreshExpiresAt"`

	// 状态管理
	IsRevoked bool `gorm:"default:false" json:"isRevoked"`

	// 关联的授权码
	AuthorizationCodeID *uint `gorm:"index" json:"authorizationCodeId"`

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 扩展属性
	Attributes map[string]interface{} `gorm:"type:json;serializer:json" json:"attributes,omitempty"`
}

// TableName returns the table name for OIDCToken model
func (*OIDCToken) TableName() string {
	return "oidc_tokens"
}

// BeforeCreate hook to set expiration time before creating
func (t *OIDCToken) BeforeCreate(tx *gorm.DB) error {
	if t.ExpiresAt.IsZero() {
		// 访问令牌默认1小时过期
		t.ExpiresAt = time.Now().Add(1 * time.Hour)
	}

	if t.RefreshToken != "" && t.RefreshExpiresAt == nil {
		// 刷新令牌默认30天过期
		refreshExpiry := time.Now().Add(30 * 24 * time.Hour)
		t.RefreshExpiresAt = &refreshExpiry
	}

	if t.TokenType == "" {
		t.TokenType = TokenTypeBearer
	}

	return nil
}

// IsExpired checks if access token is expired
func (t *OIDCToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsRefreshExpired checks if refresh token is expired
func (t *OIDCToken) IsRefreshExpired() bool {
	if t.RefreshExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.RefreshExpiresAt)
}

// IsValid checks if token is valid
func (t *OIDCToken) IsValid() bool {
	return !t.IsRevoked && !t.IsExpired()
}

// IsRefreshValid checks if refresh token is valid
func (t *OIDCToken) IsRefreshValid() bool {
	return !t.IsRevoked && t.RefreshToken != "" && !t.IsRefreshExpired()
}

// Revoke revokes the token
func (t *OIDCToken) Revoke() {
	t.IsRevoked = true
}

// HasScope checks if token has specific scope
func (t *OIDCToken) HasScope(scope string) bool {
	for _, s := range t.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// GetExpiresIn returns seconds until token expires
func (t *OIDCToken) GetExpiresIn() int64 {
	if t.IsExpired() {
		return 0
	}
	return int64(time.Until(t.ExpiresAt).Seconds())
}
