package model

import "time"

type PrincipalConflictStatus string

const (
	PrincipalConflictPending  PrincipalConflictStatus = "pending"
	PrincipalConflictResolved PrincipalConflictStatus = "resolved"
	PrincipalConflictRejected PrincipalConflictStatus = "rejected"
)

// ExternalPrincipalConflict records an ambiguous identifier set. Authentication
// and directory synchronization must stop instead of silently merging people.
type ExternalPrincipalConflict struct {
	ID                    uint                    `gorm:"primaryKey" json:"id"`
	ConnectionID          uint                    `gorm:"not null;index" json:"connectionId"`
	IdentifierSet         JSONObject              `gorm:"column:identifier_json;type:json;not null" json:"identifiers"`
	CandidatePrincipalIDs StringList              `gorm:"column:candidate_principal_ids_json;type:json;not null" json:"candidatePrincipalIds"`
	Status                PrincipalConflictStatus `gorm:"size:20;not null;default:pending;index" json:"status"`
	CanonicalPrincipalID  *uint                   `gorm:"index" json:"canonicalPrincipalId,omitempty"`
	TargetUserID          *uint                   `gorm:"index" json:"targetUserId,omitempty"`
	ResolutionNote        string                  `gorm:"type:text" json:"resolutionNote"`
	ReviewedBy            *uint                   `gorm:"index" json:"reviewedBy,omitempty"`
	ReviewedAt            *time.Time              `json:"reviewedAt,omitempty"`
	CreatedAt             time.Time               `json:"createdAt"`
	UpdatedAt             time.Time               `json:"updatedAt"`
}

func (*ExternalPrincipalConflict) TableName() string { return "external_principal_conflicts" }

type CapabilityHealthStatus string

const (
	CapabilityHealthHealthy CapabilityHealthStatus = "healthy"
	CapabilityHealthFailed  CapabilityHealthStatus = "failed"
	CapabilityHealthUnknown CapabilityHealthStatus = "unknown"
)

type IdentityConnectionHealthCheck struct {
	ID           uint                   `gorm:"primaryKey" json:"id"`
	ConnectionID uint                   `gorm:"not null;uniqueIndex:uk_connection_capability_health" json:"connectionId"`
	Capability   ConnectionCapability   `gorm:"size:30;not null;uniqueIndex:uk_connection_capability_health" json:"capability"`
	Status       CapabilityHealthStatus `gorm:"size:20;not null;index" json:"status"`
	ErrorMessage string                 `gorm:"type:text" json:"errorMessage"`
	Details      JSONObject             `gorm:"column:details_json;type:json" json:"details"`
	CheckedAt    time.Time              `gorm:"not null;index" json:"checkedAt"`
	CreatedAt    time.Time              `json:"createdAt"`
	UpdatedAt    time.Time              `json:"updatedAt"`
}

func (*IdentityConnectionHealthCheck) TableName() string {
	return "identity_connection_health_checks"
}
