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

const (
	// BuiltinAdminConsoleApplicationUUID is the stable client_id for the
	// first-party management console. Built-in behavior must also require
	// IsProtected so an arbitrary record cannot opt into it by UUID alone.
	BuiltinAdminConsoleApplicationUUID = "a1b2cb3c-af00-5ed4-b3da-f85b79012927"
	BuiltinAdminConsoleCallbackPath    = "/api/admin/auth/callback"
)

// Application 应用模型 - 协议无关的应用抽象
type Application struct {
	// 基础信息
	// uniqueIndex 与 DeletedAt 组成复合唯一索引（见 DeletedAt 的 tag）。
	ID          uint   `gorm:"primaryKey" json:"id"`
	UUID        string `gorm:"size:36;uniqueIndex:uk_applications_uuid,priority:1;not null" json:"uuid" validate:"required"`
	Name        string `gorm:"size:100;not null" json:"name" validate:"required"`
	Description string `gorm:"type:text" json:"description"`
	Icon        string `gorm:"size:255;not null;default:''" json:"icon" validate:"max=255"`

	// 协议类型和配置
	ProtocolType   string                 `gorm:"size:20;not null;index" json:"protocolType" validate:"required,oneof=oidc saml cas oauth2"`
	ProtocolConfig map[string]interface{} `gorm:"type:json;serializer:json;not null" json:"protocolConfig" validate:"required"`

	// 状态管理
	IsActive    bool `gorm:"default:true" json:"isActive"`
	IsVerified  bool `gorm:"default:false" json:"isVerified"`
	IsProtected bool `gorm:"default:false;not null" json:"isProtected"`

	// 授权范围配置
	AccessScope *ApplicationAccessScope `gorm:"type:json;serializer:json" json:"accessScope"`

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"uniqueIndex:uk_applications_uuid,priority:2;index" json:"-"`

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

// ============== Application Access Scope ==============

// ApplicationAccessScope 应用访问授权范围
// 用于控制哪些用户可以访问应用
// 未显式设置 IsPublic 的历史配置沿用空范围允许所有用户访问的规则。
type ApplicationAccessScope struct {
	IsPublic              *bool  `json:"isPublic,omitempty"`    // 公开应用：所有已登录用户可访问
	UserIDs               []uint `json:"userIds"`               // 授权用户ID列表
	DepartmentIDs         []uint `json:"departmentIds"`         // 授权部门ID列表
	RoleIDs               []uint `json:"roleIds"`               // 授权角色ID列表
	IncludeSubDepartments bool   `json:"includeSubDepartments"` // 是否包含子部门
}

// AllowsEveryone 检查是否公开，兼容未设置公开标记的历史配置。
func (a *ApplicationAccessScope) AllowsEveryone() bool {
	if a != nil && a.IsPublic != nil {
		return *a.IsPublic
	}
	return a.IsEmpty()
}

// IsEmpty 检查是否未选择任何用户、部门或角色。
func (a *ApplicationAccessScope) IsEmpty() bool {
	if a == nil {
		return true
	}
	return len(a.UserIDs) == 0 && len(a.DepartmentIDs) == 0 && len(a.RoleIDs) == 0
}

// HasUserID 检查指定用户ID是否在授权列表中
func (a *ApplicationAccessScope) HasUserID(userID uint) bool {
	if a == nil {
		return false
	}
	for _, id := range a.UserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// HasDepartmentID 检查指定部门ID是否在授权列表中
func (a *ApplicationAccessScope) HasDepartmentID(deptID uint) bool {
	if a == nil {
		return false
	}
	for _, id := range a.DepartmentIDs {
		if id == deptID {
			return true
		}
	}
	return false
}

// HasRoleID 检查指定角色ID是否在授权列表中
func (a *ApplicationAccessScope) HasRoleID(roleID uint) bool {
	if a == nil {
		return false
	}
	for _, id := range a.RoleIDs {
		if id == roleID {
			return true
		}
	}
	return false
}
