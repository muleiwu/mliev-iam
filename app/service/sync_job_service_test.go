package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSyncJobLeaseConcurrentClaimAndExpiredTakeover(t *testing.T) {
	db := openSyncAutomationTestDB(t)
	connection := seedAutomationConnection(t, db)
	job, err := NewSyncJobService(db).Enqueue(connection.ID, model.SyncJobFullSync, nil, time.Now(), nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	claimed := make(chan *model.SyncJob, 8)
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			item, claimErr := NewSyncJobService(db).Claim(context.Background(), owner, time.Second, time.Now())
			if claimErr == nil && item != nil {
				claimed <- item
			}
		}(fmt.Sprintf("worker-%d", index))
	}
	wg.Wait()
	close(claimed)
	items := make([]*model.SyncJob, 0)
	for item := range claimed {
		items = append(items, item)
	}
	require.Len(t, items, 1, "同一任务只能被一个 worker 抢占")
	require.Equal(t, job.ID, items[0].ID)

	takenOver, err := NewSyncJobService(db).Claim(context.Background(), "recovery-worker", time.Second, time.Now().Add(2*time.Second))
	require.NoError(t, err)
	require.NotNil(t, takenOver)
	require.Equal(t, "recovery-worker", takenOver.LeaseOwner)
	require.Equal(t, 2, takenOver.Attempts)
}

func TestSyncJobRetryDeadLetterAndReplay(t *testing.T) {
	db := openSyncAutomationTestDB(t)
	connection := seedAutomationConnection(t, db)
	job, err := NewSyncJobService(db).Enqueue(connection.ID, model.SyncJobReconcile, nil, time.Now(), nil)
	require.NoError(t, err)
	require.NoError(t, db.Model(job).Update("max_attempts", 1).Error)
	claimed, err := NewSyncJobService(db).Claim(context.Background(), "worker", time.Minute, time.Now())
	require.NoError(t, err)
	require.NoError(t, NewSyncJobService(db).Fail(context.Background(), claimed, "worker", fmt.Errorf("upstream unavailable")))
	require.NoError(t, db.First(job, job.ID).Error)
	require.Equal(t, model.SyncJobDeadLetter, job.Status)
	require.Contains(t, job.LastError, "upstream unavailable")
	require.NoError(t, NewSyncJobService(db).Replay(job.ID))
	require.NoError(t, db.First(job, job.ID).Error)
	require.Equal(t, model.SyncJobPending, job.Status)
	require.Zero(t, job.Attempts)
	require.LessOrEqual(t, SyncJobBackoff(30), MaxSyncJobBackoff)
}

func TestSyncScheduleEnqueuesDueExactlyOnce(t *testing.T) {
	db := openSyncAutomationTestDB(t)
	connection := seedAutomationConnection(t, db)
	root := model.Department{Name: "自动同步根", Code: "auto-root", Status: model.DepartmentStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&model.DirectorySyncPolicy{ConnectionID: connection.ID, RootDepartmentID: root.ID, MatchPolicy: model.DirectoryMatchManual, DeprovisionPolicy: model.DirectoryDeprovisionBlockIdentityRequireApproval, Enabled: true}).Error)
	schedules, err := NewSyncScheduleService(db).Replace(connection.ID, []SyncScheduleInput{{ScheduleType: model.SyncScheduleDailyReconcile, CronExpression: "0 2 * * *", Timezone: "UTC", Enabled: true}})
	require.NoError(t, err)
	require.Len(t, schedules, 1)
	dueAt := time.Now().Add(-time.Minute)
	require.NoError(t, db.Model(&model.SyncSchedule{}).Where("id = ?", schedules[0].ID).Update("next_run_at", dueAt).Error)
	enqueued, err := NewSyncScheduleService(db).EnqueueDue(time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, enqueued)
	enqueued, err = NewSyncScheduleService(db).EnqueueDue(time.Now())
	require.NoError(t, err)
	require.Zero(t, enqueued)
	var jobs int64
	require.NoError(t, db.Model(&model.SyncJob{}).Count(&jobs).Error)
	require.EqualValues(t, 1, jobs)
}

func openSyncAutomationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(12)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Department{}, &model.IdentityConnection{}, &model.DirectorySyncPolicy{},
		&model.DirectorySyncRun{}, &model.DirectorySyncChange{}, &model.ExternalEventInbox{},
		&model.SyncJob{}, &model.SyncSchedule{},
	))
	return db
}

func seedAutomationConnection(t *testing.T, db *gorm.DB) *model.IdentityConnection {
	t.Helper()
	connection := &model.IdentityConnection{Name: "自动同步", Identifier: "auto-" + t.Name(), Category: model.ConnectionCategoryEnterprise, ProviderType: "feishu", Capabilities: model.StringList{"directory_sync", "events"}, Status: model.ConnectionStatusEnabled}
	require.NoError(t, db.Create(connection).Error)
	return connection
}
