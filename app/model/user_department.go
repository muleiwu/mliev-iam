package model

import (
	"time"

	"gorm.io/gorm"
)

// UserDepartmentType 用户在部门中的类型
type UserDepartmentType int

const (
	UserDepartmentTypeMember UserDepartmentType = iota // 普通成员
	UserDepartmentTypeLeader                           // 负责人
	UserDepartmentTypeDeputy                           // 副负责人
)

// String returns the string representation of UserDepartmentType
func (t UserDepartmentType) String() string {
	switch t {
	case UserDepartmentTypeMember:
		return "member"
	case UserDepartmentTypeLeader:
		return "leader"
	case UserDepartmentTypeDeputy:
		return "deputy"
	default:
		return "unknown"
	}
}

// UserDepartment represents the relationship between users and departments.
// 用户部门关联表，用于管理用户的部门归属
// 支持 To C（用户无部门）和 To B（用户有部门）场景
type UserDepartment struct {
	// 关联字段
	UserID       uint `gorm:"primaryKey;index" json:"userId"`
	DepartmentID uint `gorm:"primaryKey;index" json:"departmentId"`

	// 职位信息
	Position string             `gorm:"size:100" json:"position"` // 职位
	Title    string             `gorm:"size:100" json:"title"`    // 职称
	Type     UserDepartmentType `gorm:"type:tinyint;default:0;not null" json:"type"`

	// 状态信息
	IsActive  bool `gorm:"default:true;not null" json:"isActive"` // 是否激活
	IsPrimary bool `gorm:"default:false" json:"isPrimary"`        // 是否主部门
	Sort      int  `gorm:"default:0" json:"sort"`                 // 排序

	// 时间信息
	JoinedAt  *time.Time     `json:"joinedAt"` // 加入时间
	LeftAt    *time.Time     `json:"leftAt"`   // 离开时间
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 扩展属性
	Attributes JSONObject `gorm:"type:json" json:"attributes,omitempty"`
	Remark     string     `gorm:"type:text" json:"remark"`

	// 关联关系
	User       User       `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"user,omitempty"`
	Department Department `gorm:"foreignKey:DepartmentID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"department,omitempty"`
}

// TableName returns the table name for UserDepartment model.
func (*UserDepartment) TableName() string {
	return "user_departments"
}

// BeforeCreate hook to set default values before creating
func (ud *UserDepartment) BeforeCreate(tx *gorm.DB) error {
	if ud.JoinedAt == nil {
		now := time.Now()
		ud.JoinedAt = &now
	}
	return nil
}

// IsLeader checks if user is a leader in the department
func (ud *UserDepartment) IsLeader() bool {
	return ud.Type == UserDepartmentTypeLeader
}

// IsDeputy checks if user is a deputy leader in the department
func (ud *UserDepartment) IsDeputy() bool {
	return ud.Type == UserDepartmentTypeDeputy
}

// IsMember checks if user is a regular member in the department
func (ud *UserDepartment) IsMember() bool {
	return ud.Type == UserDepartmentTypeMember
}

// IsValid checks if the user department relationship is valid
func (ud *UserDepartment) IsValid() bool {
	return ud.IsActive && ud.LeftAt == nil
}

// Activate activates the user department relationship
func (ud *UserDepartment) Activate() {
	ud.IsActive = true
}

// Deactivate deactivates the user department relationship
func (ud *UserDepartment) Deactivate() {
	ud.IsActive = false
}

// SetAsPrimary sets this as the primary department for the user
func (ud *UserDepartment) SetAsPrimary() {
	ud.IsPrimary = true
}

// UnsetAsPrimary removes the primary department flag
func (ud *UserDepartment) UnsetAsPrimary() {
	ud.IsPrimary = false
}

// SetLeader sets the user as a leader in the department
func (ud *UserDepartment) SetLeader() {
	ud.Type = UserDepartmentTypeLeader
}

// SetDeputy sets the user as a deputy leader in the department
func (ud *UserDepartment) SetDeputy() {
	ud.Type = UserDepartmentTypeDeputy
}

// SetMember sets the user as a regular member in the department
func (ud *UserDepartment) SetMember() {
	ud.Type = UserDepartmentTypeMember
}

// Leave marks the user as having left the department
func (ud *UserDepartment) Leave() {
	now := time.Now()
	ud.LeftAt = &now
	ud.IsActive = false
}
