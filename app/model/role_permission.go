package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

type JSONObject map[string]any

func (j *JSONObject) Scan(value interface{}) error {
	if value == nil {
		*j = JSONObject{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan JSONB")
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONObject) Value() (driver.Value, error) {
	if j == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(j)
}

// RolePermission represents the many-to-many relationship between roles and permissions.
// 角色权限关联表，用于实现RBAC（基于角色的访问控制）
type RolePermission struct {
	// 关联字段
	RoleID       uint `gorm:"primaryKey;index" json:"roleId"`
	PermissionID uint `gorm:"primaryKey;index" json:"permissionId"`

	// 授权信息
	GrantedBy uint       `gorm:"not null" json:"grantedBy"` // 授权人ID
	GrantedAt time.Time  `gorm:"not null" json:"grantedAt"` // 授权时间
	ExpiresAt *time.Time `json:"expiresAt"`                 // 过期时间，空表示永不过期

	// 约束条件
	Conditions JSONObject `gorm:"type:json" json:"conditions,omitempty"` // 权限约束条件
	DataScope  int        `gorm:"default:1" json:"dataScope"`            // 数据权限范围
	IsActive   bool       `gorm:"default:true;not null" json:"isActive"` // 是否激活

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联关系
	Role       Role       `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	Permission Permission `gorm:"foreignKey:PermissionID" json:"permission,omitempty"`
	GrantUser  User       `gorm:"foreignKey:GrantedBy" json:"grantUser,omitempty"`
}

// TableName returns the table name for RolePermission model.
func (*RolePermission) TableName() string {
	return "role_permissions"
}

// BeforeCreate hook to set default values before creating
func (rp *RolePermission) BeforeCreate(tx *gorm.DB) error {
	if rp.GrantedAt.IsZero() {
		rp.GrantedAt = time.Now()
	}
	if rp.DataScope == 0 {
		rp.DataScope = 1
	}
	return nil
}

// IsExpired checks if the role permission is expired
func (rp *RolePermission) IsExpired() bool {
	if rp.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*rp.ExpiresAt)
}

// IsValid checks if the role permission is valid (active and not expired)
func (rp *RolePermission) IsValid() bool {
	return rp.IsActive && !rp.IsExpired()
}

// Activate activates the role permission
func (rp *RolePermission) Activate() {
	rp.IsActive = true
}

// Deactivate deactivates the role permission
func (rp *RolePermission) Deactivate() {
	rp.IsActive = false
}

// SetExpiry sets the expiry time for the role permission
func (rp *RolePermission) SetExpiry(expiresAt time.Time) {
	rp.ExpiresAt = &expiresAt
}

// RemoveExpiry removes the expiry time (makes it permanent)
func (rp *RolePermission) RemoveExpiry() {
	rp.ExpiresAt = nil
}

// MatchCondition checks if the given conditions match the permission constraints
func (rp *RolePermission) MatchCondition(conditions map[string]interface{}) bool {
	if len(rp.Conditions) == 0 {
		return true // No constraints means always match
	}

	for key, value := range rp.Conditions {
		if condValue, exists := conditions[key]; !exists || condValue != value {
			return false
		}
	}
	return true
}

// SetCondition sets a condition for the role permission
func (rp *RolePermission) SetCondition(key string, value interface{}) {
	if rp.Conditions == nil {
		rp.Conditions = make(map[string]interface{})
	}
	rp.Conditions[key] = value
}

// RemoveCondition removes a condition from the role permission
func (rp *RolePermission) RemoveCondition(key string) {
	if rp.Conditions != nil {
		delete(rp.Conditions, key)
	}
}
