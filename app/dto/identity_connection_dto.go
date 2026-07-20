package dto

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

type IdentityConnectionCredentialInput map[string]map[string]any

type CreateIdentityConnectionRequest struct {
	Name         string                            `json:"name" binding:"required,max=100"`
	Identifier   string                            `json:"identifier" binding:"required,max=80"`
	Category     model.ConnectionCategory          `json:"category" binding:"required"`
	ProviderType string                            `json:"providerType" binding:"required,max=40"`
	Description  string                            `json:"description" binding:"max=255"`
	Capabilities model.StringList                  `json:"capabilities"`
	Config       map[string]any                    `json:"config"`
	Credentials  IdentityConnectionCredentialInput `json:"credentials"`
	Icon         string                            `json:"icon" binding:"max=255"`
	SortOrder    int                               `json:"sortOrder"`
	Status       model.ConnectionStatus            `json:"status"`
	IsDefault    bool                              `json:"isDefault"`
	Remark       string                            `json:"remark"`
}

type UpdateIdentityConnectionRequest struct {
	Name         *string                           `json:"name" binding:"omitempty,max=100"`
	Identifier   *string                           `json:"identifier" binding:"omitempty,max=80"`
	Category     *model.ConnectionCategory         `json:"category"`
	ProviderType *string                           `json:"providerType" binding:"omitempty,max=40"`
	Description  *string                           `json:"description" binding:"omitempty,max=255"`
	Capabilities *model.StringList                 `json:"capabilities"`
	Config       map[string]any                    `json:"config"`
	Credentials  IdentityConnectionCredentialInput `json:"credentials"`
	Icon         *string                           `json:"icon" binding:"omitempty,max=255"`
	SortOrder    *int                              `json:"sortOrder"`
	Status       *model.ConnectionStatus           `json:"status"`
	IsDefault    *bool                             `json:"isDefault"`
	Remark       *string                           `json:"remark"`
}

type IdentityConnectionCredentialSummary struct {
	Purpose    model.CredentialPurpose `json:"purpose"`
	Configured bool                    `json:"configured"`
	KeyVersion int                     `json:"keyVersion,omitempty"`
	RotatedAt  *time.Time              `json:"rotatedAt,omitempty"`
}

type IdentityConnectionResponse struct {
	ID              uint                                  `json:"id"`
	Name            string                                `json:"name"`
	Identifier      string                                `json:"identifier"`
	Category        model.ConnectionCategory              `json:"category"`
	ProviderType    string                                `json:"providerType"`
	Description     string                                `json:"description"`
	Capabilities    model.StringList                      `json:"capabilities"`
	Config          map[string]any                        `json:"config"`
	CredentialState []IdentityConnectionCredentialSummary `json:"credentialState"`
	Icon            string                                `json:"icon"`
	SortOrder       int                                   `json:"sortOrder"`
	Status          model.ConnectionStatus                `json:"status"`
	IsDefault       bool                                  `json:"isDefault"`
	LastCheckedAt   *time.Time                            `json:"lastCheckedAt,omitempty"`
	LastCheckStatus string                                `json:"lastCheckStatus,omitempty"`
	LastCheckError  string                                `json:"lastCheckError,omitempty"`
	Remark          string                                `json:"remark"`
	CreatedAt       time.Time                             `json:"createdAt"`
	UpdatedAt       time.Time                             `json:"updatedAt"`
}

type PublicIdentityConnectionInfo struct {
	Identifier   string                   `json:"identifier"`
	Name         string                   `json:"name"`
	Category     model.ConnectionCategory `json:"category"`
	ProviderType string                   `json:"providerType"`
	Capabilities model.StringList         `json:"capabilities"`
	Icon         string                   `json:"icon"`
	SortOrder    int                      `json:"sortOrder"`
	IsDefault    bool                     `json:"isDefault"`
}

type IdentityConnectionBatchRequest struct {
	IDs       []uint `json:"ids" binding:"required,min=1"`
	Operation string `json:"operation" binding:"required,oneof=enable disable delete"`
}

type IdentityConnectionStatsResponse struct {
	TotalConnections      int64 `json:"totalConnections"`
	EnabledConnections    int64 `json:"enabledConnections"`
	SocialConnections     int64 `json:"socialConnections"`
	EnterpriseConnections int64 `json:"enterpriseConnections"`
	TotalBindings         int64 `json:"totalBindings"`
}

type ExternalIdentityResponse struct {
	ID           uint                     `json:"id"`
	ConnectionID uint                     `json:"connectionId"`
	Connection   string                   `json:"connection"`
	Identifier   string                   `json:"identifier"`
	Category     model.ConnectionCategory `json:"category"`
	ProviderType string                   `json:"providerType"`
	Icon         string                   `json:"icon"`
	Status       model.PrincipalStatus    `json:"status"`
	DisplayName  string                   `json:"displayName"`
	Avatar       string                   `json:"avatar"`
	Email        string                   `json:"email"`
	Phone        string                   `json:"phone"`
	Identifiers  map[string][]string      `json:"identifiers"`
	LastLoginAt  *time.Time               `json:"lastLoginAt,omitempty"`
	LoginCount   int64                    `json:"loginCount"`
	CreatedAt    time.Time                `json:"createdAt"`
}
