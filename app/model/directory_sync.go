package model

import "time"

type DirectoryMatchPolicy string

const (
	DirectoryMatchManual          DirectoryMatchPolicy = "manual"
	DirectoryMatchCreateUnmatched DirectoryMatchPolicy = "create_unmatched"
)

const DirectoryDeprovisionBlockIdentityRequireApproval = "block_identity_require_user_approval"

type DirectoryRunStatus string

const (
	DirectoryRunPending      DirectoryRunStatus = "pending"
	DirectoryRunFetching     DirectoryRunStatus = "fetching"
	DirectoryRunPreviewReady DirectoryRunStatus = "preview_ready"
	DirectoryRunApplying     DirectoryRunStatus = "applying"
	DirectoryRunSucceeded    DirectoryRunStatus = "succeeded"
	DirectoryRunFailed       DirectoryRunStatus = "failed"
	DirectoryRunCancelled    DirectoryRunStatus = "cancelled"
)

type DirectoryChangeType string

const (
	DirectoryChangeCreate   DirectoryChangeType = "create"
	DirectoryChangeUpdate   DirectoryChangeType = "update"
	DirectoryChangeMove     DirectoryChangeType = "move"
	DirectoryChangeLink     DirectoryChangeType = "link"
	DirectoryChangeDisable  DirectoryChangeType = "disable"
	DirectoryChangeDelete   DirectoryChangeType = "delete"
	DirectoryChangeConflict DirectoryChangeType = "conflict"
)

type DirectoryObjectType string

const (
	DirectoryObjectUser       DirectoryObjectType = "user"
	DirectoryObjectOrgUnit    DirectoryObjectType = "org_unit"
	DirectoryObjectMembership DirectoryObjectType = "membership"
)

type DirectoryChangeStatus string

const (
	DirectoryChangePending  DirectoryChangeStatus = "pending"
	DirectoryChangeApproved DirectoryChangeStatus = "approved"
	DirectoryChangeRejected DirectoryChangeStatus = "rejected"
	DirectoryChangeApplied  DirectoryChangeStatus = "applied"
	DirectoryChangeFailed   DirectoryChangeStatus = "failed"
)

type DirectorySyncPolicy struct {
	ID                uint                 `gorm:"primaryKey" json:"id"`
	ConnectionID      uint                 `gorm:"not null;uniqueIndex" json:"connectionId"`
	RootDepartmentID  uint                 `gorm:"not null;uniqueIndex" json:"rootDepartmentId"`
	SyncScope         JSONObject           `gorm:"column:sync_scope_json;type:json" json:"syncScope"`
	FieldMapping      JSONObject           `gorm:"column:field_mapping_json;type:json" json:"fieldMapping"`
	FieldOwnership    JSONObject           `gorm:"column:field_ownership_json;type:json" json:"fieldOwnership"`
	MatchPolicy       DirectoryMatchPolicy `gorm:"size:30;not null;default:manual" json:"matchPolicy"`
	DeprovisionPolicy string               `gorm:"size:60;not null" json:"deprovisionPolicy"`
	Enabled           bool                 `gorm:"not null;default:false;index" json:"enabled"`
	CreatedAt         time.Time            `json:"createdAt"`
	UpdatedAt         time.Time            `json:"updatedAt"`
	Connection        *IdentityConnection  `gorm:"foreignKey:ConnectionID" json:"connection,omitempty"`
	RootDepartment    *Department          `gorm:"foreignKey:RootDepartmentID" json:"rootDepartment,omitempty"`
}

func (*DirectorySyncPolicy) TableName() string { return "directory_sync_policies" }

type ExternalOrgUnit struct {
	ID               uint                `gorm:"primaryKey" json:"id"`
	ConnectionID     uint                `gorm:"not null;uniqueIndex:uk_external_org_unit" json:"connectionId"`
	ExternalID       string              `gorm:"size:255;not null;uniqueIndex:uk_external_org_unit" json:"externalId"`
	DepartmentID     *uint               `gorm:"index" json:"departmentId,omitempty"`
	ExternalParentID string              `gorm:"size:255;index" json:"externalParentId"`
	Name             string              `gorm:"size:100;not null" json:"name"`
	SortOrder        int                 `json:"sortOrder"`
	Status           string              `gorm:"size:20;not null;default:active" json:"status"`
	LeaderExternalID string              `gorm:"size:255" json:"leaderExternalId"`
	SnapshotVersion  string              `gorm:"size:64;not null;index" json:"snapshotVersion"`
	LastSeenAt       time.Time           `gorm:"index" json:"lastSeenAt"`
	RawData          JSONObject          `gorm:"type:json" json:"rawData"`
	CreatedAt        time.Time           `json:"createdAt"`
	UpdatedAt        time.Time           `json:"updatedAt"`
	Connection       *IdentityConnection `gorm:"foreignKey:ConnectionID" json:"-"`
	Department       *Department         `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
}

func (*ExternalOrgUnit) TableName() string { return "external_org_units" }

type ExternalMembership struct {
	ID                uint               `gorm:"primaryKey" json:"id"`
	ConnectionID      uint               `gorm:"not null;uniqueIndex:uk_external_membership" json:"connectionId"`
	PrincipalID       uint               `gorm:"not null;uniqueIndex:uk_external_membership;index" json:"principalId"`
	ExternalOrgUnitID uint               `gorm:"not null;uniqueIndex:uk_external_membership;index" json:"externalOrgUnitId"`
	UserID            *uint              `gorm:"index" json:"userId,omitempty"`
	DepartmentID      *uint              `gorm:"index" json:"departmentId,omitempty"`
	IsPrimary         bool               `gorm:"not null;default:false" json:"isPrimary"`
	IsLeader          bool               `gorm:"not null;default:false" json:"isLeader"`
	Position          string             `gorm:"size:100" json:"position"`
	Status            string             `gorm:"size:20;not null;default:active" json:"status"`
	SnapshotVersion   string             `gorm:"size:64;not null;index" json:"snapshotVersion"`
	LastSeenAt        time.Time          `gorm:"index" json:"lastSeenAt"`
	RawData           JSONObject         `gorm:"type:json" json:"rawData"`
	CreatedAt         time.Time          `json:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt"`
	Principal         *ExternalPrincipal `gorm:"foreignKey:PrincipalID" json:"principal,omitempty"`
	OrgUnit           *ExternalOrgUnit   `gorm:"foreignKey:ExternalOrgUnitID" json:"orgUnit,omitempty"`
}

func (*ExternalMembership) TableName() string { return "external_memberships" }

type DirectorySyncRun struct {
	ID               uint                `gorm:"primaryKey" json:"id"`
	ConnectionID     uint                `gorm:"not null;index" json:"connectionId"`
	SnapshotVersion  string              `gorm:"size:64;not null;uniqueIndex" json:"snapshotVersion"`
	TriggerType      string              `gorm:"size:20;not null;default:manual" json:"triggerType"`
	Status           DirectoryRunStatus  `gorm:"size:30;not null;index" json:"status"`
	TotalUsers       int                 `json:"totalUsers"`
	TotalOrgUnits    int                 `json:"totalOrgUnits"`
	TotalMemberships int                 `json:"totalMemberships"`
	ChangeCount      int                 `json:"changeCount"`
	AppliedCount     int                 `json:"appliedCount"`
	ConflictCount    int                 `json:"conflictCount"`
	ErrorMessage     string              `gorm:"type:text" json:"errorMessage"`
	StartedAt        *time.Time          `json:"startedAt,omitempty"`
	FinishedAt       *time.Time          `json:"finishedAt,omitempty"`
	CreatedBy        *uint               `gorm:"index" json:"createdBy,omitempty"`
	CreatedAt        time.Time           `json:"createdAt"`
	UpdatedAt        time.Time           `json:"updatedAt"`
	Connection       *IdentityConnection `gorm:"foreignKey:ConnectionID" json:"connection,omitempty"`
}

func (*DirectorySyncRun) TableName() string { return "directory_sync_runs" }

type DirectorySyncChange struct {
	ID           uint                  `gorm:"primaryKey" json:"id"`
	RunID        uint                  `gorm:"not null;index" json:"runId"`
	ConnectionID uint                  `gorm:"not null;index" json:"connectionId"`
	ChangeType   DirectoryChangeType   `gorm:"size:20;not null;index" json:"changeType"`
	ObjectType   DirectoryObjectType   `gorm:"size:20;not null;index" json:"objectType"`
	ObjectKey    string                `gorm:"size:255;not null;index" json:"objectKey"`
	Status       DirectoryChangeStatus `gorm:"size:20;not null;default:pending;index" json:"status"`
	RiskLevel    string                `gorm:"size:20;not null;default:low;index" json:"riskLevel"`
	BeforeData   JSONObject            `gorm:"column:before_json;type:json" json:"before"`
	AfterData    JSONObject            `gorm:"column:after_json;type:json" json:"after"`
	MatchReason  string                `gorm:"size:255" json:"matchReason"`
	ErrorMessage string                `gorm:"type:text" json:"errorMessage"`
	ReviewedBy   *uint                 `gorm:"index" json:"reviewedBy,omitempty"`
	ReviewedAt   *time.Time            `json:"reviewedAt,omitempty"`
	AppliedAt    *time.Time            `json:"appliedAt,omitempty"`
	CreatedAt    time.Time             `json:"createdAt"`
	UpdatedAt    time.Time             `json:"updatedAt"`
	Run          *DirectorySyncRun     `gorm:"foreignKey:RunID" json:"-"`
}

func (*DirectorySyncChange) TableName() string { return "directory_sync_changes" }
