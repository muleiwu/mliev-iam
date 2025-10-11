package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PermissionType 权限类型枚举
type PermissionType int

const (
	PermissionTypeMenu   PermissionType = iota // 菜单权限
	PermissionTypeButton                       // 按钮权限
	PermissionTypeAPI                          // API权限
	PermissionTypeData                         // 数据权限
)

// String returns the string representation of PermissionType
func (pt PermissionType) String() string {
	switch pt {
	case PermissionTypeMenu:
		return "menu"
	case PermissionTypeButton:
		return "button"
	case PermissionTypeAPI:
		return "api"
	case PermissionTypeData:
		return "data"
	default:
		return "unknown"
	}
}

// PermissionStatus 权限状态枚举
type PermissionStatus int

const (
	PermissionStatusDisabled PermissionStatus = iota // 禁用
	PermissionStatusEnabled                          // 启用
)

// String returns the string representation of PermissionStatus
func (ps PermissionStatus) String() string {
	switch ps {
	case PermissionStatusDisabled:
		return "disabled"
	case PermissionStatusEnabled:
		return "enabled"
	default:
		return "unknown"
	}
}

// Permission represents a permission entity in the database.
// 权限模型用于定义系统中的权限资源，支持层级权限和多种权限类型
type Permission struct {
	// 基础信息
	ID          uint             `gorm:"primaryKey" json:"id"`
	Name        string           `gorm:"size:100;not null" json:"name" validate:"required,min=2,max=100"`
	Code        string           `gorm:"uniqueIndex;size:100;not null" json:"code" validate:"required,min=2,max=100"`
	Description string           `gorm:"size:255" json:"description" validate:"max=255"`
	Type        PermissionType   `gorm:"type:tinyint;default:0;not null" json:"type"`
	Status      PermissionStatus `gorm:"type:tinyint;default:1;not null" json:"status"`

	// 层级关系
	ParentID *uint  `gorm:"index" json:"parentId"`
	Level    int    `gorm:"default:1;not null" json:"level"`
	Path     string `gorm:"size:500" json:"path"` // 层级路径，如: /1/2/3/
	Sort     int    `gorm:"default:0" json:"sort"`

	// 权限资源信息
	Resource   string `gorm:"size:255" json:"resource"`       // 资源标识符，如：user:create, order:view
	Action     string `gorm:"size:100" json:"action"`         // 操作类型：create, read, update, delete
	Method     string `gorm:"size:20" json:"method"`          // HTTP方法：GET, POST, PUT, DELETE
	URL        string `gorm:"size:255" json:"url"`            // API路径
	Component  string `gorm:"size:255" json:"component"`      // 前端组件路径
	Icon       string `gorm:"size:100" json:"icon"`           // 图标
	Redirect   string `gorm:"size:255" json:"redirect"`       // 重定向路径
	External   bool   `gorm:"default:false" json:"external"`  // 是否外部链接
	Hidden     bool   `gorm:"default:false" json:"hidden"`    // 是否隐藏
	KeepAlive  bool   `gorm:"default:false" json:"keepAlive"` // 是否缓存
	ShowInMenu bool   `gorm:"default:true" json:"showInMenu"` // 是否在菜单中显示

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 扩展属性
	Attributes datatypes.JSON `gorm:"type:json" json:"attributes,omitempty"`
	Remark     string         `gorm:"type:text" json:"remark"`

	// 关联关系
	Children []Permission `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	Parent   *Permission  `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Roles    []Role       `gorm:"many2many:role_permissions;" json:"roles,omitempty"`
}

// TableName returns the table name for Permission model.
func (*Permission) TableName() string {
	return "permissions"
}

// BeforeCreate hook to set default values before creating
func (p *Permission) BeforeCreate(tx *gorm.DB) error {
	if p.Status == 0 {
		p.Status = PermissionStatusEnabled
	}
	if p.Level == 0 {
		p.Level = 1
	}
	return nil
}

// AfterCreate hook to update path after creating
func (p *Permission) AfterCreate(tx *gorm.DB) error {
	if p.ParentID != nil {
		// 查询父权限路径
		var parent Permission
		if err := tx.First(&parent, *p.ParentID).Error; err != nil {
			return err
		}
		p.Path = parent.Path + "/" + string(rune(p.ID))
		p.Level = parent.Level + 1
	} else {
		p.Path = "/" + string(rune(p.ID))
		p.Level = 1
	}

	return tx.Model(p).Updates(map[string]interface{}{
		"path":  p.Path,
		"level": p.Level,
	}).Error
}

// IsEnabled checks if permission is enabled
func (p *Permission) IsEnabled() bool {
	return p.Status == PermissionStatusEnabled
}

// IsMenu checks if permission is menu type
func (p *Permission) IsMenu() bool {
	return p.Type == PermissionTypeMenu
}

// IsButton checks if permission is button type
func (p *Permission) IsButton() bool {
	return p.Type == PermissionTypeButton
}

// IsAPI checks if permission is API type
func (p *Permission) IsAPI() bool {
	return p.Type == PermissionTypeAPI
}

// IsData checks if permission is data type
func (p *Permission) IsData() bool {
	return p.Type == PermissionTypeData
}

// Enable enables the permission
func (p *Permission) Enable() {
	p.Status = PermissionStatusEnabled
}

// Disable disables the permission
func (p *Permission) Disable() {
	p.Status = PermissionStatusDisabled
}

// IsRoot checks if permission is root level
func (p *Permission) IsRoot() bool {
	return p.ParentID == nil
}

// GetAncestors returns all ancestor permissions from path
func (p *Permission) GetAncestors() []uint {
	// 解析路径获取祖先ID列表
	// 实现略，需要解析Path字段
	return []uint{}
}

// MatchResource checks if permission matches the given resource and action
func (p *Permission) MatchResource(resource, action string) bool {
	return p.Resource == resource && (p.Action == action || p.Action == "*")
}

// MatchURL checks if permission matches the given URL and method
func (p *Permission) MatchURL(url, method string) bool {
	return p.URL == url && (p.Method == method || p.Method == "*")
}

// GenerateResourceCode generates resource code from resource and action
func (p *Permission) GenerateResourceCode() string {
	if p.Resource != "" && p.Action != "" {
		return p.Resource + ":" + p.Action
	}
	return p.Code
}
