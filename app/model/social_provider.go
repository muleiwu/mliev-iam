package model

import (
	"time"

	"gorm.io/gorm"
)

// ProviderType 提供商类型枚举
type ProviderType string

const (
	ProviderTypeGeneric ProviderType = "generic" // 通用OAuth2.0
	ProviderTypeWeChat  ProviderType = "wechat"  // 微信（预留）
	ProviderTypeQQ      ProviderType = "qq"      // QQ（预留）
	ProviderTypeAlipay  ProviderType = "alipay"  // 支付宝（预留）
	ProviderTypeGithub  ProviderType = "github"  // GitHub
	ProviderTypeGoogle  ProviderType = "google"  // Google
)

// String returns the string representation of ProviderType
func (p ProviderType) String() string {
	return string(p)
}

// ProviderStatus 提供商状态枚举
type ProviderStatus int

const (
	ProviderStatusDisabled ProviderStatus = iota // 禁用
	ProviderStatusEnabled                        // 启用
)

// SocialProvider represents a social login provider configuration.
// 社会化登录提供商配置模型，支持动态配置多种OAuth2.0提供商
type SocialProvider struct {
	// 基础信息
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"size:50;not null" json:"name" validate:"required,max=50"`                   // 提供商名称（显示用）
	Identifier  string       `gorm:"uniqueIndex;size:50;not null" json:"identifier" validate:"required,max=50"` // 唯一标识（用于URL路由）
	Type        ProviderType `gorm:"size:20;not null" json:"type" validate:"required"`                          // 提供商类型
	Description string       `gorm:"size:255" json:"description" validate:"max=255"`                            // 描述

	// OAuth2.0配置
	ClientID     string `gorm:"size:255;not null" json:"clientId" validate:"required,max=255"`    // 应用ID
	ClientSecret string `gorm:"size:255;not null" json:"-"`                                       // 应用密钥（不返回给前端）
	AuthURL      string `gorm:"size:255" json:"authUrl" validate:"max=255"`                       // 授权端点URL
	TokenURL     string `gorm:"size:255" json:"tokenUrl" validate:"max=255"`                      // 令牌端点URL
	UserInfoURL  string `gorm:"size:255" json:"userInfoUrl" validate:"max=255"`                   // 用户信息端点URL
	RedirectURL  string `gorm:"size:255;not null" json:"redirectUrl" validate:"required,max=255"` // 回调URL

	// 配置参数
	Scopes string `gorm:"type:text" json:"scopes"` // OAuth2.0作用域（逗号分隔）

	// 显示配置
	Icon      string `gorm:"size:255" json:"icon" validate:"max=255"` // 图标URL或图标名称
	SortOrder int    `gorm:"default:0" json:"sortOrder"`              // 排序（越小越靠前）

	// 状态管理
	Status    ProviderStatus `gorm:"type:tinyint;default:0;not null" json:"status"` // 状态：0-禁用，1-启用
	IsDefault bool           `gorm:"default:false" json:"isDefault"`                // 是否为默认推荐

	// 扩展属性（用于存储特定平台的额外配置）
	// 例如：微信的 UnionID 配置、支付宝的特定参数等
	Attributes JSONObject `gorm:"type:json" json:"attributes,omitempty"`

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 备注
	Remark string `gorm:"type:text" json:"remark"`
}

// TableName returns the table name for SocialProvider model.
func (*SocialProvider) TableName() string {
	return "social_providers"
}

// BeforeCreate hook to set default values before creating
func (sp *SocialProvider) BeforeCreate(tx *gorm.DB) error {
	if sp.Status == 0 {
		sp.Status = ProviderStatusDisabled
	}
	return nil
}

// IsEnabled checks if provider is enabled
func (sp *SocialProvider) IsEnabled() bool {
	return sp.Status == ProviderStatusEnabled
}

// Enable enables the provider
func (sp *SocialProvider) Enable() {
	sp.Status = ProviderStatusEnabled
}

// Disable disables the provider
func (sp *SocialProvider) Disable() {
	sp.Status = ProviderStatusDisabled
}

// GetScopes returns scopes as a slice
func (sp *SocialProvider) GetScopes() []string {
	if sp.Scopes == "" {
		return []string{}
	}
	result := []string{}
	for _, scope := range splitAndTrim(sp.Scopes, ",") {
		if scope != "" {
			result = append(result, scope)
		}
	}
	return result
}

// splitAndTrim splits string by delimiter and trims spaces
func splitAndTrim(s, sep string) []string {
	parts := []string{}
	for i := 0; i < len(s); {
		// Find next separator
		j := i
		for j < len(s) && string(s[j]) != sep {
			j++
		}
		// Trim and add non-empty parts
		part := s[i:j]
		// Manual trim
		start := 0
		end := len(part)
		for start < end && part[start] == ' ' {
			start++
		}
		for start < end && part[end-1] == ' ' {
			end--
		}
		if start < end {
			parts = append(parts, part[start:end])
		}
		i = j + len(sep)
	}
	return parts
}
