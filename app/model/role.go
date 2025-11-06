package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RoleType 角色类型枚举
type RoleType int

const (
	RoleTypeSystem RoleType = iota // 系统角色
	RoleTypeCustom                 // 自定义角色
)

// String returns the string representation of RoleType
func (rt RoleType) String() string {
	switch rt {
	case RoleTypeSystem:
		return "system"
	case RoleTypeCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// RoleStatus 角色状态枚举
type RoleStatus int

const (
	RoleStatusDisabled RoleStatus = iota // 禁用
	RoleStatusEnabled                    // 启用
)

// String returns the string representation of RoleStatus
func (rs RoleStatus) String() string {
	switch rs {
	case RoleStatusDisabled:
		return "disabled"
	case RoleStatusEnabled:
		return "enabled"
	default:
		return "unknown"
	}
}

// Role represents a role entity in the database.
// 角色模型用于RBAC权限控制，支持角色定义与管理
type Role struct {
	// 基础信息
	ID          uint        `gorm:"primaryKey" json:"id"`
	Name        string      `gorm:"uniqueIndex;size:50;not null" json:"name" validate:"required,min=2,max=50"`
	Code        string      `gorm:"uniqueIndex;size:50;not null" json:"code" validate:"required,min=2,max=50"`
	Description string      `gorm:"size:255" json:"description" validate:"max=255"`
	Type        *RoleType   `gorm:"type:tinyint;default:1;not null" json:"type"`
	Status      *RoleStatus `gorm:"type:tinyint;default:1;not null" json:"status"`

	// 层级关系
	ParentID *uint  `gorm:"index" json:"parentId"`
	Level    *int   `gorm:"default:1;not null" json:"level"`
	Path     string `gorm:"size:500" json:"path"` // 层级路径，如: /1/2/3/
	Sort     int    `gorm:"default:0" json:"sort"`

	// 权限范围
	DataScope *int   `gorm:"default:1;not null" json:"dataScope"` // 数据权限范围：1全部 2本部门 3本部门及下级 4仅本人
	ScopeDesc string `gorm:"size:255" json:"scopeDesc"`           // 数据权限范围描述

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 扩展属性
	Attributes datatypes.JSON `gorm:"type:json" json:"attributes,omitempty"`
	Remark     string         `gorm:"type:text" json:"remark"`

	// 关联关系
	Children    []Role       `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	Parent      *Role        `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
	Users       []User       `gorm:"many2many:user_roles;" json:"users,omitempty"`
}

// TableName returns the table name for Role model.
func (*Role) TableName() string {
	return "roles"
}

// BeforeCreate hook to set default values before creating
func (r *Role) BeforeCreate(tx *gorm.DB) error {
	if r.Status == nil {
		enabled := RoleStatusEnabled
		r.Status = &enabled
	}
	if r.Type == nil {
		custom := RoleTypeCustom
		r.Type = &custom
	}
	if r.Level == nil {
		level := 1
		r.Level = &level
	}
	if r.DataScope == nil {
		scope := 1
		r.DataScope = &scope
	}
	return nil
}

// AfterCreate hook to update path after creating
func (r *Role) AfterCreate(tx *gorm.DB) error {
	if r.ParentID != nil {
		// 查询父角色路径
		var parent Role
		if err := tx.First(&parent, *r.ParentID).Error; err != nil {
			return err
		}
		r.Path = parent.Path + "/" + string(rune(r.ID))
		if parent.Level != nil {
			level := *parent.Level + 1
			r.Level = &level
		}
	} else {
		r.Path = "/" + string(rune(r.ID))
		level := 1
		r.Level = &level
	}

	return tx.Model(r).Updates(map[string]interface{}{
		"path":  r.Path,
		"level": r.Level,
	}).Error
}

// IsEnabled checks if role is enabled
func (r *Role) IsEnabled() bool {
	return r.Status != nil && *r.Status == RoleStatusEnabled
}

// IsSystem checks if role is system role
func (r *Role) IsSystem() bool {
	return r.Type != nil && *r.Type == RoleTypeSystem
}

// Enable enables the role
func (r *Role) Enable() {
	enabled := RoleStatusEnabled
	r.Status = &enabled
}

// Disable disables the role
func (r *Role) Disable() {
	disabled := RoleStatusDisabled
	r.Status = &disabled
}

// IsRoot checks if role is root level
func (r *Role) IsRoot() bool {
	return r.ParentID == nil
}

// GetAncestors returns all ancestor roles from path
func (r *Role) GetAncestors() []uint {
	// 解析路径获取祖先ID列表
	// 实现略，需要解析Path字段
	return []uint{}
}

// CanManageDataScope checks if role can manage data in specified scope
func (r *Role) CanManageDataScope(targetScope int) bool {
	return r.DataScope != nil && *r.DataScope <= targetScope
}
