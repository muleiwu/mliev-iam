package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
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
	jobService := NewSyncJobService(w.db)
	job, err := jobService.Claim(ctx, w.owner, w.leaseDuration, time.Now())
	if err != nil || job == nil {
		return false, err
	}
	jobCtx, cancel := context.WithCancel(ctx)
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
	if executeErr == nil {
		if err := jobService.Complete(ctx, job.ID, w.owner, runID); err != nil {
			return true, err
		}
		if job.InboxEventID != nil {
			_ = NewExternalEventService(w.db).MarkProcessed(*job.InboxEventID)
		}
		return true, nil
	}
	if err := jobService.Fail(ctx, job, w.owner, executeErr); err != nil {
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
	syncService := NewDirectorySyncService(w.db)
	switch job.JobType {
	case model.SyncJobFullSync, model.SyncJobIncrementalSync, model.SyncJobReconcile, model.SyncJobEventRefresh:
		run, err := syncService.CreateRun(ctx, job.ConnectionID, nil, string(job.JobType))
		if err != nil {
			return nil, err
		}
		if err := syncService.ApproveAutomationSafeChanges(run.ID); err != nil {
			return &run.ID, err
		}
		if _, err := syncService.ApplyRun(run.ID, 0); err != nil {
			return &run.ID, err
		}
		return &run.ID, nil
	case model.SyncJobApplyChanges:
		runID := uintValue(job.Payload["runId"])
		if runID == 0 {
			return nil, fmt.Errorf("apply_changes 任务缺少 runId")
		}
		if _, err := syncService.ApplyRun(runID, 0); err != nil {
			return &runID, err
		}
		return &runID, nil
	default:
		return nil, fmt.Errorf("未知任务类型 %q", job.JobType)
	}
}

func (w *SyncWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		processed, err := w.ProcessOne(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			helper.GetLogger().Error(fmt.Sprintf("[sync worker] owner=%s: %v", w.owner, err))
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type SyncWorkerServer struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
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
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	for index := 0; index < workerCount; index++ {
		worker := NewSyncWorker(db, fmt.Sprintf("%s:%d:%d", hostname, os.Getpid(), index))
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			worker.Run(ctx)
		}()
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		scheduleService := NewSyncScheduleService(db)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			_ = scheduleService.EnsureDailyReconciliation(time.Now())
			_, _ = scheduleService.EnqueueDue(time.Now())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (s *SyncWorkerServer) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	return nil
}
