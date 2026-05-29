package model

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"gorm.io/gorm"
)

// UserStatus 用户状态枚举
type UserStatus int

const (
	UserStatusActive   UserStatus = 1 // 激活
	UserStatusDisabled UserStatus = 2 // 禁用
)

// String returns the string representation of UserStatus
func (s UserStatus) String() string {
	switch s {
	case UserStatusActive:
		return "active"
	case UserStatusDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

// User represents a user entity in the database.
// 用户模型包含用户的基础信息、认证信息和状态管理
type User struct {
	// 基础信息
	// uniqueIndex 与 DeletedAt 组成复合唯一索引，允许同一 UUID/Username/... 在软删后重建。
	// 注意：MySQL 对 NULL 视为互不相等，所以"活跃（deleted_at IS NULL）状态下的重复值"
	// 仍需 DAO 层显式检查。现有 userDao 的 CheckUsernameExists 等方法保留此职责。
	ID       uint    `gorm:"primaryKey" json:"id"`
	UUID     string  `gorm:"uniqueIndex:uk_users_uuid,priority:1;size:36;not null" json:"uuid"`
	Username string  `gorm:"uniqueIndex:uk_users_username,priority:1;size:50;not null" json:"username" validate:"required,min=3,max=50"`
	Email    *string `gorm:"uniqueIndex:uk_users_email,priority:1;size:100" json:"email" validate:"email,max=100"`
	Phone    *string `gorm:"uniqueIndex:uk_users_phone,priority:1;size:20" json:"phone" validate:"max=20"`
	Nickname string  `gorm:"size:50" json:"nickname" validate:"max=50"`
	RealName string  `gorm:"size:50" json:"realName" validate:"max=50"`
	Avatar   string  `gorm:"size:255" json:"avatar" validate:"max=255"`

	Salt         string `gorm:"size:32" json:"-"`
	PasswordHash string `gorm:"size:100" json:"-"`

	// 状态管理
	Status          UserStatus `gorm:"default:0;not null" json:"status"`
	IsActivated     bool       `gorm:"default:false;not null" json:"isActivated"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt"`
	PhoneVerifiedAt *time.Time `json:"phoneVerifiedAt"`
	LoginCount      int64      `gorm:"default:0" json:"loginCount"`
	FailureCount    int64      `gorm:"default:0" json:"failureCount"`

	// 时间信息
	LastLoginAt    *time.Time     `json:"lastLoginAt"`
	LastLoginIP    string         `gorm:"size:45" json:"lastLoginIP"`
	PasswordExpiry *time.Time     `json:"passwordExpiry"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"uniqueIndex:uk_users_uuid,priority:2;uniqueIndex:uk_users_username,priority:2;uniqueIndex:uk_users_email,priority:2;uniqueIndex:uk_users_phone,priority:2;index" json:"-"`

	// 扩展属性
	Attributes JSONObject `gorm:"type:json" json:"attributes,omitempty"`
	Remark     string     `gorm:"type:text" json:"remark"`

	// 关联关系（通过独立关联表）
	// Departments  []Department     `gorm:"many2many:user_departments;" json:"departments,omitempty"`
	// Roles        []Role           `gorm:"many2many:user_roles;" json:"roles,omitempty"`
}

// TableName returns the table name for User model.
func (*User) TableName() string {
	return "users"
}

// BeforeCreate hook to set default values before creating
func (u *User) BeforeCreate(tx *gorm.DB) error {
	// Generate UUIDv7 if not already set
	if u.UUID == "" {
		uuid, err := utils.GenerateUUIDv7()
		if err != nil {
			return err
		}
		u.UUID = uuid
	}

	if u.Status == 0 {
		u.Status = UserStatusActive
	}
	return nil
}

// IsActive checks if user is active
func (u *User) IsActive() bool {
	return u.Status == UserStatusActive
}

// EmailVerified 是否已验证邮箱（派生自 EmailVerifiedAt 时间戳）
func (u *User) EmailVerified() bool {
	return u.EmailVerifiedAt != nil
}

// PhoneVerified 是否已验证手机（派生自 PhoneVerifiedAt 时间戳）
func (u *User) PhoneVerified() bool {
	return u.PhoneVerifiedAt != nil
}

// IsDisabled checks if user is disabled
func (u *User) IsDisabled() bool {
	return u.Status == UserStatusDisabled
}

// CanLogin checks if user can login
func (u *User) CanLogin() bool {
	return u.IsActive() && u.IsActivated && !u.IsDisabled()
}

// UpdateLastLogin updates last login information
func (u *User) UpdateLastLogin(ip string) {
	now := time.Now()
	u.LastLoginAt = &now
	u.LastLoginIP = ip
	u.LoginCount++
}

// Activate activates the user account
func (u *User) Activate() {
	u.IsActivated = true
	u.Status = UserStatusActive
}

// Deactivate deactivates the user account
func (u *User) Deactivate() {
	u.Status = UserStatusDisabled
}
