package model

import (
	"time"

	"gorm.io/gorm"
)

// MigrationSourceType 迁移源类型。
type MigrationSourceType string

const (
	// MigrationSourceTypeAPI 渐进式过渡：登录时调用旧系统 API 校验。
	MigrationSourceTypeAPI MigrationSourceType = "api"
	// MigrationSourceTypeDatabase 一次性导入：从旧库批量导入到 legacy_users。
	MigrationSourceTypeDatabase MigrationSourceType = "database"
)

// LegacyUserStatus 旧用户迁移状态。
type LegacyUserStatus string

const (
	LegacyUserStatusPending  LegacyUserStatus = "pending"  // 已导入、尚未迁移
	LegacyUserStatusMigrated LegacyUserStatus = "migrated" // 已迁移进主 users 表
	LegacyUserStatusFailed   LegacyUserStatus = "failed"   // 迁移失败
)

// MigrationTaskStatus 导入任务状态。
type MigrationTaskStatus string

const (
	MigrationTaskStatusPending   MigrationTaskStatus = "pending"
	MigrationTaskStatusRunning   MigrationTaskStatus = "running"
	MigrationTaskStatusSucceeded MigrationTaskStatus = "succeeded"
	MigrationTaskStatusFailed    MigrationTaskStatus = "failed"
	MigrationTaskStatusCanceled  MigrationTaskStatus = "canceled"
)

// MigrationSource 迁移源配置：描述如何对接一个旧身份系统。
//
//   - type=api：仅需 LoginScript（登录时调用旧系统 API）。
//   - type=database：ConnectionConfig 存旧库连接信息，TransformScript 把旧行
//     映射为 legacy_users 记录，LoginScript 校验旧密码格式。
type MigrationSource struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	UUID string `gorm:"uniqueIndex:uk_migration_sources_uuid,priority:1;size:36;not null" json:"uuid"`
	Name string `gorm:"size:100;not null" json:"name"`

	Type    MigrationSourceType `gorm:"size:20;not null" json:"type"`
	Enabled bool                `gorm:"default:true;not null" json:"enabled"`
	Sort    int                 `gorm:"default:0" json:"sort"`

	// ConnectionConfig 旧库/旧 API 连接配置。database 类型示例：
	// {"driver":"mysql","dsn":"user:pass@tcp(host:3306)/db","query":"SELECT ...","batchSize":500}
	ConnectionConfig JSONObject `gorm:"type:json" json:"connectionConfig,omitempty"`

	LoginScript     string `gorm:"type:text" json:"loginScript"`
	TransformScript string `gorm:"type:text" json:"transformScript"`

	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"uniqueIndex:uk_migration_sources_uuid,priority:2;index" json:"-"`

	Remark string `gorm:"type:text" json:"remark"`
}

func (*MigrationSource) TableName() string {
	return "migration_sources"
}

// LegacyUser 一次性导入的旧用户（单独的旧表，与主 users 表隔离）。
type LegacyUser struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	SourceID uint   `gorm:"not null;uniqueIndex:uk_legacy_users_source_username,priority:1;index" json:"sourceId"`
	Username string `gorm:"size:100;not null;uniqueIndex:uk_legacy_users_source_username,priority:2;index" json:"username"`

	ExternalID string  `gorm:"size:255;index" json:"externalId"`
	Email      *string `gorm:"size:100;index" json:"email"`
	Phone      *string `gorm:"size:20;index" json:"phone"`
	Nickname   string  `gorm:"size:100" json:"nickname"`

	// 旧系统原始密码凭据，仅用于登录时由脚本校验，迁移后不进主表。
	PasswordHash string `gorm:"size:255" json:"-"`
	Salt         string `gorm:"size:64" json:"-"`

	// RawData 旧行的完整原始数据，供脚本灵活取用。
	RawData JSONObject `gorm:"type:json" json:"rawData,omitempty"`

	Status          LegacyUserStatus `gorm:"size:20;default:'pending';not null;index" json:"status"`
	MigratedUserID  *uint            `gorm:"index" json:"migratedUserId,omitempty"`
	MigratedAt      *time.Time       `json:"migratedAt,omitempty"`
	MigrationRemark string           `gorm:"type:text" json:"migrationRemark"`

	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"uniqueIndex:uk_legacy_users_source_username,priority:3;index" json:"-"`
}

func (*LegacyUser) TableName() string {
	return "legacy_users"
}

// MigrationTask 批量导入异步任务，记录进度供前端轮询。
type MigrationTask struct {
	ID       uint                `gorm:"primaryKey" json:"id"`
	SourceID uint                `gorm:"not null;index" json:"sourceId"`
	Status   MigrationTaskStatus `gorm:"size:20;default:'pending';not null;index" json:"status"`

	Total     int64 `gorm:"default:0" json:"total"`
	Processed int64 `gorm:"default:0" json:"processed"`
	Succeeded int64 `gorm:"default:0" json:"succeeded"`
	Failed    int64 `gorm:"default:0" json:"failed"`

	Error      string     `gorm:"type:text" json:"error"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`

	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (*MigrationTask) TableName() string {
	return "migration_tasks"
}
