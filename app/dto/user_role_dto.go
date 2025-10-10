package dto

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// AssignUserRoleRequest 分配用户角色请求
type AssignUserRoleRequest struct {
	IsPrimary      bool             `json:"isPrimary"`
	DepartmentID   *uint            `json:"departmentId"`
	OrganizationID *uint            `json:"organizationId"`
	ApplyScope     *int             `json:"applyScope" validate:"min=1,max=3"`
	DataScope      *int             `json:"dataScope" validate:"min=1,max=4"`
	ExpiresAt      *time.Time       `json:"expiresAt"`
	Constraints    model.JSONObject `json:"constraints,omitempty"`
	Remark         string           `json:"remark"`
}

// BatchAssignUserRolesRequest 批量分配用户角色请求
type BatchAssignUserRolesRequest struct {
	RoleIDs []uint `json:"roleIds" binding:"required,min=1"`
	Remark  string `json:"remark"`
}

// BatchAssignRoleUsersRequest 批量分配角色用户请求
type BatchAssignRoleUsersRequest struct {
	UserIDs []uint `json:"userIds" binding:"required,min=1"`
	Remark  string `json:"remark"`
}

// UpdateUserRoleRequest 更新用户角色请求
type UpdateUserRoleRequest struct {
	IsActive       *bool            `json:"isActive"`
	IsPrimary      *bool            `json:"isPrimary"`
	DepartmentID   *uint            `json:"departmentId"`
	OrganizationID *uint            `json:"organizationId"`
	ApplyScope     *int             `json:"applyScope" validate:"min=1,max=3"`
	DataScope      *int             `json:"dataScope" validate:"min=1,max=4"`
	ExpiresAt      *time.Time       `json:"expiresAt"`
	Constraints    model.JSONObject `json:"constraints,omitempty"`
	Remark         string           `json:"remark"`
}

// UserRoleQueryRequest 用户角色查询请求
type UserRoleQueryRequest struct {
	BaseQueryRequest
	UserID         *uint `form:"userId" json:"userId"`
	RoleID         *uint `form:"roleId" json:"roleId"`
	IsActive       *bool `form:"isActive" json:"isActive"`
	IsPrimary      *bool `form:"isPrimary" json:"isPrimary"`
	ApprovalStatus *int  `form:"approvalStatus" json:"approvalStatus"`
	DepartmentID   *uint `form:"departmentId" json:"departmentId"`
	AssignedBy     *uint `form:"assignedBy" json:"assignedBy"`
	ShowExpired    *bool `form:"showExpired" json:"showExpired"`
}

// ApproveUserRoleRequest 审批用户角色请求
type ApproveUserRoleRequest struct {
	Approved bool   `json:"approved" binding:"required"`
	Remark   string `json:"remark"`
}

// BatchRemoveUserRolesRequest 批量移除用户角色请求
type BatchRemoveUserRolesRequest struct {
	RoleIDs []uint `json:"roleIds" binding:"required,min=1"`
	Remark  string `json:"remark"`
}

// UserRoleResponse 用户角色响应
type UserRoleResponse struct {
	UserID             uint             `json:"userId"`
	RoleID             uint             `json:"roleId"`
	AssignedBy         uint             `json:"assignedBy"`
	AssignedAt         time.Time        `json:"assignedAt"`
	ExpiresAt          *time.Time       `json:"expiresAt"`
	IsActive           bool             `json:"isActive"`
	IsPrimary          bool             `json:"isPrimary"`
	DepartmentID       *uint            `json:"departmentId"`
	OrganizationID     *uint            `json:"organizationId"`
	ApplyScope         int              `json:"applyScope"`
	ApplyScopeName     string           `json:"applyScopeName"`
	DataScope          int              `json:"dataScope"`
	DataScopeName      string           `json:"dataScopeName"`
	ApprovalStatus     int              `json:"approvalStatus"`
	ApprovalStatusName string           `json:"approvalStatusName"`
	ApprovedBy         *uint            `json:"approvedBy"`
	ApprovedAt         *time.Time       `json:"approvedAt"`
	ApprovalRemark     string           `json:"approvalRemark"`
	Constraints        model.JSONObject `json:"constraints,omitempty"`
	CreatedAt          time.Time        `json:"createdAt"`
	UpdatedAt          time.Time        `json:"updatedAt"`

	// 关联数据
	User       *UserResponse       `json:"user,omitempty"`
	Role       *RoleResponse       `json:"role,omitempty"`
	AssignUser *UserResponse       `json:"assignUser,omitempty"`
	Approver   *UserResponse       `json:"approver,omitempty"`
	Department *DepartmentResponse `json:"department,omitempty"`

	// 计算字段
	IsExpired bool `json:"isExpired"`
	IsValid   bool `json:"isValid"`
}

// UserRoleListResponse 用户角色列表响应
type UserRoleListResponse struct {
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Size  int                 `json:"size"`
	Items []*UserRoleResponse `json:"items"`
}

// UserRoleStatsResponse 用户角色统计响应
type UserRoleStatsResponse struct {
	TotalAssignments    int64 `json:"totalAssignments"`
	ActiveAssignments   int64 `json:"activeAssignments"`
	InactiveAssignments int64 `json:"inactiveAssignments"`
	PendingApprovals    int64 `json:"pendingApprovals"`
	ApprovedAssignments int64 `json:"approvedAssignments"`
	RejectedAssignments int64 `json:"rejectedAssignments"`
	ExpiredAssignments  int64 `json:"expiredAssignments"`
}

// ConvertToUserRoleResponse 转换为用户角色响应DTO
func ConvertToUserRoleResponse(userRole *model.UserRole) UserRoleResponse {
	response := UserRoleResponse{
		UserID:             userRole.UserID,
		RoleID:             userRole.RoleID,
		AssignedBy:         userRole.AssignedBy,
		AssignedAt:         userRole.AssignedAt,
		ExpiresAt:          userRole.ExpiresAt,
		IsActive:           userRole.IsActive,
		IsPrimary:          userRole.IsPrimary,
		DepartmentID:       userRole.DepartmentID,
		OrganizationID:     userRole.OrganizationID,
		ApplyScope:         userRole.ApplyScope,
		ApplyScopeName:     GetApplyScopeName(userRole.ApplyScope),
		DataScope:          userRole.DataScope,
		DataScopeName:      GetDataScopeName(userRole.DataScope),
		ApprovalStatus:     userRole.ApprovalStatus,
		ApprovalStatusName: GetApprovalStatusName(userRole.ApprovalStatus),
		ApprovedBy:         userRole.ApprovedBy,
		ApprovedAt:         userRole.ApprovedAt,
		ApprovalRemark:     userRole.ApprovalRemark,
		Constraints:        userRole.Constraints,
		CreatedAt:          userRole.CreatedAt,
		UpdatedAt:          userRole.UpdatedAt,
		IsExpired:          userRole.IsExpired(),
		IsValid:            userRole.IsValid(),
	}

	// 转换关联数据
	if userRole.User.ID != 0 {
		user := ConvertToUserResponse(&userRole.User)
		response.User = &user
	}

	if userRole.Role.ID != 0 {
		role := ConvertToRoleResponse(&userRole.Role)
		response.Role = &role
	}

	if userRole.AssignUser.ID != 0 {
		assignUser := ConvertToUserResponse(&userRole.AssignUser)
		response.AssignUser = &assignUser
	}

	if userRole.Approver != nil && userRole.Approver.ID != 0 {
		approver := ConvertToUserResponse(userRole.Approver)
		response.Approver = &approver
	}

	if userRole.Department != nil && userRole.Department.ID != 0 {
		department := ConvertToDepartmentResponse(userRole.Department)
		response.Department = &department
	}

	return response
}

// ConvertToUserRoleResponses 转换为用户角色响应DTO列表
func ConvertToUserRoleResponses(userRoles []*model.UserRole) []*UserRoleResponse {
	var responses []*UserRoleResponse
	for _, userRole := range userRoles {
		response := ConvertToUserRoleResponse(userRole)
		responses = append(responses, &response)
	}
	return responses
}

// GetApplyScopeName 获取应用范围名称
func GetApplyScopeName(scope int) string {
	switch scope {
	case 1:
		return "全局"
	case 2:
		return "指定部门"
	case 3:
		return "指定组织"
	default:
		return "未知"
	}
}

// GetApprovalStatusName 获取审批状态名称
func GetApprovalStatusName(status int) string {
	switch status {
	case 0:
		return "待审批"
	case 1:
		return "已批准"
	case 2:
		return "已拒绝"
	default:
		return "未知"
	}
}

// UserWithRolesResponse 用户及其角色响应
type UserWithRolesResponse struct {
	*UserResponse
	Roles       []*RoleResponse `json:"roles"`
	PrimaryRole *RoleResponse   `json:"primaryRole,omitempty"`
	RoleCount   int             `json:"roleCount"`
}

// RoleWithUsersResponse 角色及其用户响应
type RoleWithUsersResponse struct {
	*RoleResponse
	Users     []*UserResponse `json:"users"`
	UserCount int             `json:"userCount"`
}
