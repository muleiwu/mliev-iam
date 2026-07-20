package service

import (
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

type SyncConnectionHealth struct {
	ConnectionID       uint                                  `json:"connectionId"`
	ConnectionName     string                                `json:"connectionName"`
	ProviderType       string                                `json:"providerType"`
	LastRunID          *uint                                 `json:"lastRunId,omitempty"`
	LastRunStatus      model.DirectoryRunStatus              `json:"lastRunStatus,omitempty"`
	LastRunAt          *time.Time                            `json:"lastRunAt,omitempty"`
	NextScheduledAt    *time.Time                            `json:"nextScheduledAt,omitempty"`
	PendingChanges     int64                                 `json:"pendingChanges"`
	DeadLetterJobs     int64                                 `json:"deadLetterJobs"`
	LastConnectionErr  string                                `json:"lastConnectionError,omitempty"`
	CapabilityHealth   []model.IdentityConnectionHealthCheck `json:"capabilityHealth"`
	LeaseOwner         string                                `json:"leaseOwner,omitempty"`
	LeaseExpiresAt     *time.Time                            `json:"leaseExpiresAt,omitempty"`
	PrincipalConflicts int64                                 `json:"principalConflicts"`
	SupersededRuns     int64                                 `json:"supersededRuns"`
}

type SyncCenterSummary struct {
	EnabledConnections int64                  `json:"enabledConnections"`
	RunningJobs        int64                  `json:"runningJobs"`
	QueuedJobs         int64                  `json:"queuedJobs"`
	PendingApprovals   int64                  `json:"pendingApprovals"`
	DeadLetterJobs     int64                  `json:"deadLetterJobs"`
	FailedEvents       int64                  `json:"failedEvents"`
	PrincipalConflicts int64                  `json:"principalConflicts"`
	Connections        []SyncConnectionHealth `json:"connections"`
}

type SyncCenterService struct{ db *gorm.DB }

func NewSyncCenterService(db *gorm.DB) *SyncCenterService { return &SyncCenterService{db: db} }

func (s *SyncCenterService) Summary() (*SyncCenterSummary, error) {
	result := &SyncCenterSummary{}
	counts := []struct {
		target *int64
		model  any
		where  string
		args   []any
	}{
		{&result.EnabledConnections, &model.DirectorySyncPolicy{}, "enabled = ?", []any{true}},
		{&result.RunningJobs, &model.SyncJob{}, "status = ?", []any{model.SyncJobLeased}},
		{&result.QueuedJobs, &model.SyncJob{}, "status IN ?", []any{[]model.SyncJobStatus{model.SyncJobPending, model.SyncJobRetryWait}}},
		{&result.PendingApprovals, &model.DirectorySyncChange{}, "status = ?", []any{model.DirectoryChangePending}},
		{&result.DeadLetterJobs, &model.SyncJob{}, "status = ?", []any{model.SyncJobDeadLetter}},
		{&result.FailedEvents, &model.ExternalEventInbox{}, "status IN ?", []any{[]model.ExternalEventStatus{model.ExternalEventFailed, model.ExternalEventDeadLetter}}},
		{&result.PrincipalConflicts, &model.ExternalPrincipalConflict{}, "status = ?", []any{model.PrincipalConflictPending}},
	}
	for _, item := range counts {
		if err := s.db.Model(item.model).Where(item.where, item.args...).Count(item.target).Error; err != nil {
			return nil, err
		}
	}
	var policies []model.DirectorySyncPolicy
	if err := s.db.Preload("Connection").Where("enabled = ?", true).Find(&policies).Error; err != nil {
		return nil, err
	}
	result.Connections = make([]SyncConnectionHealth, 0, len(policies))
	for _, policy := range policies {
		health := SyncConnectionHealth{ConnectionID: policy.ConnectionID}
		if policy.Connection != nil {
			health.ConnectionName = policy.Connection.Name
			health.ProviderType = policy.Connection.ProviderType
			health.LastConnectionErr = policy.Connection.LastCheckError
		}
		var run model.DirectorySyncRun
		if err := s.db.Where("connection_id = ?", policy.ConnectionID).Order("id DESC").First(&run).Error; err == nil {
			health.LastRunID = &run.ID
			health.LastRunStatus = run.Status
			health.LastRunAt = &run.CreatedAt
		}
		var schedule model.SyncSchedule
		if err := s.db.Where("connection_id = ? AND enabled = ?", policy.ConnectionID, true).Order("next_run_at ASC").First(&schedule).Error; err == nil {
			health.NextScheduledAt = schedule.NextRunAt
		}
		_ = s.db.Model(&model.DirectorySyncChange{}).Where("connection_id = ? AND status = ?", policy.ConnectionID, model.DirectoryChangePending).Count(&health.PendingChanges).Error
		_ = s.db.Model(&model.SyncJob{}).Where("connection_id = ? AND status = ?", policy.ConnectionID, model.SyncJobDeadLetter).Count(&health.DeadLetterJobs).Error
		_ = s.db.Model(&model.ExternalPrincipalConflict{}).Where("connection_id = ? AND status = ?", policy.ConnectionID, model.PrincipalConflictPending).Count(&health.PrincipalConflicts).Error
		_ = s.db.Model(&model.DirectorySyncRun{}).Where("connection_id = ? AND status = ?", policy.ConnectionID, model.DirectoryRunSuperseded).Count(&health.SupersededRuns).Error
		_ = s.db.Where("connection_id = ?", policy.ConnectionID).Order("capability ASC").Find(&health.CapabilityHealth).Error
		var lease model.DirectoryOperationLease
		if err := s.db.Where("lease_key = ? AND lease_expires_at > ?", fmt.Sprintf("connection:%d", policy.ConnectionID), time.Now()).First(&lease).Error; err == nil {
			health.LeaseOwner, health.LeaseExpiresAt = lease.Owner, &lease.LeaseExpiresAt
		}
		result.Connections = append(result.Connections, health)
	}
	return result, nil
}
