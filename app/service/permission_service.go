package service

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// PermissionService 权限业务逻辑层
type PermissionService struct {
	permissionDao *dao.PermissionDao
	roleDao       *dao.RoleDao
	database      *gorm.DB
}

// NewPermissionService 创建权限服务实例
func NewPermissionService(database *gorm.DB) *PermissionService {
	return &PermissionService{
		permissionDao: dao.NewPermissionDao(database),
		roleDao:       dao.NewRoleDao(database),
		database:      database,
	}
}

// CreatePermission 创建权限
func (s *PermissionService) CreatePermission(req *dto.CreatePermissionRequest) (*dto.PermissionResponse, error) {
	// 验证权限类型对应的必需字段
	if err := dto.ValidatePermissionType(req); err != nil {
		return nil, err
	}

	// 验证权限名称是否已存在
	exists, err := s.permissionDao.ExistsName(req.Name)
	if err != nil {
		return nil, fmt.Errorf("检查权限名称失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("权限名称已存在")
	}

	// 验证权限代码是否已存在
	exists, err = s.permissionDao.ExistsCode(req.Code)
	if err != nil {
		return nil, fmt.Errorf("检查权限代码失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("权限代码已存在")
	}

	// 验证父权限是否存在
	if req.ParentID != nil {
		parent, err := s.permissionDao.GetByID(*req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("父权限不存在")
		}

		// 验证父权限类型是否合适
		if parent.Type == model.PermissionTypeButton || parent.Type == model.PermissionTypeAPI {
			return nil, fmt.Errorf("按钮权限和API权限不能作为父权限")
		}
	}

	// 创建权限对象
	permission := &model.Permission{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Type:        *req.Type,
		ParentID:    req.ParentID,
		Resource:    req.Resource,
		Action:      req.Action,
		Method:      req.Method,
		URL:         req.URL,
		Component:   req.Component,
		Icon:        req.Icon,
		Redirect:    req.Redirect,
		External:    req.External,
		Hidden:      req.Hidden,
		KeepAlive:   req.KeepAlive,
		ShowInMenu:  req.ShowInMenu,
		Status:      model.PermissionStatusEnabled,
		Attributes:  req.Attributes,
		Remark:      req.Remark,
	}

	// 设置排序
	if req.Sort != nil {
		permission.Sort = *req.Sort
	}

	// 保存到数据库
	if err := s.permissionDao.Create(permission); err != nil {
		return nil, err
	}

	// 重新查询获取完整信息
	permission, err = s.permissionDao.GetByID(permission.ID)
	if err != nil {
		return nil, err
	}

	// 转换为响应DTO
	response := dto.ConvertToPermissionResponse(permission)
	return &response, nil
}

// GetPermissionByID 根据ID获取权限
func (s *PermissionService) GetPermissionByID(id uint) (*dto.PermissionResponse, error) {
	permission, err := s.permissionDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 获取角色数量
	roleCount, err := s.getRoleCountByPermission(id)
	if err != nil {
		roleCount = 0 // 忽略统计错误
	}

	response := dto.ConvertToPermissionResponse(permission)
	response.RoleCount = roleCount
	return &response, nil
}

// UpdatePermission 更新权限信息
func (s *PermissionService) UpdatePermission(id uint, req *dto.UpdatePermissionRequest) (*dto.PermissionResponse, error) {
	// 获取权限
	permission, err := s.permissionDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 准备更新的字段
	updates := make(map[string]interface{})

	// 验证权限名称是否已存在（排除自己）
	if req.Name != nil && *req.Name != permission.Name {
		exists, err := s.permissionDao.ExistsName(*req.Name, id)
		if err != nil {
			return nil, fmt.Errorf("检查权限名称失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("权限名称已存在")
		}
		updates["name"] = *req.Name
	}

	// 更新字段 - 使用指针来区分"未传值"和"传了空值"
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Resource != nil {
		updates["resource"] = *req.Resource
	}
	if req.Action != nil {
		updates["action"] = *req.Action
	}
	if req.Method != nil {
		updates["method"] = *req.Method
	}
	if req.URL != nil {
		updates["url"] = *req.URL
	}
	if req.Component != nil {
		updates["component"] = *req.Component
	}
	if req.Icon != nil {
		updates["icon"] = *req.Icon
	}
	if req.Redirect != nil {
		updates["redirect"] = *req.Redirect
	}
	if req.External != nil {
		updates["external"] = *req.External
	}
	if req.Hidden != nil {
		updates["hidden"] = *req.Hidden
	}
	if req.KeepAlive != nil {
		updates["keep_alive"] = *req.KeepAlive
	}
	if req.ShowInMenu != nil {
		updates["show_in_menu"] = *req.ShowInMenu
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	if req.Attributes != nil {
		updates["attributes"] = req.Attributes
	}

	// 保存更新
	if err := s.permissionDao.UpdateById(id, updates); err != nil {
		return nil, err
	}

	// 重新查询获取完整信息
	permission, err = s.permissionDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	response := dto.ConvertToPermissionResponse(permission)
	return &response, nil
}

// DeletePermission 删除权限
func (s *PermissionService) DeletePermission(id uint) error {
	// 检查权限是否存在
	_, err := s.permissionDao.GetByID(id)
	if err != nil {
		return err
	}

	// 检查是否有子权限
	children, err := s.permissionDao.GetChildrenIDs(id)
	if err != nil {
		return fmt.Errorf("检查子权限失败: %w", err)
	}
	if len(children) > 0 {
		return fmt.Errorf("存在子权限，不能删除")
	}

	// 检查是否有角色使用此权限
	roleCount, err := s.getRoleCountByPermission(id)
	if err != nil {
		return fmt.Errorf("检查角色关联失败: %w", err)
	}
	if roleCount > 0 {
		return fmt.Errorf("有 %d 个角色使用此权限，不能删除", roleCount)
	}

	return s.permissionDao.Delete(id)
}

// GetPermissionList 获取权限列表
func (s *PermissionService) GetPermissionList(req *dto.PermissionQueryRequest) (*dto.PermissionListResponse, error) {
	permissions, total, err := s.permissionDao.List(req)
	if err != nil {
		return nil, err
	}

	// 调试日志：检查是否加载了 Children
	for i, p := range permissions {
		if i < 3 { // 只打印前3条
			fmt.Printf("[DEBUG] Permission ID=%d, Name=%s, Children count=%d, Parent is nil=%v\n",
				p.ID, p.Name, len(p.Children), p.Parent == nil)
		}
	}

	// 转换为响应DTO
	permissionResponses := dto.ConvertToPermissionResponses(permissions)

	// 设置默认分页参数
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	return &dto.PermissionListResponse{
		Total: int64(total),
		Page:  page,
		Size:  pageSize,
		Items: permissionResponses,
	}, nil
}

// GetPermissionTree 获取权限树形结构
func (s *PermissionService) GetPermissionTree() (*dto.PermissionTreeResponse, error) {
	permissions, err := s.permissionDao.GetPermissionTree()
	if err != nil {
		return nil, err
	}

	permissionResponses := dto.ConvertToPermissionResponses(permissions)
	return &dto.PermissionTreeResponse{
		Permissions: permissionResponses,
	}, nil
}

// GetMenuTree 获取菜单树形结构
func (s *PermissionService) GetMenuTree() (*dto.MenuTreeResponse, error) {
	permissions, err := s.permissionDao.GetMenuPermissions()
	if err != nil {
		return nil, err
	}

	permissionResponses := dto.ConvertToPermissionResponses(permissions)
	menuTree := dto.BuildMenuTree(permissionResponses)

	return &dto.MenuTreeResponse{
		Menus: menuTree,
	}, nil
}

// GetUserMenuTree 获取用户的菜单树
func (s *PermissionService) GetUserMenuTree(userID uint) (*dto.MenuTreeResponse, error) {
	permissions, err := s.permissionDao.GetPermissionsByUserID(userID)
	if err != nil {
		return nil, err
	}

	// 过滤菜单权限
	var menuPermissions []*model.Permission
	for _, permission := range permissions {
		if permission.IsMenu() && permission.ShowInMenu && !permission.Hidden {
			menuPermissions = append(menuPermissions, permission)
		}
	}

	permissionResponses := dto.ConvertToPermissionResponses(menuPermissions)
	menuTree := dto.BuildMenuTree(permissionResponses)

	return &dto.MenuTreeResponse{
		Menus: menuTree,
	}, nil
}

// UpdatePermissionStatus 更新权限状态
func (s *PermissionService) UpdatePermissionStatus(id uint, req *dto.PermissionStatusUpdateRequest) error {
	// 检查权限是否存在
	_, err := s.permissionDao.GetByID(id)
	if err != nil {
		return err
	}

	// 更新状态
	updates := map[string]interface{}{
		"status": req.Status,
	}
	if req.Remark != "" {
		updates["remark"] = req.Remark
	}

	return s.permissionDao.UpdateById(id, updates)
}

// BatchOperation 批量操作权限
func (s *PermissionService) BatchOperation(req *dto.BatchPermissionRequest) error {
	if len(req.PermissionIDs) == 0 {
		return fmt.Errorf("权限ID列表不能为空")
	}

	switch req.Action {
	case "enable":
		return s.permissionDao.BatchUpdateStatus(req.PermissionIDs, model.PermissionStatusEnabled)
	case "disable":
		return s.permissionDao.BatchUpdateStatus(req.PermissionIDs, model.PermissionStatusDisabled)
	case "delete":
		// 检查是否有角色关联
		for _, permissionID := range req.PermissionIDs {
			roleCount, err := s.getRoleCountByPermission(permissionID)
			if err != nil {
				return fmt.Errorf("检查权限 %d 角色关联失败", permissionID)
			}
			if roleCount > 0 {
				return fmt.Errorf("权限 %d 有角色关联，不能删除", permissionID)
			}
		}
		return s.permissionDao.BatchDelete(req.PermissionIDs)
	default:
		return fmt.Errorf("不支持的操作类型: %s", req.Action)
	}
}

// GetPermissionStats 获取权限统计信息
func (s *PermissionService) GetPermissionStats() (*dto.PermissionStatsResponse, error) {
	typeCounts, err := s.permissionDao.CountPermissionsByType()
	if err != nil {
		return nil, err
	}

	statusCounts, err := s.permissionDao.CountPermissionsByStatus()
	if err != nil {
		return nil, err
	}

	var total int64
	for _, count := range statusCounts {
		total += count
	}

	return &dto.PermissionStatsResponse{
		TotalPermissions:    total,
		EnabledPermissions:  statusCounts["enabled"],
		DisabledPermissions: statusCounts["disabled"],
		MenuPermissions:     typeCounts["menu"],
		ButtonPermissions:   typeCounts["button"],
		APIPermissions:      typeCounts["api"],
		DataPermissions:     typeCounts["data"],
	}, nil
}

// CheckUserPermission 检查用户是否有特定权限
func (s *PermissionService) CheckUserPermission(userID uint, resource, action string) (bool, error) {
	permissions, err := s.permissionDao.GetPermissionsByUserID(userID)
	if err != nil {
		return false, err
	}

	for _, permission := range permissions {
		if permission.MatchResource(resource, action) {
			return true, nil
		}
	}

	return false, nil
}

// CheckUserURLPermission 检查用户是否有URL访问权限
func (s *PermissionService) CheckUserURLPermission(userID uint, url, method string) (bool, error) {
	permissions, err := s.permissionDao.GetPermissionsByUserID(userID)
	if err != nil {
		return false, err
	}

	for _, permission := range permissions {
		if permission.MatchURL(url, method) {
			return true, nil
		}
	}

	return false, nil
}

// GetUserPermissions 获取用户的权限列表
func (s *PermissionService) GetUserPermissions(userID uint) ([]*dto.PermissionResponse, error) {
	permissions, err := s.permissionDao.GetPermissionsByUserID(userID)
	if err != nil {
		return nil, err
	}

	return dto.ConvertToPermissionResponses(permissions), nil
}

// getRoleCountByPermission 获取权限关联的角色数量
func (s *PermissionService) getRoleCountByPermission(permissionID uint) (int64, error) {
	var count int64
	if err := s.database.Model(&model.RolePermission{}).
		Where("permission_id = ? AND deleted_at IS NULL", permissionID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
