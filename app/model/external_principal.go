package model

import (
	"time"

	"gorm.io/gorm"
)

type PrincipalStatus string

const (
	PrincipalStatusActive    PrincipalStatus = "active"
	PrincipalStatusBlocked   PrincipalStatus = "blocked"
	PrincipalStatusUnmatched PrincipalStatus = "unmatched"
)

type IdentifierNamespace string

const (
	IdentifierNamespaceLoginSubject    IdentifierNamespace = "login_subject"
	IdentifierNamespaceDirectoryUserID IdentifierNamespace = "directory_user_id"
	IdentifierNamespaceOpenID          IdentifierNamespace = "open_id"
	IdentifierNamespaceOpenUserID      IdentifierNamespace = "open_userid"
	IdentifierNamespaceUnionID         IdentifierNamespace = "union_id"
)

type ExternalPrincipal struct {
	ID           uint                `gorm:"primaryKey" json:"id"`
	ConnectionID uint                `gorm:"not null;index;uniqueIndex:uk_principal_connection_user" json:"connectionId"`
	UserID       *uint               `gorm:"index;uniqueIndex:uk_principal_connection_user" json:"userId,omitempty"`
	Status       PrincipalStatus     `gorm:"size:20;not null;default:unmatched;index" json:"status"`
	DisplayName  string              `gorm:"size:100" json:"displayName"`
	Avatar       string              `gorm:"size:255" json:"avatar"`
	Email        string              `gorm:"size:100;index" json:"email"`
	Phone        string              `gorm:"size:30;index" json:"phone"`
	RawProfile   JSONObject          `gorm:"type:json" json:"rawProfile,omitempty"`
	LastLoginAt  *time.Time          `json:"lastLoginAt,omitempty"`
	LastSyncedAt *time.Time          `json:"lastSyncedAt,omitempty"`
	LoginCount   int64               `gorm:"not null;default:0" json:"loginCount"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt      `gorm:"index" json:"-"`
	Connection   *IdentityConnection `gorm:"foreignKey:ConnectionID" json:"connection,omitempty"`
	User         *User               `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (*ExternalPrincipal) TableName() string { return "external_principals" }

type ExternalIdentifier struct {
	ID           uint                `gorm:"primaryKey" json:"id"`
	ConnectionID uint                `gorm:"not null;index;uniqueIndex:uk_external_identifier" json:"connectionId"`
	PrincipalID  uint                `gorm:"not null;index" json:"principalId"`
	Namespace    IdentifierNamespace `gorm:"size:40;not null;uniqueIndex:uk_external_identifier" json:"namespace"`
	Value        string              `gorm:"size:255;not null;uniqueIndex:uk_external_identifier" json:"value"`
	IsPrimary    bool                `gorm:"not null;default:false" json:"isPrimary"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
	Principal    *ExternalPrincipal  `gorm:"foreignKey:PrincipalID" json:"-"`
}

func (*ExternalIdentifier) TableName() string { return "external_identifiers" }
