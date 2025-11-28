package dto

import (
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/datatypes"
)

// CreateRoleRequest 创建角色请求
type CreateRoleRequest struct {
	Name        string          `json:"name" binding:"required,min=2,max=50" validate:"required"`
	Code        string          `json:"code" binding:"required,min=2,max=50" validate:"required"`
	Description string          `json:"description" validate:"max=255"`
	Type        *model.RoleType `json:"type"`
	ParentID    *uint           `json:"parentId"`
	DataScope   *int            `json:"dataScope" validate:"min=1,max=4"`
	ScopeDesc   string          `json:"scopeDesc" validate:"max=255"`
	Sort        *int            `json:"sort"`
	Remark      string          `json:"remark"`
	Attributes  datatypes.JSON  `json:"attributes,omitempty"`
}

// UpdateRoleRequest 更新角色请求
type UpdateRoleRequest struct {
	Name        string         `json:"name" validate:"min=2,max=50"`
	Code        string         `json:"code" validate:"omitempty,min=2,max=50"`
	Description string         `json:"description" validate:"max=255"`
	DataScope   *int           `json:"dataScope" validate:"min=1,max=4"`
	ScopeDesc   string         `json:"scopeDesc" validate:"max=255"`
	Sort        *int           `json:"sort"`
	Remark      string         `json:"remark"`
	Attributes  datatypes.JSON `json:"attributes,omitempty"`
}

// RoleQueryRequest 角色查询请求
type RoleQueryRequest struct {
	BaseQueryRequest
	Name            string            `form:"name" json:"name"`
	Code            string            `form:"code" json:"code"`
	Type            *model.RoleType   `form:"type" json:"type"`
	Status          *model.RoleStatus `form:"status" json:"status"`
	ParentID        *uint             `form:"parentId" json:"parentId"`
	IncludeChildren bool              `form:"includeChildren" json:"includeChildren"` // 是否递归查询所有子孙角色
}

// RoleStatusUpdateRequest 角色状态更新请求
type RoleStatusUpdateRequest struct {
	Status model.RoleStatus `json:"status" binding:"gte=0,lte=1"`
	Remark string           `json:"remark"`
}

// BatchRoleRequest 批量角色操作请求
type BatchRoleRequest struct {
	RoleIDs []uint `json:"roleIds" binding:"required,min=1"`
	Action  string `json:"action" binding:"required"`
	Remark  string `json:"remark"`
}

// AssignPermissionsRequest 分配权限请求
type AssignPermissionsRequest struct {
	PermissionIDs []uint `json:"permissionIds" binding:"required"`
}

// AssignUsersRequest 分配用户请求
type AssignUsersRequest struct {
	UserIDs []uint `json:"userIds" binding:"required"`
}

// RoleResponse 角色响应
type RoleResponse struct {
	ID          uint             `json:"id"`
	Name        string           `json:"name"`
	Code        string           `json:"code"`
	Description string           `json:"description"`
	Type        model.RoleType   `json:"type"`
	TypeName    string           `json:"typeName"`
	Status      model.RoleStatus `json:"status"`
	StatusName  string           `json:"statusName"`
	ParentID    *uint            `json:"parentId"`
	Level       int              `json:"level"`
	Path        string           `json:"path"`
	DataScope   int              `json:"dataScope"`
	ScopeDesc   string           `json:"scopeDesc"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	Attributes  datatypes.JSON   `json:"attributes,omitempty"`
	Remark      string           `json:"remark"`

	// 关联数据
	Parent      *RoleResponse         `json:"parent,omitempty"`
	Children    []*RoleResponse       `json:"children,omitempty"`
	Permissions []*PermissionResponse `json:"permissions,omitempty"`
	UserCount   int64                 `json:"userCount,omitempty"`
}

// RoleListResponse 角色列表响应
type RoleListResponse struct {
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
	Items []*RoleResponse `json:"items"`
}

// RoleTreeResponse 角色树响应
type RoleTreeResponse struct {
	Roles []*RoleResponse `json:"roles"`
}

// RoleStatsResponse 角色统计响应
type RoleStatsResponse struct {
	TotalRoles    int64 `json:"totalRoles"`
	EnabledRoles  int64 `json:"enabledRoles"`
	DisabledRoles int64 `json:"disabledRoles"`
	SystemRoles   int64 `json:"systemRoles"`
	CustomRoles   int64 `json:"customRoles"`
}

// ConvertToRoleResponse 转换为角色响应DTO
func ConvertToRoleResponse(role *model.Role) RoleResponse {
	// 处理指针类型的字段
	var roleType model.RoleType
	var typeName string
	if role.Type != nil {
		roleType = *role.Type
		typeName = role.Type.String()
	}

	var roleStatus model.RoleStatus
	var statusName string
	if role.Status != nil {
		roleStatus = *role.Status
		statusName = role.Status.String()
	}

	var level int
	if role.Level != nil {
		level = *role.Level
	}

	var dataScope int
	if role.DataScope != nil {
		dataScope = *role.DataScope
	}

	response := RoleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Code:        role.Code,
		Description: role.Description,
		Type:        roleType,
		TypeName:    typeName,
		Status:      roleStatus,
		StatusName:  statusName,
		ParentID:    role.ParentID,
		Level:       level,
		Path:        role.Path,
		DataScope:   dataScope,
		ScopeDesc:   role.ScopeDesc,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
		Attributes:  role.Attributes,
		Remark:      role.Remark,
	}

	// 转换父角色
	if role.Parent != nil {
		parent := ConvertToRoleResponse(role.Parent)
		response.Parent = &parent
	}

	// 转换子角色
	if len(role.Children) > 0 {
		for _, child := range role.Children {
			childResponse := ConvertToRoleResponse(&child)
			response.Children = append(response.Children, &childResponse)
		}
	}

	// 转换权限
	if len(role.Permissions) > 0 {
		for _, permission := range role.Permissions {
			permissionResponse := ConvertToPermissionResponse(&permission)
			response.Permissions = append(response.Permissions, &permissionResponse)
		}
	}

	return response
}

// ConvertToRoleResponses 转换为角色响应DTO列表
func ConvertToRoleResponses(roles []*model.Role) []*RoleResponse {
	// 初始化为空切片而非nil，确保JSON序列化为[]而不是null
	responses := make([]*RoleResponse, 0)
	for _, role := range roles {
		response := ConvertToRoleResponse(role)
		responses = append(responses, &response)
	}
	return responses
}

// GetDataScopeName 获取数据权限范围名称
func GetDataScopeName(scope int) string {
	switch scope {
	case 1:
		return "全部数据"
	case 2:
		return "本部门数据"
	case 3:
		return "本部门及下级数据"
	case 4:
		return "仅本人数据"
	default:
		return "未知"
	}
}

// ValidateRoleHierarchy 验证角色层级关系（防止循环引用）
func ValidateRoleHierarchy(parentID, childID uint, existingRoles []*model.Role) error {
	if parentID == childID {
		return fmt.Errorf("角色不能设置自己为父角色")
	}

	// 构建角色关系图
	roleMap := make(map[uint]*model.Role)
	for _, role := range existingRoles {
		roleMap[role.ID] = role
	}

	// 检查是否会形成循环
	visited := make(map[uint]bool)
	var checkCycle func(uint) bool
	checkCycle = func(id uint) bool {
		if visited[id] {
			return true // 发现循环
		}
		visited[id] = true

		if role, exists := roleMap[id]; exists && role.ParentID != nil {
			return checkCycle(*role.ParentID)
		}
		return false
	}

	// 模拟设置父角色后检查循环
	if role, exists := roleMap[childID]; exists {
		originalParentID := role.ParentID
		role.ParentID = &parentID
		defer func() { role.ParentID = originalParentID }()

		if checkCycle(childID) {
			return fmt.Errorf("设置父角色会导致循环引用")
		}
	}

	return nil
}
