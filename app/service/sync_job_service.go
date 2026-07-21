package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

const (
	DefaultSyncJobMaxAttempts = 8
	MaxSyncJobBackoff         = 6 * time.Hour
)

var ErrSyncNotRunnable = errors.New("同步连接当前不可执行")

type SyncJobListFilter struct {
	ConnectionID uint
	JobType      model.SyncJobType
	Status       model.SyncJobStatus
	Page         int
	PageSize     int
}

type SyncJobService struct{ db *gorm.DB }

func NewSyncJobService(db *gorm.DB) *SyncJobService { return &SyncJobService{db: db} }

func (s *SyncJobService) Enqueue(connectionID uint, jobType model.SyncJobType, payload map[string]any, availableAt time.Time, inboxEventID *uint) (*model.SyncJob, error) {
	if !validSyncJobType(jobType) {
		return nil, fmt.Errorf("不支持的同步任务类型 %q", jobType)
	}
	if availableAt.IsZero() {
		availableAt = time.Now()
	}
	if err := s.ValidateRunnable(connectionID, jobType); err != nil {
		return nil, err
	}
	job := &model.SyncJob{
		ConnectionID: connectionID, InboxEventID: inboxEventID, JobType: jobType,
		Status: model.SyncJobPending, Payload: model.JSONObject(payload), AvailableAt: availableAt,
		MaxAttempts: DefaultSyncJobMaxAttempts,
	}
	if err := s.db.Create(job).Error; err != nil {
		if inboxEventID != nil {
			var existing model.SyncJob
			if loadErr := s.db.Where("inbox_event_id = ?", *inboxEventID).First(&existing).Error; loadErr == nil {
				return &existing, nil
			}
		}
		return nil, err
	}
	return job, nil
}

// Claim uses an atomic conditional update rather than relying on a dialect-
// specific SKIP LOCKED clause. It therefore provides the same lease semantics
// on MySQL, PostgreSQL and a single SQLite worker.
func (s *SyncJobService) Claim(ctx context.Context, owner string, leaseDuration time.Duration, now time.Time) (*model.SyncJob, error) {
	if owner == "" {
		return nil, fmt.Errorf("租约 owner 不能为空")
	}
	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	for candidateAttempt := 0; candidateAttempt < 20; candidateAttempt++ {
		var candidate model.SyncJob
		err := s.db.WithContext(ctx).
			Where("attempts < max_attempts AND available_at <= ? AND ((status IN ?) OR (status = ? AND lease_expires_at <= ?))", now, []model.SyncJobStatus{model.SyncJobPending, model.SyncJobRetryWait}, model.SyncJobLeased, now).
			Order("available_at ASC, id ASC").First(&candidate).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A worker may die after consuming its last allowed attempt. Expired
			// leases at the limit are terminal and must never be claimed forever.
			if deadErr := s.deadLetterExhaustedLeases(ctx, now); deadErr != nil {
				return nil, deadErr
			}
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		expires := now.Add(leaseDuration)
		result := s.db.WithContext(ctx).Model(&model.SyncJob{}).
			Where("id = ? AND attempts < max_attempts AND available_at <= ? AND ((status IN ?) OR (status = ? AND lease_expires_at <= ?))", candidate.ID, now, []model.SyncJobStatus{model.SyncJobPending, model.SyncJobRetryWait}, model.SyncJobLeased, now).
			Updates(map[string]any{
				"status": model.SyncJobLeased, "lease_owner": owner, "lease_expires_at": expires,
				"attempts": gorm.Expr("attempts + 1"), "finished_at": nil,
			})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		// Avoid a second SELECT here: on SQLite it can lose to concurrent claim
		// readers after this worker already won the atomic conditional UPDATE.
		candidate.Status = model.SyncJobLeased
		candidate.LeaseOwner = owner
		candidate.LeaseExpiresAt = &expires
		candidate.Attempts++
		candidate.FinishedAt = nil
		if runnableErr := s.ValidateRunnable(candidate.ConnectionID, candidate.JobType); runnableErr != nil {
			_ = s.cancelLeased(ctx, candidate.ID, owner, runnableErr)
			continue
		}
		return &candidate, nil
	}
	return nil, nil
}

func (s *SyncJobService) ValidateRunnable(connectionID uint, jobType model.SyncJobType) error {
	var connection model.IdentityConnection
	if err := s.db.First(&connection, connectionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: 同步连接不存在", ErrSyncNotRunnable)
		}
		return err
	}
	if !connection.IsEnabled() || !connection.HasCapability(model.ConnectionCapabilityDirectorySync) {
		return fmt.Errorf("%w: 连接已禁用或未启用 directory_sync 能力", ErrSyncNotRunnable)
	}
	if jobType == model.SyncJobEventRefresh && !connection.HasCapability(model.ConnectionCapabilityEvents) {
		return fmt.Errorf("%w: 连接未启用 events 能力", ErrSyncNotRunnable)
	}
	var policy model.DirectorySyncPolicy
	if err := s.db.Where("connection_id = ? AND enabled = ?", connectionID, true).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: 目录同步策略未启用", ErrSyncNotRunnable)
		}
		return err
	}
	return nil
}

func (s *SyncJobService) cancelLeased(ctx context.Context, jobID uint, owner string, cause error) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&model.SyncJob{}).
		Where("id = ? AND status = ? AND lease_owner = ?", jobID, model.SyncJobLeased, owner).
		Updates(map[string]any{"status": model.SyncJobCancelled, "finished_at": now, "lease_owner": "", "lease_expires_at": nil, "last_error": cause.Error()}).Error
}

func (s *SyncJobService) deadLetterExhaustedLeases(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Model(&model.SyncJob{}).
		Where("status = ? AND lease_expires_at <= ? AND attempts >= max_attempts", model.SyncJobLeased, now).
		Updates(map[string]any{
			"status": model.SyncJobDeadLetter, "finished_at": now,
			"lease_owner": "", "lease_expires_at": nil,
			"last_error": "任务租约过期且已达到最大尝试次数",
		}).Error
}

func (s *SyncJobService) RenewLease(ctx context.Context, jobID uint, owner string, leaseDuration time.Duration) (bool, error) {
	result := s.db.WithContext(ctx).Model(&model.SyncJob{}).
		Where("id = ? AND status = ? AND lease_owner = ?", jobID, model.SyncJobLeased, owner).
		Update("lease_expires_at", time.Now().Add(leaseDuration))
	return result.RowsAffected == 1, result.Error
}

func (s *SyncJobService) ValidateLease(ctx context.Context, jobID uint, owner string) error {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.SyncJob{}).
		Where("id = ? AND status = ? AND lease_owner = ? AND lease_expires_at > ?", jobID, model.SyncJobLeased, owner, time.Now()).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("任务 %d 租约已丢失", jobID)
	}
	return nil
}

func (s *SyncJobService) Complete(ctx context.Context, jobID uint, owner string, runID *uint) error {
	now := time.Now()
	updates := map[string]any{
		"status": model.SyncJobSucceeded, "finished_at": now, "lease_owner": "",
		"lease_expires_at": nil, "last_error": "",
	}
	if runID != nil {
		updates["run_id"] = *runID
	}
	result := s.db.WithContext(ctx).Model(&model.SyncJob{}).
		Where("id = ? AND status = ? AND lease_owner = ?", jobID, model.SyncJobLeased, owner).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("任务 %d 租约已丢失，不能标记完成", jobID)
	}
	return nil
}

func (s *SyncJobService) Fail(ctx context.Context, job *model.SyncJob, owner string, cause error) error {
	if job == nil {
		return fmt.Errorf("任务不能为空")
	}
	status := model.SyncJobRetryWait
	available := time.Now().Add(SyncJobBackoff(job.Attempts))
	var finishedAt any
	if job.Attempts >= job.MaxAttempts {
		status = model.SyncJobDeadLetter
		finishedAt = time.Now()
	}
	result := s.db.WithContext(ctx).Model(&model.SyncJob{}).
		Where("id = ? AND status = ? AND lease_owner = ?", job.ID, model.SyncJobLeased, owner).
		Updates(map[string]any{
			"status": status, "available_at": available, "lease_owner": "", "lease_expires_at": nil,
			"last_error": cause.Error(), "finished_at": finishedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("任务 %d 租约已丢失，不能记录失败", job.ID)
	}
	return nil
}

func (s *SyncJobService) FailPermanent(ctx context.Context, job *model.SyncJob, owner string, cause error) error {
	if job == nil {
		return fmt.Errorf("任务不能为空")
	}
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&model.SyncJob{}).
		Where("id = ? AND status = ? AND lease_owner = ?", job.ID, model.SyncJobLeased, owner).
		Updates(map[string]any{"status": model.SyncJobDeadLetter, "finished_at": now, "lease_owner": "", "lease_expires_at": nil, "last_error": "需要管理员处理: " + cause.Error()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("任务 %d 租约已丢失，不能记录永久失败", job.ID)
	}
	return nil
}

func SyncJobBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := 30 * time.Second * time.Duration(math.Pow(2, float64(attempt-1)))
	if base > MaxSyncJobBackoff {
		base = MaxSyncJobBackoff
	}
	// 0.8–1.2 jitter prevents every tenant retrying in lockstep.
	factor := 0.8 + rand.Float64()*0.4
	delay := time.Duration(float64(base) * factor)
	if delay > MaxSyncJobBackoff {
		return MaxSyncJobBackoff
	}
	return delay
}

func (s *SyncJobService) Retry(jobID uint) error {
	result := s.db.Model(&model.SyncJob{}).
		Where("id = ? AND status IN ?", jobID, []model.SyncJobStatus{model.SyncJobDeadLetter, model.SyncJobRetryWait, model.SyncJobCancelled}).
		Updates(map[string]any{"status": model.SyncJobPending, "available_at": time.Now(), "attempts": 0, "lease_owner": "", "lease_expires_at": nil, "finished_at": nil, "last_error": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("任务不存在或当前状态不能重试")
	}
	return nil
}

func (s *SyncJobService) Replay(jobID uint) error {
	result := s.db.Model(&model.SyncJob{}).
		Where("id = ? AND status <> ?", jobID, model.SyncJobLeased).
		Updates(map[string]any{"status": model.SyncJobPending, "available_at": time.Now(), "attempts": 0, "lease_owner": "", "lease_expires_at": nil, "finished_at": nil, "last_error": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("任务不存在或正在执行，不能重放")
	}
	return nil
}

func (s *SyncJobService) Cancel(jobID uint) error {
	now := time.Now()
	result := s.db.Model(&model.SyncJob{}).
		Where("id = ? AND status IN ?", jobID, []model.SyncJobStatus{model.SyncJobPending, model.SyncJobRetryWait}).
		Updates(map[string]any{"status": model.SyncJobCancelled, "finished_at": now, "lease_owner": "", "lease_expires_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("任务不存在或已在执行/结束")
	}
	return nil
}

func (s *SyncJobService) CancelConnectionPending(connectionID uint, reason string) error {
	if !s.db.Migrator().HasTable(&model.SyncJob{}) {
		return nil
	}
	now := time.Now()
	return s.db.Model(&model.SyncJob{}).
		Where("connection_id = ? AND status IN ?", connectionID, []model.SyncJobStatus{model.SyncJobPending, model.SyncJobRetryWait}).
		Updates(map[string]any{"status": model.SyncJobCancelled, "finished_at": now, "lease_owner": "", "lease_expires_at": nil, "last_error": reason}).Error
}

func (s *SyncJobService) List(filter SyncJobListFilter) ([]model.SyncJob, int64, error) {
	query := s.db.Model(&model.SyncJob{}).Preload("Connection").Preload("Run")
	if filter.ConnectionID != 0 {
		query = query.Where("connection_id = ?", filter.ConnectionID)
	}
	if filter.JobType != "" {
		query = query.Where("job_type = ?", filter.JobType)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 200 {
		filter.PageSize = 20
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var jobs []model.SyncJob
	err := query.Order("id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&jobs).Error
	return jobs, total, err
}

func validSyncJobType(jobType model.SyncJobType) bool {
	switch jobType {
	case model.SyncJobFullSync, model.SyncJobIncrementalSync, model.SyncJobReconcile, model.SyncJobEventRefresh, model.SyncJobApplyChanges:
		return true
	default:
		return false
	}
}
