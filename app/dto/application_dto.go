package dto

// ============== Application DTOs ==============

// CreateApplicationRequest 创建应用请求
type CreateApplicationRequest struct {
	Name           string                 `json:"name" binding:"required" validate:"max=100"`
	Description    string                 `json:"description" validate:"max=500"`
	ProtocolType   string                 `json:"protocolType" binding:"required,oneof=oidc saml cas oauth2"`
	ProtocolConfig map[string]interface{} `json:"protocolConfig" binding:"required"`
}

// UpdateApplicationRequest 更新应用请求
type UpdateApplicationRequest struct {
	Name           *string                 `json:"name,omitempty" validate:"max=100"`
	Description    *string                 `json:"description,omitempty" validate:"max=500"`
	ProtocolConfig *map[string]interface{} `json:"protocolConfig,omitempty"`
	IsActive       *bool                   `json:"isActive,omitempty"`
}

// ApplicationResponse 应用响应
type ApplicationResponse struct {
	ID             uint                   `json:"id"`
	UUID           string                 `json:"uuid"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	ProtocolType   string                 `json:"protocolType"`
	ProtocolConfig map[string]interface{} `json:"protocolConfig"`
	IsActive       bool                   `json:"isActive"`
	IsVerified     bool                   `json:"isVerified"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
}

// ApplicationStats 应用统计信息
type ApplicationStats struct {
	ActiveApplications   int64 `json:"activeApplications"`
	InactiveApplications int64 `json:"inactiveApplications"`
	ActiveTokens         int64 `json:"activeTokens"`
}

// ============== OIDC Application Config DTOs ==============

// OIDCApplicationConfig OIDC应用配置（用于前端表单）
type OIDCApplicationConfig struct {
	ClientID      string   `json:"clientId,omitempty"`
	ClientSecret  string   `json:"clientSecret,omitempty"` // 仅在创建时返回
	RedirectURIs  []string `json:"redirectUris" binding:"required,min=1"`
	GrantTypes    []string `json:"grantTypes" binding:"required,min=1"`
	ResponseTypes []string `json:"responseTypes" binding:"required,min=1"`
	Scopes        []string `json:"scopes" binding:"required,min=1"`
	ClientType    string   `json:"clientType" binding:"required,oneof=confidential public"`
	RequirePKCE   bool     `json:"requirePkce"`
	TokenLifetime int      `json:"tokenLifetime,omitempty"`
}

// SAMLApplicationConfig SAML应用配置（用于前端表单）
type SAMLApplicationConfig struct {
	EntityID          string            `json:"entityId" binding:"required"`
	ACSURL            string            `json:"acsUrl" binding:"required"`
	SLOURL            string            `json:"sloUrl,omitempty"`
	Certificate       string            `json:"certificate,omitempty"`
	SignatureMethod   string            `json:"signatureMethod,omitempty"`
	NameIDFormat      string            `json:"nameIdFormat,omitempty"`
	AttributeMappings map[string]string `json:"attributeMappings,omitempty"`
}

// CASApplicationConfig CAS应用配置（用于前端表单）
type CASApplicationConfig struct {
	ServiceURL string            `json:"serviceUrl" binding:"required"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ============== Application Key DTOs ==============

// ApplicationKeyResponse 应用密钥响应（脱敏）
type ApplicationKeyResponse struct {
	ID        uint    `json:"id"`
	KeyID     string  `json:"keyId"`
	Algorithm string  `json:"algorithm"`
	KeyType   string  `json:"keyType"`
	KeySize   int     `json:"keySize"`
	IsPrimary bool    `json:"isPrimary"`
	PublicKey string  `json:"publicKey"`
	ExpiresAt *string `json:"expiresAt"`
	CreatedAt string  `json:"createdAt"`
}

// RotateKeyRequest 轮转密钥请求
type RotateKeyRequest struct {
	Algorithm string `json:"algorithm,omitempty" validate:"omitempty,oneof=RS256 RS384 RS512 HS256 HS384 HS512 ES256 ES384 ES512"`
	KeySize   int    `json:"keySize,omitempty" validate:"omitempty,min=256"`
}

// ResetClientSecretResponse 重置客户端密钥响应
type ResetClientSecretResponse struct {
	ClientSecret string `json:"clientSecret"`
	Message      string `json:"message"`
}
