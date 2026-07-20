package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DirectoryLease struct {
	db       *gorm.DB
	key      string
	owner    string
	duration time.Duration
}

func AcquireDirectoryLease(ctx context.Context, db *gorm.DB, key string, runID *uint, duration time.Duration) (*DirectoryLease, error) {
	if duration <= 0 {
		duration = 2 * time.Minute
	}
	owner := uuid.NewString()
	now := time.Now()
	expires := now.Add(duration)
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.DirectoryOperationLease
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("lease_key = ?", key).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&model.DirectoryOperationLease{LeaseKey: key, Owner: owner, RunID: runID, LeaseExpiresAt: expires}).Error
		}
		if err != nil {
			return err
		}
		if current.LeaseExpiresAt.After(now) {
			return fmt.Errorf("目录操作 %s 已由 %s 持有至 %s", key, current.Owner, current.LeaseExpiresAt.Format(time.RFC3339))
		}
		return tx.Model(&current).Updates(map[string]any{"owner": owner, "run_id": runID, "lease_expires_at": expires}).Error
	})
	if err != nil {
		return nil, err
	}
	return &DirectoryLease{db: db, key: key, owner: owner, duration: duration}, nil
}

func (l *DirectoryLease) Renew(ctx context.Context) error {
	if l == nil {
		return errors.New("lease 为空")
	}
	result := l.db.WithContext(ctx).Model(&model.DirectoryOperationLease{}).
		Where("lease_key = ? AND owner = ? AND lease_expires_at > ?", l.key, l.owner, time.Now()).
		Update("lease_expires_at", time.Now().Add(l.duration))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("目录操作租约已丢失")
	}
	return nil
}

func (l *DirectoryLease) Validate(ctx context.Context) error {
	if l == nil {
		return errors.New("lease 为空")
	}
	var count int64
	err := l.db.WithContext(ctx).Model(&model.DirectoryOperationLease{}).
		Where("lease_key = ? AND owner = ? AND lease_expires_at > ?", l.key, l.owner, time.Now()).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("目录操作租约已丢失")
	}
	return nil
}

func (l *DirectoryLease) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	return l.db.WithContext(ctx).Where("lease_key = ? AND owner = ?", l.key, l.owner).Delete(&model.DirectoryOperationLease{}).Error
}

func (l *DirectoryLease) StartRenewal(ctx context.Context) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(l.duration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				_ = l.Renew(context.Background())
			}
		}
	}()
	return func() { close(stop) }
}
