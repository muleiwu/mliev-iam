package model

import (
	"time"

	"gorm.io/gorm"
)

// GrantType 授权类型枚举
type GrantType string

const (
	GrantTypeAuthorizationCode GrantType = "authorization_code"
	GrantTypeClientCredentials GrantType = "client_credentials"
	GrantTypeRefreshToken      GrantType = "refresh_token"
	GrantTypeImplicit          GrantType = "implicit"
)

// ResponseType 响应类型枚举
type ResponseType string

const (
	ResponseTypeCode    ResponseType = "code"
	ResponseTypeToken   ResponseType = "token"
	ResponseTypeIDToken ResponseType = "id_token"
)

// OIDCClient OIDC客户端模型
// 用于存储第三方应用的客户端信息
type OIDCClient struct {
	// 基础信息
	ID           uint   `gorm:"primaryKey" json:"id"`
	ClientID     string `gorm:"uniqueIndex;size:100;not null" json:"clientId" validate:"required"`
	ClientSecret string `gorm:"size:100;not null" json:"-" validate:"required"`
	Name         string `gorm:"size:100;not null" json:"name" validate:"required"`
	Description  string `gorm:"type:text" json:"description"`

	// OIDC 配置
	RedirectURIs  []string `gorm:"type:json;not null" json:"redirectUris" validate:"required"`
	GrantTypes    []string `gorm:"type:json;not null" json:"grantTypes" validate:"required"`
	ResponseTypes []string `gorm:"type:json;not null" json:"responseTypes" validate:"required"`
	Scopes        []string `gorm:"type:json;not null" json:"scopes" validate:"required"`

	// 客户端类型和配置
	ClientType    string `gorm:"size:20;default:'confidential'" json:"clientType"` // confidential, public
	TokenLifetime int    `gorm:"default:3600" json:"tokenLifetime"`                // 令牌有效期（秒）

	// 状态管理
	IsActive   bool `gorm:"default:true" json:"isActive"`
	IsVerified bool `gorm:"default:false" json:"isVerified"`

	// PKCE 支持
	RequirePKCE bool `gorm:"default:false" json:"requirePkce"`

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 扩展属性
	Attributes map[string]interface{} `gorm:"type:json" json:"attributes,omitempty"`

	// 创建者信息
	CreatedByUserID *uint `gorm:"index" json:"createdByUserId"`
	// CreatedByUser   *User `gorm:"foreignKey:CreatedByUserID" json:"createdByUser,omitempty"`
}

// TableName returns the table name for OIDCClient model
func (*OIDCClient) TableName() string {
	return "oidc_clients"
}

// BeforeCreate hook to set default values before creating
func (c *OIDCClient) BeforeCreate(tx *gorm.DB) error {
	if len(c.GrantTypes) == 0 {
		c.GrantTypes = []string{string(GrantTypeAuthorizationCode)}
	}
	if len(c.ResponseTypes) == 0 {
		c.ResponseTypes = []string{string(ResponseTypeCode)}
	}
	if len(c.Scopes) == 0 {
		c.Scopes = []string{"openid", "profile", "email"}
	}
	return nil
}

// HasGrantType checks if client supports specific grant type
func (c *OIDCClient) HasGrantType(grantType GrantType) bool {
	for _, gt := range c.GrantTypes {
		if gt == string(grantType) {
			return true
		}
	}
	return false
}

// HasResponseType checks if client supports specific response type
func (c *OIDCClient) HasResponseType(responseType ResponseType) bool {
	for _, rt := range c.ResponseTypes {
		if rt == string(responseType) {
			return true
		}
	}
	return false
}

// HasScope checks if client has specific scope
func (c *OIDCClient) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// IsRedirectURIValid checks if redirect URI is valid for this client
func (c *OIDCClient) IsRedirectURIValid(redirectURI string) bool {
	for _, uri := range c.RedirectURIs {
		if uri == redirectURI {
			return true
		}
	}
	return false
}

// IsPublicClient checks if this is a public client
func (c *OIDCClient) IsPublicClient() bool {
	return c.ClientType == "public"
}

// IsConfidentialClient checks if this is a confidential client
func (c *OIDCClient) IsConfidentialClient() bool {
	return c.ClientType == "confidential"
}
