package model

import (
	"time"

	"gorm.io/gorm"
)

// ProtocolType 协议类型枚举
type ProtocolType string

const (
	ProtocolTypeOIDC   ProtocolType = "oidc"
	ProtocolTypeSAML   ProtocolType = "saml"
	ProtocolTypeCAS    ProtocolType = "cas"
	ProtocolTypeOAuth2 ProtocolType = "oauth2"
)

// Application 应用模型 - 协议无关的应用抽象
type Application struct {
	// 基础信息
	ID          uint   `gorm:"primaryKey" json:"id"`
	UUID        string `gorm:"size:36;uniqueIndex;not null" json:"uuid" validate:"required"`
	Name        string `gorm:"size:100;not null" json:"name" validate:"required"`
	Description string `gorm:"type:text" json:"description"`

	// 协议类型和配置
	ProtocolType   string                 `gorm:"size:20;not null;index" json:"protocolType" validate:"required,oneof=oidc saml cas oauth2"`
	ProtocolConfig map[string]interface{} `gorm:"type:json;serializer:json;not null" json:"protocolConfig" validate:"required"`

	// 状态管理
	IsActive   bool `gorm:"default:true" json:"isActive"`
	IsVerified bool `gorm:"default:false" json:"isVerified"`

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 创建者信息
	CreatedByUserID *uint `gorm:"index" json:"createdByUserId"`
}

// TableName returns the table name for Application model
func (*Application) TableName() string {
	return "applications"
}

// ============== Protocol-Specific Config Structures ==============

// OIDCProtocolConfig OIDC协议特定配置
type OIDCProtocolConfig struct {
	ClientID      string   `json:"clientId"`
	ClientSecret  string   `json:"clientSecret"`
	RedirectURIs  []string `json:"redirectUris"`
	GrantTypes    []string `json:"grantTypes"`
	ResponseTypes []string `json:"responseTypes"`
	Scopes        []string `json:"scopes"`
	ClientType    string   `json:"clientType"` // confidential, public
	RequirePKCE   bool     `json:"requirePkce"`
	TokenLifetime int      `json:"tokenLifetime"` // 令牌有效期（秒）
}

// SAMLProtocolConfig SAML协议特定配置
type SAMLProtocolConfig struct {
	EntityID          string            `json:"entityId"`
	ACSURL            string            `json:"acsUrl"`
	SLOURL            string            `json:"sloUrl,omitempty"`
	Certificate       string            `json:"certificate,omitempty"`
	SignatureMethod   string            `json:"signatureMethod,omitempty"`
	NameIDFormat      string            `json:"nameIdFormat,omitempty"`
	AttributeMappings map[string]string `json:"attributeMappings,omitempty"`
}

// CASProtocolConfig CAS协议特定配置
type CASProtocolConfig struct {
	ServiceURL string            `json:"serviceUrl"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ============== OIDC-Specific Enums (kept for OIDC protocol) ==============

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
