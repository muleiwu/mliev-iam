package dto

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

type CreateIdentityProviderRequest struct {
	Name         string                       `json:"name" binding:"required,max=50"`
	Identifier   string                       `json:"identifier" binding:"required,max=50"`
	Type         model.IdentityProviderType   `json:"type" binding:"required"`
	Description  string                       `json:"description" binding:"max=255"`
	ClientID     string                       `json:"clientId" binding:"required,max=255"`
	ClientSecret string                       `json:"clientSecret" binding:"max=255"`
	AuthURL      string                       `json:"authUrl" binding:"max=255"`
	TokenURL     string                       `json:"tokenUrl" binding:"max=255"`
	UserInfoURL  string                       `json:"userInfoUrl" binding:"max=255"`
	RedirectURL  string                       `json:"redirectUrl" binding:"required,max=255"`
	Scopes       string                       `json:"scopes"`
	Icon         string                       `json:"icon" binding:"max=255"`
	SortOrder    int                          `json:"sortOrder"`
	Status       model.IdentityProviderStatus `json:"status"`
	IsDefault    bool                         `json:"isDefault"`
	Attributes   map[string]any               `json:"attributes,omitempty"`
	Remark       string                       `json:"remark"`
}

type UpdateIdentityProviderRequest struct {
	Name         *string                       `json:"name" binding:"omitempty,max=50"`
	Identifier   *string                       `json:"identifier" binding:"omitempty,max=50"`
	Type         *model.IdentityProviderType   `json:"type" binding:"omitempty"`
	Description  *string                       `json:"description" binding:"omitempty,max=255"`
	ClientID     *string                       `json:"clientId" binding:"omitempty,max=255"`
	ClientSecret *string                       `json:"clientSecret" binding:"omitempty,max=255"`
	AuthURL      *string                       `json:"authUrl" binding:"omitempty,max=255"`
	TokenURL     *string                       `json:"tokenUrl" binding:"omitempty,max=255"`
	UserInfoURL  *string                       `json:"userInfoUrl" binding:"omitempty,max=255"`
	RedirectURL  *string                       `json:"redirectUrl" binding:"omitempty,max=255"`
	Scopes       *string                       `json:"scopes"`
	Icon         *string                       `json:"icon" binding:"omitempty,max=255"`
	SortOrder    *int                          `json:"sortOrder"`
	Status       *model.IdentityProviderStatus `json:"status"`
	IsDefault    *bool                         `json:"isDefault"`
	Attributes   map[string]any                `json:"attributes,omitempty"`
	Remark       *string                       `json:"remark"`
}

type IdentityProviderResponse struct {
	ID          uint                         `json:"id"`
	Name        string                       `json:"name"`
	Identifier  string                       `json:"identifier"`
	Type        model.IdentityProviderType   `json:"type"`
	Description string                       `json:"description"`
	ClientID    string                       `json:"clientId"`
	AuthURL     string                       `json:"authUrl"`
	TokenURL    string                       `json:"tokenUrl"`
	UserInfoURL string                       `json:"userInfoUrl"`
	RedirectURL string                       `json:"redirectUrl"`
	Scopes      string                       `json:"scopes"`
	Icon        string                       `json:"icon"`
	SortOrder   int                          `json:"sortOrder"`
	Status      model.IdentityProviderStatus `json:"status"`
	IsDefault   bool                         `json:"isDefault"`
	Attributes  map[string]any               `json:"attributes,omitempty"`
	Remark      string                       `json:"remark"`
	CreatedAt   time.Time                    `json:"createdAt"`
	UpdatedAt   time.Time                    `json:"updatedAt"`
}

type IdentityProviderListItem struct {
	ID          uint                         `json:"id"`
	Name        string                       `json:"name"`
	Identifier  string                       `json:"identifier"`
	Type        model.IdentityProviderType   `json:"type"`
	Icon        string                       `json:"icon"`
	Status      model.IdentityProviderStatus `json:"status"`
	IsDefault   bool                         `json:"isDefault"`
	SortOrder   int                          `json:"sortOrder"`
	Description string                       `json:"description"`
}

type PublicIdentityProviderInfo struct {
	Identifier string                     `json:"identifier"`
	Name       string                     `json:"name"`
	Icon       string                     `json:"icon"`
	SortOrder  int                        `json:"sortOrder"`
	IsDefault  bool                       `json:"isDefault"`
	Type       model.IdentityProviderType `json:"type"`
}

type IdentityProviderBatchRequest struct {
	IDs       []uint `json:"ids" binding:"required,min=1"`
	Operation string `json:"operation" binding:"required,oneof=enable disable delete"`
}

type UserIdentityAccountResponse struct {
	ID             uint           `json:"id"`
	UserID         uint           `json:"userId"`
	ProviderID     uint           `json:"providerId"`
	ProviderName   string         `json:"providerName"`
	ProviderIcon   string         `json:"providerIcon"`
	ExternalUserID string         `json:"externalUserId"`
	UnionID        string         `json:"unionId,omitempty"`
	TenantID       string         `json:"tenantId,omitempty"`
	UserInfo       map[string]any `json:"userInfo,omitempty"`
	LastUsedAt     *time.Time     `json:"lastUsedAt,omitempty"`
	LoginCount     int64          `json:"loginCount"`
	CreatedAt      time.Time      `json:"createdAt"`
}

type IdentityProviderStatsResponse struct {
	TotalProviders    int64 `json:"totalProviders"`
	EnabledProviders  int64 `json:"enabledProviders"`
	DisabledProviders int64 `json:"disabledProviders"`
	TotalBindings     int64 `json:"totalBindings"`
}
