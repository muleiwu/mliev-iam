package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

type ConnectionCategory string

const (
	ConnectionCategorySocial     ConnectionCategory = "social"
	ConnectionCategoryEnterprise ConnectionCategory = "enterprise"
)

type ConnectionStatus string

const (
	ConnectionStatusDisabled ConnectionStatus = "disabled"
	ConnectionStatusEnabled  ConnectionStatus = "enabled"
)

type ConnectionCapability string

const (
	ConnectionCapabilityAuthentication ConnectionCapability = "authentication"
	ConnectionCapabilityDirectorySync  ConnectionCapability = "directory_sync"
	ConnectionCapabilityEvents         ConnectionCapability = "events"
)

type CredentialPurpose string

const (
	CredentialPurposeAuthentication CredentialPurpose = "authentication"
	CredentialPurposeDirectory      CredentialPurpose = "directory"
	CredentialPurposeEvents         CredentialPurpose = "events"
)

type StringList []string

func (s *StringList) Scan(value any) error {
	if value == nil {
		*s = StringList{}
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("failed to scan string list")
	}
	if len(b) == 0 {
		*s = StringList{}
		return nil
	}
	return json.Unmarshal(b, s)
}

func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s StringList) Contains(value string) bool {
	for _, item := range s {
		if item == value {
			return true
		}
	}
	return false
}

type IdentityConnection struct {
	ID              uint               `gorm:"primaryKey" json:"id"`
	Name            string             `gorm:"size:100;not null" json:"name"`
	Identifier      string             `gorm:"uniqueIndex;size:80;not null" json:"identifier"`
	Category        ConnectionCategory `gorm:"size:20;not null;index" json:"category"`
	ProviderType    string             `gorm:"size:40;not null;index" json:"providerType"`
	Description     string             `gorm:"size:255" json:"description"`
	Capabilities    StringList         `gorm:"type:json;not null" json:"capabilities"`
	Config          JSONObject         `gorm:"column:config_json;type:json" json:"config"`
	Icon            string             `gorm:"size:255" json:"icon"`
	SortOrder       int                `gorm:"default:0" json:"sortOrder"`
	Status          ConnectionStatus   `gorm:"size:20;not null;default:disabled;index" json:"status"`
	IsDefault       bool               `gorm:"default:false" json:"isDefault"`
	LastCheckedAt   *time.Time         `json:"lastCheckedAt,omitempty"`
	LastCheckStatus string             `gorm:"size:20" json:"lastCheckStatus,omitempty"`
	LastCheckError  string             `gorm:"type:text" json:"lastCheckError,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt     `gorm:"index" json:"-"`
	Remark          string             `gorm:"type:text" json:"remark"`
}

func (*IdentityConnection) TableName() string { return "identity_connections" }

func (c *IdentityConnection) IsEnabled() bool {
	return c != nil && c.Status == ConnectionStatusEnabled
}

func (c *IdentityConnection) HasCapability(capability ConnectionCapability) bool {
	return c != nil && c.Capabilities.Contains(string(capability))
}

type IdentityConnectionCredential struct {
	ID           uint                `gorm:"primaryKey" json:"id"`
	ConnectionID uint                `gorm:"not null;uniqueIndex:uk_connection_credential" json:"connectionId"`
	Purpose      CredentialPurpose   `gorm:"size:30;not null;uniqueIndex:uk_connection_credential" json:"purpose"`
	Ciphertext   string              `gorm:"type:text;not null" json:"-"`
	Nonce        string              `gorm:"size:255;not null" json:"-"`
	Algorithm    string              `gorm:"size:30;not null" json:"algorithm"`
	KeyVersion   int                 `gorm:"not null;default:1" json:"keyVersion"`
	RotatedAt    time.Time           `json:"rotatedAt"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
	Connection   *IdentityConnection `gorm:"foreignKey:ConnectionID" json:"-"`
}

func (*IdentityConnectionCredential) TableName() string {
	return "identity_connection_credentials"
}
