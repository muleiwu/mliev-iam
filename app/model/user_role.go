package model

import (
	"time"

	"gorm.io/gorm"
)

// UserRole represents the many-to-many relationship between users and roles.
// 用户角色关联表，用于用户角色绑定管理
type UserRole struct {
	// 关联字段
	UserID uint `gorm:"primaryKey;index" json:"userId"`
	RoleID uint `gorm:"primaryKey;index" json:"roleId"`

	// 授权信息
	AssignedBy uint       `gorm:"not null" json:"assignedBy"` // 分配人ID
	AssignedAt time.Time  `gorm:"not null" json:"assignedAt"` // 分配时间
	ExpiresAt  *time.Time `json:"expiresAt"`                  // 过期时间，空表示永不过期

	// 角色约束
	Constraints JSONObject `gorm:"type:json" json:"constraints,omitempty"` // 角色约束条件
	DataScope   int        `gorm:"default:1" json:"dataScope"`             // 数据权限范围
	IsActive    bool       `gorm:"default:true;not null" json:"isActive"`  // 是否激活
	IsPrimary   bool       `gorm:"default:false" json:"isPrimary"`         // 是否主要角色

	// 组织约束
	DepartmentID   *uint `gorm:"index" json:"departmentId"`   // 生效部门ID（空表示全局生效）
	OrganizationID *uint `gorm:"index" json:"organizationId"` // 生效组织ID
	ApplyScope     int   `gorm:"default:1" json:"applyScope"` // 应用范围：1全局 2指定部门 3指定组织

	// 审批信息
	ApprovalStatus int        `gorm:"default:1" json:"approvalStatus"` // 审批状态：0待审批 1已批准 2已拒绝
	ApprovedBy     *uint      `gorm:"index" json:"approvedBy"`         // 审批人ID
	ApprovedAt     *time.Time `json:"approvedAt"`                      // 审批时间
	ApprovalRemark string     `gorm:"type:text" json:"approvalRemark"` // 审批备注

	// 时间信息
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联关系
	User       User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role       Role        `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	AssignUser User        `gorm:"foreignKey:AssignedBy" json:"assignUser,omitempty"`
	Approver   *User       `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`
	Department *Department `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
}

// TableName returns the table name for UserRole model.
func (*UserRole) TableName() string {
	return "user_roles"
}

// BeforeCreate hook to set default values before creating
func (ur *UserRole) BeforeCreate(tx *gorm.DB) error {
	if ur.AssignedAt.IsZero() {
		ur.AssignedAt = time.Now()
	}
	if ur.DataScope == 0 {
		ur.DataScope = 1
	}
	if ur.ApplyScope == 0 {
		ur.ApplyScope = 1
	}
	if ur.ApprovalStatus == 0 {
		ur.ApprovalStatus = 1 // 默认已批准
	}
	return nil
}

// IsExpired checks if the user role is expired
func (ur *UserRole) IsExpired() bool {
	if ur.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*ur.ExpiresAt)
}

// IsValid checks if the user role is valid (active, approved and not expired)
func (ur *UserRole) IsValid() bool {
	return ur.IsActive && ur.IsApproved() && !ur.IsExpired()
}

// IsApproved checks if the user role is approved
func (ur *UserRole) IsApproved() bool {
	return ur.ApprovalStatus == 1
}

// IsPending checks if the user role is pending approval
func (ur *UserRole) IsPending() bool {
	return ur.ApprovalStatus == 0
}

// IsRejected checks if the user role is rejected
func (ur *UserRole) IsRejected() bool {
	return ur.ApprovalStatus == 2
}

// Activate activates the user role
func (ur *UserRole) Activate() {
	ur.IsActive = true
}

// Deactivate deactivates the user role
func (ur *UserRole) Deactivate() {
	ur.IsActive = false
}

// SetAsPrimary sets this role as primary for the user
func (ur *UserRole) SetAsPrimary() {
	ur.IsPrimary = true
}

// UnsetAsPrimary unsets this role as primary for the user
func (ur *UserRole) UnsetAsPrimary() {
	ur.IsPrimary = false
}

// SetExpiry sets the expiry time for the user role
func (ur *UserRole) SetExpiry(expiresAt time.Time) {
	ur.ExpiresAt = &expiresAt
}

// RemoveExpiry removes the expiry time (makes it permanent)
func (ur *UserRole) RemoveExpiry() {
	ur.ExpiresAt = nil
}

// Approve approves the user role assignment
func (ur *UserRole) Approve(approverID uint, remark string) {
	ur.ApprovalStatus = 1
	ur.ApprovedBy = &approverID
	now := time.Now()
	ur.ApprovedAt = &now
	ur.ApprovalRemark = remark
}

// Reject rejects the user role assignment
func (ur *UserRole) Reject(approverID uint, remark string) {
	ur.ApprovalStatus = 2
	ur.ApprovedBy = &approverID
	now := time.Now()
	ur.ApprovedAt = &now
	ur.ApprovalRemark = remark
	ur.IsActive = false
}

// SetConstraint sets a constraint for the user role
func (ur *UserRole) SetConstraint(key string, value interface{}) {
	if ur.Constraints == nil {
		ur.Constraints = make(map[string]interface{})
	}
	ur.Constraints[key] = value
}

// RemoveConstraint removes a constraint from the user role
func (ur *UserRole) RemoveConstraint(key string) {
	if ur.Constraints != nil {
		delete(ur.Constraints, key)
	}
}

// MatchConstraint checks if the given conditions match the role constraints
func (ur *UserRole) MatchConstraint(conditions map[string]interface{}) bool {
	if len(ur.Constraints) == 0 {
		return true // No constraints means always match
	}

	for key, value := range ur.Constraints {
		if condValue, exists := conditions[key]; !exists || condValue != value {
			return false
		}
	}
	return true
}

// IsGlobalScope checks if the user role applies globally
func (ur *UserRole) IsGlobalScope() bool {
	return ur.ApplyScope == 1
}

// IsDepartmentScope checks if the user role applies to specific department
func (ur *UserRole) IsDepartmentScope() bool {
	return ur.ApplyScope == 2 && ur.DepartmentID != nil
}

// IsOrganizationScope checks if the user role applies to specific organization
func (ur *UserRole) IsOrganizationScope() bool {
	return ur.ApplyScope == 3 && ur.OrganizationID != nil
}
