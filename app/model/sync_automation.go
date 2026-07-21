package model

import "time"

type SyncJobType string

const (
	SyncJobFullSync        SyncJobType = "full_sync"
	SyncJobIncrementalSync SyncJobType = "incremental_sync"
	SyncJobReconcile       SyncJobType = "reconcile"
	SyncJobEventRefresh    SyncJobType = "event_refresh"
	SyncJobApplyChanges    SyncJobType = "apply_changes"
)

type SyncJobStatus string

const (
	SyncJobPending    SyncJobStatus = "pending"
	SyncJobLeased     SyncJobStatus = "leased"
	SyncJobSucceeded  SyncJobStatus = "succeeded"
	SyncJobRetryWait  SyncJobStatus = "retry_wait"
	SyncJobDeadLetter SyncJobStatus = "dead_letter"
	SyncJobCancelled  SyncJobStatus = "cancelled"
)

type SyncJob struct {
	ID             uint                `gorm:"primaryKey" json:"id"`
	ConnectionID   uint                `gorm:"not null;index" json:"connectionId"`
	RunID          *uint               `gorm:"index" json:"runId,omitempty"`
	InboxEventID   *uint               `gorm:"uniqueIndex" json:"inboxEventId,omitempty"`
	JobType        SyncJobType         `gorm:"size:30;not null;index" json:"jobType"`
	Status         SyncJobStatus       `gorm:"size:30;not null;index" json:"status"`
	Payload        JSONObject          `gorm:"column:payload_json;type:json" json:"payload"`
	AvailableAt    time.Time           `gorm:"not null;index" json:"availableAt"`
	LeaseOwner     string              `gorm:"size:120;index" json:"leaseOwner"`
	LeaseExpiresAt *time.Time          `gorm:"index" json:"leaseExpiresAt,omitempty"`
	Attempts       int                 `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts    int                 `gorm:"not null;default:8" json:"maxAttempts"`
	LastError      string              `gorm:"type:text" json:"lastError"`
	FinishedAt     *time.Time          `json:"finishedAt,omitempty"`
	CreatedAt      time.Time           `json:"createdAt"`
	UpdatedAt      time.Time           `json:"updatedAt"`
	Connection     *IdentityConnection `gorm:"foreignKey:ConnectionID" json:"connection,omitempty"`
	Run            *DirectorySyncRun   `gorm:"foreignKey:RunID" json:"run,omitempty"`
}

func (*SyncJob) TableName() string { return "sync_jobs" }

type SyncScheduleType string

const (
	SyncScheduleCron           SyncScheduleType = "cron"
	SyncScheduleDailyReconcile SyncScheduleType = "daily_reconcile"
)

type SyncSchedule struct {
	ID             uint                `gorm:"primaryKey" json:"id"`
	ConnectionID   uint                `gorm:"not null;uniqueIndex:uk_sync_schedule" json:"connectionId"`
	ScheduleType   SyncScheduleType    `gorm:"size:30;not null;uniqueIndex:uk_sync_schedule" json:"scheduleType"`
	CronExpression string              `gorm:"size:100" json:"cronExpression"`
	Timezone       string              `gorm:"size:80;not null;default:UTC" json:"timezone"`
	Enabled        bool                `gorm:"not null;default:true;index" json:"enabled"`
	NextRunAt      *time.Time          `gorm:"index" json:"nextRunAt,omitempty"`
	LastEnqueuedAt *time.Time          `json:"lastEnqueuedAt,omitempty"`
	CreatedAt      time.Time           `json:"createdAt"`
	UpdatedAt      time.Time           `json:"updatedAt"`
	Connection     *IdentityConnection `gorm:"foreignKey:ConnectionID" json:"connection,omitempty"`
}

func (*SyncSchedule) TableName() string { return "sync_schedules" }

type ExternalEventStatus string

const (
	ExternalEventReceived   ExternalEventStatus = "received"
	ExternalEventVerified   ExternalEventStatus = "verified"
	ExternalEventQueued     ExternalEventStatus = "queued"
	ExternalEventProcessed  ExternalEventStatus = "processed"
	ExternalEventFailed     ExternalEventStatus = "failed"
	ExternalEventDeadLetter ExternalEventStatus = "dead_letter"
)

type ExternalEventInbox struct {
	ID                uint                `gorm:"primaryKey" json:"id"`
	ConnectionID      uint                `gorm:"not null;uniqueIndex:uk_external_event" json:"connectionId"`
	ProviderEventID   string              `gorm:"size:255;not null;uniqueIndex:uk_external_event" json:"providerEventId"`
	EventType         string              `gorm:"size:120;not null;index" json:"eventType"`
	ObjectType        string              `gorm:"size:40;index" json:"objectType"`
	ExternalObjectID  string              `gorm:"size:255;index" json:"externalObjectId"`
	Status            ExternalEventStatus `gorm:"size:30;not null;index" json:"status"`
	SignatureValid    bool                `gorm:"not null;default:false" json:"signatureValid"`
	PayloadCiphertext string              `gorm:"type:text;not null" json:"-"`
	PayloadNonce      string              `gorm:"size:255;not null" json:"-"`
	PayloadAlgorithm  string              `gorm:"size:30;not null" json:"-"`
	PayloadKeyVersion int                 `gorm:"not null;default:1" json:"-"`
	Attempts          int                 `gorm:"not null;default:0" json:"attempts"`
	LastError         string              `gorm:"type:text" json:"lastError"`
	ReceivedAt        time.Time           `gorm:"not null;index" json:"receivedAt"`
	ProcessedAt       *time.Time          `json:"processedAt,omitempty"`
	CreatedAt         time.Time           `json:"createdAt"`
	UpdatedAt         time.Time           `json:"updatedAt"`
	Connection        *IdentityConnection `gorm:"foreignKey:ConnectionID" json:"connection,omitempty"`
}

func (*ExternalEventInbox) TableName() string { return "external_event_inbox" }
