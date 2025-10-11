package dto

import (
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/datatypes"
)

// CreatePermissionRequest 创建权限请求
type CreatePermissionRequest struct {
	Name        string                `json:"name" binding:"required,min=2,max=100" validate:"required"`
	Code        string                `json:"code" binding:"required,min=2,max=100" validate:"required"`
	Description string                `json:"description" validate:"max=255"`
	Type        *model.PermissionType `json:"type" binding:"required"`
	ParentID    *uint                 `json:"parentId"`
	Resource    string                `json:"resource" validate:"max=255"`
	Action      string                `json:"action" validate:"max=100"`
	Method      string                `json:"method" validate:"max=20"`
	URL         string                `json:"url" validate:"max=255"`
	Component   string                `json:"component" validate:"max=255"`
	Icon        string                `json:"icon" validate:"max=100"`
	Redirect    string                `json:"redirect" validate:"max=255"`
	External    bool                  `json:"external"`
	Hidden      bool                  `json:"hidden"`
	KeepAlive   bool                  `json:"keepAlive"`
	ShowInMenu  bool                  `json:"showInMenu"`
	Sort        *int                  `json:"sort"`
	Remark      string                `json:"remark"`
	Attributes  datatypes.JSON        `json:"attributes,omitempty"`
}

// UpdatePermissionRequest 更新权限请求
type UpdatePermissionRequest struct {
	Name        *string        `json:"name" validate:"omitempty,min=2,max=100"`
	Description *string        `json:"description" validate:"omitempty,max=255"`
	Resource    *string        `json:"resource" validate:"omitempty,max=255"`
	Action      *string        `json:"action" validate:"omitempty,max=100"`
	Method      *string        `json:"method" validate:"omitempty,max=20"`
	URL         *string        `json:"url" validate:"omitempty,max=255"`
	Component   *string        `json:"component" validate:"omitempty,max=255"`
	Icon        *string        `json:"icon" validate:"omitempty,max=100"`
	Redirect    *string        `json:"redirect" validate:"omitempty,max=255"`
	External    *bool          `json:"external"`
	Hidden      *bool          `json:"hidden"`
	KeepAlive   *bool          `json:"keepAlive"`
	ShowInMenu  *bool          `json:"showInMenu"`
	Sort        *int           `json:"sort"`
	Remark      *string        `json:"remark"`
	Attributes  datatypes.JSON `json:"attributes,omitempty"`
}

// PermissionQueryRequest 权限查询请求
type PermissionQueryRequest struct {
	BaseQueryRequest
	Name     string                  `form:"name" json:"name"`
	Code     string                  `form:"code" json:"code"`
	Type     *model.PermissionType   `form:"type" json:"type"`
	Status   *model.PermissionStatus `form:"status" json:"status"`
	ParentID *uint                   `form:"parentId" json:"parentId"`
	Resource string                  `form:"resource" json:"resource"`
}

// PermissionStatusUpdateRequest 权限状态更新请求
type PermissionStatusUpdateRequest struct {
	Status model.PermissionStatus `json:"status" binding:"required"`
	Remark string                 `json:"remark"`
}

// BatchPermissionRequest 批量权限操作请求
type BatchPermissionRequest struct {
	PermissionIDs []uint `json:"permissionIds" binding:"required,min=1"`
	Action        string `json:"action" binding:"required"`
	Remark        string `json:"remark"`
}

// PermissionResponse 权限响应
type PermissionResponse struct {
	ID          uint                   `json:"id"`
	Name        string                 `json:"name"`
	Code        string                 `json:"code"`
	Description string                 `json:"description"`
	Type        model.PermissionType   `json:"type"`
	TypeName    string                 `json:"typeName"`
	Status      model.PermissionStatus `json:"status"`
	StatusName  string                 `json:"statusName"`
	ParentID    *uint                  `json:"parentId"`
	Level       int                    `json:"level"`
	Path        string                 `json:"path"`
	Resource    string                 `json:"resource"`
	Action      string                 `json:"action"`
	Method      string                 `json:"method"`
	URL         string                 `json:"url"`
	Component   string                 `json:"component"`
	Icon        string                 `json:"icon"`
	Redirect    string                 `json:"redirect"`
	External    bool                   `json:"external"`
	Hidden      bool                   `json:"hidden"`
	KeepAlive   bool                   `json:"keepAlive"`
	ShowInMenu  bool                   `json:"showInMenu"`
	Sort        int                    `json:"sort"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	Attributes  datatypes.JSON         `json:"attributes,omitempty"`
	Remark      string                 `json:"remark"`

	// 关联数据
	Parent    *PermissionResponse   `json:"parent,omitempty"`
	Children  []*PermissionResponse `json:"children,omitempty"`
	RoleCount int64                 `json:"roleCount,omitempty"`
}

// PermissionListResponse 权限列表响应
type PermissionListResponse struct {
	Total int64                 `json:"total"`
	Page  int                   `json:"page"`
	Size  int                   `json:"size"`
	Items []*PermissionResponse `json:"items"`
}

// PermissionTreeResponse 权限树响应
type PermissionTreeResponse struct {
	Permissions []*PermissionResponse `json:"permissions"`
}

// PermissionStatsResponse 权限统计响应
type PermissionStatsResponse struct {
	TotalPermissions    int64 `json:"totalPermissions"`
	EnabledPermissions  int64 `json:"enabledPermissions"`
	DisabledPermissions int64 `json:"disabledPermissions"`
	MenuPermissions     int64 `json:"menuPermissions"`
	ButtonPermissions   int64 `json:"buttonPermissions"`
	APIPermissions      int64 `json:"apiPermissions"`
	DataPermissions     int64 `json:"dataPermissions"`
}

// MenuTreeResponse 菜单树响应
type MenuTreeResponse struct {
	Menus []*PermissionResponse `json:"menus"`
}

// ConvertToPermissionResponse 转换为权限响应DTO
func ConvertToPermissionResponse(permission *model.Permission) PermissionResponse {
	response := PermissionResponse{
		ID:          permission.ID,
		Name:        permission.Name,
		Code:        permission.Code,
		Description: permission.Description,
		Type:        permission.Type,
		TypeName:    permission.Type.String(),
		Status:      permission.Status,
		StatusName:  permission.Status.String(),
		ParentID:    permission.ParentID,
		Level:       permission.Level,
		Path:        permission.Path,
		Resource:    permission.Resource,
		Action:      permission.Action,
		Method:      permission.Method,
		URL:         permission.URL,
		Component:   permission.Component,
		Icon:        permission.Icon,
		Redirect:    permission.Redirect,
		External:    permission.External,
		Hidden:      permission.Hidden,
		KeepAlive:   permission.KeepAlive,
		ShowInMenu:  permission.ShowInMenu,
		Sort:        permission.Sort,
		CreatedAt:   permission.CreatedAt,
		UpdatedAt:   permission.UpdatedAt,
		Attributes:  permission.Attributes,
		Remark:      permission.Remark,
	}

	// 转换父权限
	if permission.Parent != nil {
		parent := ConvertToPermissionResponse(permission.Parent)
		response.Parent = &parent
	}

	// 转换子权限
	if len(permission.Children) > 0 {
		for _, child := range permission.Children {
			childResponse := ConvertToPermissionResponse(&child)
			response.Children = append(response.Children, &childResponse)
		}
	}

	return response
}

// ConvertToPermissionResponses 转换为权限响应DTO列表
func ConvertToPermissionResponses(permissions []*model.Permission) []*PermissionResponse {
	// 初始化为空切片而非nil，确保JSON序列化为[]而不是null
	responses := make([]*PermissionResponse, 0)
	for _, permission := range permissions {
		response := ConvertToPermissionResponse(permission)
		responses = append(responses, &response)
	}
	return responses
}

// GetPermissionTypeName 获取权限类型名称
func GetPermissionTypeName(permType model.PermissionType) string {
	switch permType {
	case model.PermissionTypeMenu:
		return "菜单权限"
	case model.PermissionTypeButton:
		return "按钮权限"
	case model.PermissionTypeAPI:
		return "API权限"
	case model.PermissionTypeData:
		return "数据权限"
	default:
		return "未知类型"
	}
}

// ValidatePermissionType 验证权限类型对应的字段
func ValidatePermissionType(req *CreatePermissionRequest) error {
	if req.Type == nil {
		return fmt.Errorf("权限类型不能为空")
	}

	switch *req.Type {
	case model.PermissionTypeMenu:
		if req.Component == "" && !req.External {
			return fmt.Errorf("菜单权限必须指定组件路径或设置为外部链接")
		}
	case model.PermissionTypeButton:
		if req.Resource == "" || req.Action == "" {
			return fmt.Errorf("按钮权限必须指定资源和操作")
		}
	case model.PermissionTypeAPI:
		if req.URL == "" || req.Method == "" {
			return fmt.Errorf("API权限必须指定URL和HTTP方法")
		}
	case model.PermissionTypeData:
		if req.Resource == "" {
			return fmt.Errorf("数据权限必须指定资源")
		}
	}
	return nil
}

// ValidatePermissionHierarchy 验证权限层级关系（防止循环引用）
func ValidatePermissionHierarchy(parentID, childID uint, existingPermissions []*model.Permission) error {
	if parentID == childID {
		return fmt.Errorf("权限不能设置自己为父权限")
	}

	// 构建权限关系图
	permissionMap := make(map[uint]*model.Permission)
	for _, permission := range existingPermissions {
		permissionMap[permission.ID] = permission
	}

	// 检查是否会形成循环
	visited := make(map[uint]bool)
	var checkCycle func(uint) bool
	checkCycle = func(id uint) bool {
		if visited[id] {
			return true // 发现循环
		}
		visited[id] = true

		if permission, exists := permissionMap[id]; exists && permission.ParentID != nil {
			return checkCycle(*permission.ParentID)
		}
		return false
	}

	// 模拟设置父权限后检查循环
	if permission, exists := permissionMap[childID]; exists {
		originalParentID := permission.ParentID
		permission.ParentID = &parentID
		defer func() { permission.ParentID = originalParentID }()

		if checkCycle(childID) {
			return fmt.Errorf("设置父权限会导致循环引用")
		}
	}

	return nil
}

// BuildMenuTree 构建菜单树形结构
func BuildMenuTree(permissions []*PermissionResponse) []*PermissionResponse {
	// 创建ID到权限的映射
	permissionMap := make(map[uint]*PermissionResponse)
	var rootPermissions []*PermissionResponse

	// 第一遍遍历，建立映射
	for _, permission := range permissions {
		permissionMap[permission.ID] = permission
	}

	// 第二遍遍历，建立层级关系
	for _, permission := range permissions {
		if permission.ParentID == nil {
			// 根节点
			rootPermissions = append(rootPermissions, permission)
		} else {
			// 子节点
			if parent, exists := permissionMap[*permission.ParentID]; exists {
				parent.Children = append(parent.Children, permission)
			}
		}
	}

	return rootPermissions
}
