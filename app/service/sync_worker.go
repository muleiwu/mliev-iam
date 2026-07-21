package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"cnb.cool/mliev/open/go-web/pkg/interfaces"
	"gorm.io/gorm"
)

type SyncWorker struct {
	db            *gorm.DB
	owner         string
	leaseDuration time.Duration
	pollInterval  time.Duration
}

func NewSyncWorker(db *gorm.DB, owner string) *SyncWorker {
	return &SyncWorker{db: db, owner: owner, leaseDuration: 45 * time.Second, pollInterval: 2 * time.Second}
}

func (w *SyncWorker) ProcessOne(ctx context.Context) (bool, error) {
	return w.processOne(ctx, ctx)
}

func (w *SyncWorker) processOne(claimCtx, workCtx context.Context) (bool, error) {
	jobService := NewSyncJobService(w.db)
	job, err := jobService.Claim(claimCtx, w.owner, w.leaseDuration, time.Now())
	if err != nil || job == nil {
		return false, err
	}
	jobCtx, cancel := context.WithCancel(workCtx)
	defer cancel()
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(w.leaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				ok, renewErr := jobService.RenewLease(jobCtx, job.ID, w.owner, w.leaseDuration)
				if renewErr != nil || !ok {
					cancel()
					return
				}
			}
		}
	}()
	runID, executeErr := w.execute(jobCtx, job)
	cancel()
	<-renewDone
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finalCancel()
	if executeErr == nil {
		if err := jobService.Complete(finalCtx, job.ID, w.owner, runID); err != nil {
			return true, err
		}
		if job.InboxEventID != nil {
			_ = NewExternalEventService(w.db).MarkProcessed(*job.InboxEventID)
		}
		return true, nil
	}
	if errors.Is(executeErr, ErrSyncNotRunnable) {
		if err := jobService.cancelLeased(finalCtx, job.ID, w.owner, executeErr); err != nil {
			return true, err
		}
		return true, executeErr
	}
	fail := jobService.Fail
	if isPermanentSyncError(executeErr) {
		fail = jobService.FailPermanent
	}
	if err := fail(finalCtx, job, w.owner, executeErr); err != nil {
		return true, err
	}
	if job.InboxEventID != nil {
		var updated model.SyncJob
		_ = w.db.First(&updated, job.ID).Error
		_ = NewExternalEventService(w.db).MarkFailed(*job.InboxEventID, updated.Status == model.SyncJobDeadLetter, executeErr)
	}
	return true, executeErr
}

func (w *SyncWorker) execute(ctx context.Context, job *model.SyncJob) (*uint, error) {
	jobService := NewSyncJobService(w.db)
	if err := jobService.ValidateRunnable(job.ConnectionID, job.JobType); err != nil {
		return nil, err
	}
	guard := func(guardCtx context.Context) error {
		if err := jobService.ValidateLease(guardCtx, job.ID, w.owner); err != nil {
			return err
		}
		return jobService.ValidateRunnable(job.ConnectionID, job.JobType)
	}
	syncService := NewDirectorySyncService(w.db)
	switch job.JobType {
	case model.SyncJobFullSync, model.SyncJobIncrementalSync, model.SyncJobReconcile, model.SyncJobEventRefresh:
		run, err := syncService.CreateRun(ctx, job.ConnectionID, nil, string(job.JobType))
		if err != nil {
			return nil, err
		}
		if run.TriggerType != string(job.JobType) {
			// Another trigger already owns the single approvable snapshot. This
			// task is coalesced into it and must not auto-approve a manual run.
			return &run.ID, nil
		}
		if err := syncService.ApproveAutomationSafeChanges(run.ID); err != nil {
			return &run.ID, err
		}
		if _, err := syncService.ApplyRunWithGuard(ctx, run.ID, 0, guard); err != nil {
			return &run.ID, err
		}
		return &run.ID, nil
	case model.SyncJobApplyChanges:
		runID := uintValue(job.Payload["runId"])
		if runID == 0 {
			return nil, fmt.Errorf("apply_changes 任务缺少 runId")
		}
		if _, err := syncService.ApplyRunWithGuard(ctx, runID, 0, guard); err != nil {
			return &runID, err
		}
		return &runID, nil
	default:
		return nil, fmt.Errorf("未知任务类型 %q", job.JobType)
	}
}

func isPermanentSyncError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"credential", "permission", "unauthorized", "forbidden", "invalid secret", "invalid token", "凭据", "权限", "未授权", "密钥错误"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (w *SyncWorker) Run(ctx context.Context) {
	w.RunWithContexts(ctx, ctx)
}

func (w *SyncWorker) RunWithContexts(claimCtx, workCtx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		processed, err := w.processOne(claimCtx, workCtx)
		if err != nil && !errors.Is(err, context.Canceled) {
			helper.GetLogger().Error(fmt.Sprintf("[sync worker] owner=%s: %v", w.owner, err))
		}
		if processed {
			continue
		}
		select {
		case <-claimCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

type SyncWorkerServer struct {
	schedulerCancel context.CancelFunc
	claimCancel     context.CancelFunc
	workCancel      context.CancelFunc
	schedulerWG     sync.WaitGroup
	workerWG        sync.WaitGroup
}

var _ interfaces.ServerInterface = (*SyncWorkerServer)(nil)

func (s *SyncWorkerServer) Run() error {
	if value := os.Getenv("SYNC_WORKER_ENABLED"); value == "0" || value == "false" {
		return nil
	}
	db := helper.GetDatabase()
	if db == nil {
		return fmt.Errorf("[sync worker] 数据库未初始化")
	}
	workerCount := 1
	if value, err := strconv.Atoi(os.Getenv("SYNC_WORKER_COUNT")); err == nil && value > 0 {
		workerCount = value
	}
	driver := helper.GetConfig().GetString("database.driver", "mysql")
	if driver == "sqlite" || driver == "sqlite3" {
		workerCount = 1
	}
	hostname, _ := os.Hostname()
	claimCtx, claimCancel := context.WithCancel(context.Background())
	workCtx, workCancel := context.WithCancel(context.Background())
	schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
	s.claimCancel, s.workCancel, s.schedulerCancel = claimCancel, workCancel, schedulerCancel
	for index := 0; index < workerCount; index++ {
		worker := NewSyncWorker(db, fmt.Sprintf("%s:%d:%d", hostname, os.Getpid(), index))
		s.workerWG.Add(1)
		go func() {
			defer s.workerWG.Done()
			worker.RunWithContexts(claimCtx, workCtx)
		}()
	}
	s.schedulerWG.Add(1)
	go func() {
		defer s.schedulerWG.Done()
		scheduleService := NewSyncScheduleService(db)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			_ = scheduleService.EnsureDailyReconciliation(time.Now())
			_, _ = scheduleService.EnqueueDue(time.Now())
			select {
			case <-schedulerCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (s *SyncWorkerServer) Stop() error {
	// Stop producers first, then stop new claims while allowing already leased
	// work up to 30 seconds to persist a terminal state.
	if s.schedulerCancel != nil {
		s.schedulerCancel()
		s.schedulerWG.Wait()
	}
	if s.claimCancel != nil {
		s.claimCancel()
	}
	drained := make(chan struct{})
	go func() {
		s.workerWG.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		return nil
	case <-time.After(30 * time.Second):
		if s.workCancel != nil {
			s.workCancel()
		}
		<-drained
	}
	return nil
}
