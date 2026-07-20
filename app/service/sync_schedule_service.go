package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SyncScheduleInput struct {
	ScheduleType   model.SyncScheduleType `json:"scheduleType"`
	CronExpression string                 `json:"cronExpression"`
	Timezone       string                 `json:"timezone"`
	Enabled        bool                   `json:"enabled"`
}

type SyncScheduleService struct{ db *gorm.DB }

func NewSyncScheduleService(db *gorm.DB) *SyncScheduleService {
	return &SyncScheduleService{db: db}
}

func (s *SyncScheduleService) List(connectionID uint) ([]model.SyncSchedule, error) {
	var schedules []model.SyncSchedule
	err := s.db.Where("connection_id = ?", connectionID).Order("schedule_type ASC").Find(&schedules).Error
	return schedules, err
}

func (s *SyncScheduleService) Replace(connectionID uint, inputs []SyncScheduleInput) ([]model.SyncSchedule, error) {
	var policy model.DirectorySyncPolicy
	if err := s.db.Where("connection_id = ? AND enabled = ?", connectionID, true).First(&policy).Error; err != nil {
		return nil, fmt.Errorf("目录同步策略未启用: %w", err)
	}
	seen := map[model.SyncScheduleType]bool{}
	now := time.Now()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, input := range inputs {
			if input.ScheduleType != model.SyncScheduleCron && input.ScheduleType != model.SyncScheduleDailyReconcile {
				return fmt.Errorf("非法计划类型 %q", input.ScheduleType)
			}
			if seen[input.ScheduleType] {
				return fmt.Errorf("计划类型 %q 重复", input.ScheduleType)
			}
			seen[input.ScheduleType] = true
			if input.Timezone == "" {
				input.Timezone = "UTC"
			}
			location, err := time.LoadLocation(input.Timezone)
			if err != nil {
				return fmt.Errorf("非法时区 %q", input.Timezone)
			}
			if input.ScheduleType == model.SyncScheduleDailyReconcile && input.CronExpression == "" {
				input.CronExpression = "0 2 * * *"
			}
			next, err := nextCronTime(input.CronExpression, now.In(location))
			if err != nil {
				return err
			}
			schedule := &model.SyncSchedule{
				ConnectionID: connectionID, ScheduleType: input.ScheduleType,
				CronExpression: input.CronExpression, Timezone: input.Timezone,
				Enabled: input.Enabled, NextRunAt: &next,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "connection_id"}, {Name: "schedule_type"}},
				DoUpdates: clause.Assignments(map[string]any{
					"cron_expression": input.CronExpression, "timezone": input.Timezone,
					"enabled": input.Enabled, "next_run_at": next, "updated_at": now,
				}),
			}).Create(schedule).Error; err != nil {
				return err
			}
		}
		if len(seen) == 0 {
			return tx.Where("connection_id = ?", connectionID).Delete(&model.SyncSchedule{}).Error
		}
		types := make([]model.SyncScheduleType, 0, len(seen))
		for scheduleType := range seen {
			types = append(types, scheduleType)
		}
		return tx.Where("connection_id = ? AND schedule_type NOT IN ?", connectionID, types).Delete(&model.SyncSchedule{}).Error
	}); err != nil {
		return nil, err
	}
	return s.List(connectionID)
}

func (s *SyncScheduleService) EnsureDailyReconciliation(now time.Time) error {
	var policies []model.DirectorySyncPolicy
	if err := s.db.Where("enabled = ?", true).Find(&policies).Error; err != nil {
		return err
	}
	for _, policy := range policies {
		location := time.UTC
		next, _ := nextCronTime("0 2 * * *", now.In(location))
		schedule := &model.SyncSchedule{
			ConnectionID: policy.ConnectionID, ScheduleType: model.SyncScheduleDailyReconcile,
			CronExpression: "0 2 * * *", Timezone: "UTC", Enabled: true, NextRunAt: &next,
		}
		if err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "connection_id"}, {Name: "schedule_type"}}, DoNothing: true}).Create(schedule).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *SyncScheduleService) EnqueueDue(now time.Time) (int, error) {
	var due []model.SyncSchedule
	if err := s.db.Where("enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", true, now).Order("next_run_at ASC, id ASC").Limit(200).Find(&due).Error; err != nil {
		return 0, err
	}
	enqueued := 0
	for _, candidate := range due {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			var schedule model.SyncSchedule
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&schedule, candidate.ID).Error; err != nil {
				return err
			}
			if !schedule.Enabled || schedule.NextRunAt == nil || schedule.NextRunAt.After(now) {
				return nil
			}
			location, err := time.LoadLocation(schedule.Timezone)
			if err != nil {
				return err
			}
			next, err := nextCronTime(schedule.CronExpression, now.In(location))
			if err != nil {
				return err
			}
			jobType := model.SyncJobFullSync
			if schedule.ScheduleType == model.SyncScheduleDailyReconcile {
				jobType = model.SyncJobReconcile
			}
			if _, err := NewSyncJobService(tx).Enqueue(schedule.ConnectionID, jobType, map[string]any{"scheduleId": schedule.ID}, now, nil); err != nil {
				return err
			}
			if err := tx.Model(&schedule).Updates(map[string]any{"last_enqueued_at": now, "next_run_at": next}).Error; err != nil {
				return err
			}
			enqueued++
			return nil
		})
		if err != nil {
			return enqueued, err
		}
	}
	return enqueued, nil
}

func nextCronTime(expression string, after time.Time) (time.Time, error) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron 必须使用 5 段格式（分 时 日 月 周）")
	}
	minute, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron 分钟字段错误: %w", err)
	}
	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron 小时字段错误: %w", err)
	}
	if fields[2] != "*" || fields[3] != "*" || fields[4] != "*" {
		return time.Time{}, fmt.Errorf("当前版本 cron 的日、月、周必须为 *")
	}
	candidate := after.Truncate(time.Minute).Add(time.Minute)
	limit := candidate.Add(370 * 24 * time.Hour)
	for candidate.Before(limit) {
		if minute[candidate.Minute()] && hour[candidate.Hour()] {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, errors.New("无法计算下一次执行时间")
}

func parseCronField(field string, minimum, maximum int) (map[int]bool, error) {
	result := map[int]bool{}
	if field == "*" {
		for value := minimum; value <= maximum; value++ {
			result[value] = true
		}
		return result, nil
	}
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(strings.TrimPrefix(field, "*/"))
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("步长非法")
		}
		for value := minimum; value <= maximum; value += step {
			result[value] = true
		}
		return result, nil
	}
	for _, part := range strings.Split(field, ",") {
		value, err := strconv.Atoi(part)
		if err != nil || value < minimum || value > maximum {
			return nil, fmt.Errorf("值 %q 越界", part)
		}
		result[value] = true
	}
	return result, nil
}
