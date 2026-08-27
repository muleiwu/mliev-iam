package dto

// ============== Application DTOs ==============

// CreateApplicationRequest 创建应用请求
type CreateApplicationRequest struct {
	Name           string                 `json:"name" binding:"required" validate:"max=100"`
	Description    string                 `json:"description" validate:"max=500"`
	Icon           string                 `json:"icon" validate:"max=255"`
	ProtocolType   string                 `json:"protocolType" binding:"required,oneof=oidc saml cas oauth2"`
	ProtocolConfig map[string]interface{} `json:"protocolConfig" binding:"required"`
}

// UpdateApplicationRequest 更新应用请求
type UpdateApplicationRequest struct {
	Name           *string                 `json:"name,omitempty" validate:"max=100"`
	Description    *string                 `json:"description,omitempty" validate:"max=500"`
	Icon           *string                 `json:"icon,omitempty" validate:"omitempty,max=255"`
	ProtocolConfig *map[string]interface{} `json:"protocolConfig,omitempty"`
	IsActive       *bool                   `json:"isActive,omitempty"`
}

// ApplicationResponse 应用响应
type ApplicationResponse struct {
	ID             uint                   `json:"id"`
	UUID           string                 `json:"uuid"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Icon           string                 `json:"icon"`
	ProtocolType   string                 `json:"protocolType"`
	ProtocolConfig map[string]interface{} `json:"protocolConfig"`
	IsActive       bool                   `json:"isActive"`
	IsVerified     bool                   `json:"isVerified"`
	IsProtected    bool                   `json:"isProtected"`
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

// ============== Application Access Scope DTOs ==============

// ApplicationAccessScopeRequest 应用授权范围请求
type ApplicationAccessScopeRequest struct {
	UserIDs               []uint `json:"userIds"`
	DepartmentIDs         []uint `json:"departmentIds"`
	RoleIDs               []uint `json:"roleIds"`
	IncludeSubDepartments bool   `json:"includeSubDepartments"`
}

// ApplicationAccessScopeQuery 应用授权范围查询参数
type ApplicationAccessScopeQuery struct {
	UserPage     int `form:"userPage" binding:"omitempty,min=1"`
	UserPageSize int `form:"userPageSize" binding:"omitempty,min=1,max=100"`
}

// ApplicationAccessScopeResponse 应用授权范围响应
type ApplicationAccessScopeResponse struct {
	UserIDs               []uint                `json:"userIds"`
	DepartmentIDs         []uint                `json:"departmentIds"`
	RoleIDs               []uint                `json:"roleIds"`
	IncludeSubDepartments bool                  `json:"includeSubDepartments"`
	Users                 []AccessScopeUserInfo `json:"users"` // 用户详细信息
	UserPagination        AccessScopePagination `json:"userPagination"`
	Departments           []AccessScopeDeptInfo `json:"departments"` // 部门详细信息
	Roles                 []AccessScopeRoleInfo `json:"roles"`       // 角色详细信息
}

// AccessScopePagination 授权范围明细分页信息
type AccessScopePagination struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

// AccessScopeUserInfo 授权范围用户信息
type AccessScopeUserInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	RealName string `json:"realName"`
	Email    string `json:"email"`
}

// AccessScopeDeptInfo 授权范围部门信息
type AccessScopeDeptInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
	Path string `json:"path"`
}

// AccessScopeRoleInfo 授权范围角色信息
type AccessScopeRoleInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}
