package dto

import "time"

// ============== OIDC Client DTOs ==============

// CreateOIDCClientRequest 创建OIDC客户端请求
type CreateOIDCClientRequest struct {
	Name         string   `json:"name" binding:"required" validate:"max=100"`
	Description  string   `json:"description" validate:"max=500"`
	RedirectURIs []string `json:"redirectUris" binding:"required,min=1"`
	GrantTypes   []string `json:"grantTypes" binding:"required,min=1"`
	Scopes       []string `json:"scopes" binding:"required,min=1"`
	ClientType   string   `json:"clientType" binding:"oneof=confidential public"`
	RequirePKCE  bool     `json:"requirePkce"`
}

// UpdateOIDCClientRequest 更新OIDC客户端请求
type UpdateOIDCClientRequest struct {
	Name         *string  `json:"name,omitempty" validate:"max=100"`
	Description  *string  `json:"description,omitempty" validate:"max=500"`
	RedirectURIs []string `json:"redirectUris,omitempty" validate:"min=1"`
	GrantTypes   []string `json:"grantTypes,omitempty" validate:"min=1"`
	Scopes       []string `json:"scopes,omitempty" validate:"min=1"`
	ClientType   *string  `json:"clientType,omitempty" validate:"oneof=confidential public"`
	RequirePKCE  *bool    `json:"requirePkce,omitempty"`
	IsActive     *bool    `json:"isActive,omitempty"`
}

// OIDCClientResponse OIDC客户端响应
type OIDCClientResponse struct {
	ID           uint     `json:"id"`
	ClientID     string   `json:"clientId"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	RedirectURIs []string `json:"redirectUris"`
	GrantTypes   []string `json:"grantTypes"`
	Scopes       []string `json:"scopes"`
	ClientType   string   `json:"clientType"`
	RequirePKCE  bool     `json:"requirePkce"`
	IsActive     bool     `json:"isActive"`
	IsVerified   bool     `json:"isVerified"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// OIDCClientWithSecretResponse 包含客户端密钥的响应（仅在创建时返回）
type OIDCClientWithSecretResponse struct {
	OIDCClientResponse
	ClientSecret string `json:"clientSecret"`
}

// ============== OIDC Authorization DTOs ==============

// AuthorizeRequest 授权请求
type AuthorizeRequest struct {
	ResponseType        string `form:"response_type" binding:"required"`
	ClientID            string `form:"client_id" binding:"required"`
	RedirectURI         string `form:"redirect_uri" binding:"required"`
	Scope               string `form:"scope" binding:"required"`
	State               string `form:"state"`
	Nonce               string `form:"nonce"`
	CodeChallenge       string `form:"code_challenge"`
	CodeChallengeMethod string `form:"code_challenge_method"`
	Prompt              string `form:"prompt"`
	MaxAge              string `form:"max_age"`
	UILocales           string `form:"ui_locales"`
	IDTokenHint         string `form:"id_token_hint"`
	LoginHint           string `form:"login_hint"`
	ACRValues           string `form:"acr_values"`
}

// AuthorizeResponse 授权响应
type AuthorizeResponse struct {
	Code             string `json:"code,omitempty"`
	State            string `json:"state,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// ============== OIDC Token DTOs ==============

// TokenRequest 令牌请求
type TokenRequest struct {
	GrantType    string `form:"grant_type" binding:"required"`
	Code         string `form:"code"`
	RedirectURI  string `form:"redirect_uri"`
	ClientID     string `form:"client_id"`
	ClientSecret string `form:"client_secret"`
	RefreshToken string `form:"refresh_token"`
	Scope        string `form:"scope"`
	CodeVerifier string `form:"code_verifier"`
}

// TokenResponse 令牌响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// TokenErrorResponse 令牌错误响应
type TokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorURI         string `json:"error_uri,omitempty"`
}

// ============== OIDC UserInfo DTOs ==============

// UserInfoResponse 用户信息响应
type UserInfoResponse struct {
	Sub                 string   `json:"sub"`
	Name                string   `json:"name,omitempty"`
	GivenName           string   `json:"given_name,omitempty"`
	FamilyName          string   `json:"family_name,omitempty"`
	MiddleName          string   `json:"middle_name,omitempty"`
	Nickname            string   `json:"nickname,omitempty"`
	PreferredUsername   string   `json:"preferred_username,omitempty"`
	Profile             string   `json:"profile,omitempty"`
	Picture             string   `json:"picture,omitempty"`
	Website             string   `json:"website,omitempty"`
	Email               string   `json:"email,omitempty"`
	EmailVerified       bool     `json:"email_verified,omitempty"`
	Gender              string   `json:"gender,omitempty"`
	Birthdate           string   `json:"birthdate,omitempty"`
	Zoneinfo            string   `json:"zoneinfo,omitempty"`
	Locale              string   `json:"locale,omitempty"`
	PhoneNumber         string   `json:"phone_number,omitempty"`
	PhoneNumberVerified bool     `json:"phone_number_verified,omitempty"`
	Address             *Address `json:"address,omitempty"`
	UpdatedAt           int64    `json:"updated_at,omitempty"`
}

// Address 地址信息
type Address struct {
	Formatted     string `json:"formatted,omitempty"`
	StreetAddress string `json:"street_address,omitempty"`
	Locality      string `json:"locality,omitempty"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	Country       string `json:"country,omitempty"`
}

// ============== OIDC Discovery DTOs ==============

// OIDCDiscoveryResponse OIDC发现文档响应
type OIDCDiscoveryResponse struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserInfoEndpoint                  string   `json:"userinfo_endpoint"`
	JwksURI                           string   `json:"jwks_uri"`
	RevocationEndpoint                string   `json:"revocation_endpoint,omitempty"`
	IntrospectionEndpoint             string   `json:"introspection_endpoint,omitempty"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
}

// ============== OIDC Introspection DTOs ==============

// IntrospectionRequest 令牌内省请求
type IntrospectionRequest struct {
	Token         string `form:"token" binding:"required"`
	TokenTypeHint string `form:"token_type_hint"`
	ClientID      string `form:"client_id"`
	ClientSecret  string `form:"client_secret"`
}

// IntrospectionResponse 令牌内省响应
type IntrospectionResponse struct {
	Active    bool   `json:"active"`
	Sub       string `json:"sub,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
	Username  string `json:"username,omitempty"`
	TokenType string `json:"token_type,omitempty"`
	Exp       int64  `json:"exp,omitempty"`
	Iat       int64  `json:"iat,omitempty"`
	Nbf       int64  `json:"nbf,omitempty"`
	Aud       string `json:"aud,omitempty"`
	Iss       string `json:"iss,omitempty"`
	Jti       string `json:"jti,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

// ============== OIDC Revocation DTOs ==============

// RevocationRequest 令牌撤销请求
type RevocationRequest struct {
	Token         string `form:"token" binding:"required"`
	TokenTypeHint string `form:"token_type_hint"`
	ClientID      string `form:"client_id"`
	ClientSecret  string `form:"client_secret"`
}

// ============== OIDC Session DTOs ==============

// OIDCSessionInfo OIDC会话信息
type OIDCSessionInfo struct {
	UserID      uint      `json:"userId"`
	ClientID    string    `json:"clientId"`
	Scopes      []string  `json:"scopes"`
	Nonce       string    `json:"nonce,omitempty"`
	MaxAge      int       `json:"maxAge,omitempty"`
	AuthTime    time.Time `json:"authTime"`
	RedirectURI string    `json:"redirectUri"`
	State       string    `json:"state,omitempty"`
}
