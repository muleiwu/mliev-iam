package model

import (
	"time"

	"gorm.io/gorm"
)

// IdentityProviderType 企业身份源类型枚举。
type IdentityProviderType string

const (
	IdentityProviderTypeFeishu   IdentityProviderType = "feishu"
	IdentityProviderTypeWeCom    IdentityProviderType = "wecom"
	IdentityProviderTypeDingTalk IdentityProviderType = "dingtalk"
)

func (p IdentityProviderType) String() string {
	return string(p)
}

// IdentityProviderStatus 身份源状态枚举。
type IdentityProviderStatus int

const (
	IdentityProviderStatusDisabled IdentityProviderStatus = iota
	IdentityProviderStatusEnabled
)

// IdentityProvider 企业身份源配置。
type IdentityProvider struct {
	ID          uint                 `gorm:"primaryKey" json:"id"`
	Name        string               `gorm:"size:50;not null" json:"name" validate:"required,max=50"`
	Identifier  string               `gorm:"uniqueIndex;size:50;not null" json:"identifier" validate:"required,max=50"`
	Type        IdentityProviderType `gorm:"size:20;not null" json:"type" validate:"required"`
	Description string               `gorm:"size:255" json:"description" validate:"max=255"`

	ClientID     string `gorm:"size:255;not null" json:"clientId" validate:"required,max=255"`
	ClientSecret string `gorm:"size:255;not null" json:"-"`
	AuthURL      string `gorm:"size:255" json:"authUrl" validate:"max=255"`
	TokenURL     string `gorm:"size:255" json:"tokenUrl" validate:"max=255"`
	UserInfoURL  string `gorm:"size:255" json:"userInfoUrl" validate:"max=255"`
	RedirectURL  string `gorm:"size:255;not null" json:"redirectUrl" validate:"required,max=255"`
	Scopes       string `gorm:"type:text" json:"scopes"`

	Icon      string `gorm:"size:255" json:"icon" validate:"max=255"`
	SortOrder int    `gorm:"default:0" json:"sortOrder"`

	Status    IdentityProviderStatus `gorm:"default:0;not null" json:"status"`
	IsDefault bool                   `gorm:"default:false" json:"isDefault"`

	Attributes JSONObject `gorm:"type:json" json:"attributes,omitempty"`

	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Remark string `gorm:"type:text" json:"remark"`
}

func (*IdentityProvider) TableName() string {
	return "identity_providers"
}

func (p *IdentityProvider) BeforeCreate(tx *gorm.DB) error {
	if p.Status == 0 {
		p.Status = IdentityProviderStatusDisabled
	}
	return nil
}

func (p *IdentityProvider) IsEnabled() bool {
	return p.Status == IdentityProviderStatusEnabled
}

func (p *IdentityProvider) GetScopes() []string {
	if p.Scopes == "" {
		return []string{}
	}
	result := []string{}
	for _, scope := range splitAndTrim(p.Scopes, ",") {
		if scope != "" {
			result = append(result, scope)
		}
	}
	return result
}
