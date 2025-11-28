package service

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// RoleService 角色业务逻辑层
type RoleService struct {
	roleDao  *dao.RoleDao
	userDao  *dao.UserDao
	database *gorm.DB
}

// NewRoleService 创建角色服务实例
func NewRoleService(database *gorm.DB) *RoleService {
	return &RoleService{
		roleDao:  dao.NewRoleDao(database),
		userDao:  dao.NewUserDao(database),
		database: database,
	}
}

// CreateRole 创建角色
func (s *RoleService) CreateRole(req *dto.CreateRoleRequest) (*dto.RoleResponse, error) {
	// 验证角色名称是否已存在
	exists, err := s.roleDao.ExistsName(req.Name)
	if err != nil {
		return nil, fmt.Errorf("检查角色名称失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("角色名称已存在")
	}

	// 验证角色代码是否已存在
	exists, err = s.roleDao.ExistsCode(req.Code)
	if err != nil {
		return nil, fmt.Errorf("检查角色代码失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("角色代码已存在")
	}

	// 验证父角色是否存在
	if req.ParentID != nil {
		_, err := s.roleDao.GetByID(*req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("父角色不存在")
		}
	}

	// 创建角色对象
	status := model.RoleStatusEnabled
	role := &model.Role{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		ParentID:    req.ParentID,
		Status:      &status,
		Attributes:  req.Attributes,
		Remark:      req.Remark,
	}

	// 设置角色类型
	if req.Type != nil {
		role.Type = req.Type
	} else {
		custom := model.RoleTypeCustom
		role.Type = &custom
	}

	// 设置数据权限范围
	if req.DataScope != nil {
		role.DataScope = req.DataScope
	} else {
		defaultScope := 2 // 默认本部门数据
		role.DataScope = &defaultScope
	}
	role.ScopeDesc = req.ScopeDesc

	// 保存到数据库
	if err := s.roleDao.Create(role); err != nil {
		return nil, err
	}

	// 重新查询获取完整信息
	role, err = s.roleDao.GetByID(role.ID)
	if err != nil {
		return nil, err
	}

	// 转换为响应DTO
	response := dto.ConvertToRoleResponse(role)
	return &response, nil
}

// GetRoleByID 根据ID获取角色
func (s *RoleService) GetRoleByID(id uint) (*dto.RoleResponse, error) {
	role, err := s.roleDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 获取用户数量
	userCount, err := s.getUserCountByRole(id)
	if err != nil {
		userCount = 0 // 忽略统计错误
	}

	response := dto.ConvertToRoleResponse(role)
	response.UserCount = userCount
	return &response, nil
}

// UpdateRole 更新角色信息
func (s *RoleService) UpdateRole(id uint, req *dto.UpdateRoleRequest, currentUserID uint) (*dto.RoleResponse, error) {
	// 获取角色
	role, err := s.roleDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 检查是否是系统角色
	if role.IsSystem() {
		return nil, fmt.Errorf("系统角色不允许修改")
	}

	// 特别保护超级管理员角色
	if role.Code == "super_admin" {
		return nil, fmt.Errorf("超级管理员角色不允许修改")
	}

	// 检查当前用户是否有权限操作此角色
	hasPermission, err := s.checkRoleOperationPermission(currentUserID, role)
	if err != nil {
		return nil, fmt.Errorf("权限检查失败: %w", err)
	}
	if !hasPermission {
		return nil, fmt.Errorf("权限不足，无法操作此角色")
	}

	// 验证角色名称是否已存在（排除自己）
	if req.Name != "" && req.Name != role.Name {
		exists, err := s.roleDao.ExistsName(req.Name, id)
		if err != nil {
			return nil, fmt.Errorf("检查角色名称失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("角色名称已存在")
		}
		role.Name = req.Name
	}

	// 验证角色代码是否已存在（排除自己）
	if req.Code != "" && req.Code != role.Code {
		exists, err := s.roleDao.ExistsCode(req.Code, id)
		if err != nil {
			return nil, fmt.Errorf("检查角色代码失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("角色代码已存在")
		}
		role.Code = req.Code
	}

	// 更新字段
	if req.Description != "" {
		role.Description = req.Description
	}
	if req.DataScope != nil {
		role.DataScope = req.DataScope
	}
	if req.ScopeDesc != "" {
		role.ScopeDesc = req.ScopeDesc
	}
	if req.Remark != "" {
		role.Remark = req.Remark
	}
	if req.Attributes != nil {
		role.Attributes = req.Attributes
	}

	// 保存更新
	if err := s.roleDao.Update(role); err != nil {
		return nil, err
	}

	response := dto.ConvertToRoleResponse(role)
	return &response, nil
}

// DeleteRole 删除角色
func (s *RoleService) DeleteRole(id uint, currentUserID uint) error {
	// 检查角色是否存在
	role, err := s.roleDao.GetByID(id)
	if err != nil {
		return err
	}

	// 检查是否是系统角色
	if role.IsSystem() {
		return fmt.Errorf("系统角色不允许删除")
	}

	// 特别保护超级管理员角色
	if role.Code == "super_admin" {
		return fmt.Errorf("超级管理员角色不允许删除")
	}

	// 检查当前用户是否有权限操作此角色
	hasPermission, err := s.checkRoleOperationPermission(currentUserID, role)
	if err != nil {
		return fmt.Errorf("权限检查失败: %w", err)
	}
	if !hasPermission {
		return fmt.Errorf("权限不足，无法操作此角色")
	}

	// 检查是否有子角色
	children, err := s.roleDao.GetChildrenIDs(id)
	if err != nil {
		return fmt.Errorf("检查子角色失败: %w", err)
	}
	if len(children) > 0 {
		return fmt.Errorf("存在子角色，不能删除")
	}

	// 检查是否有用户使用此角色
	userCount, err := s.getUserCountByRole(id)
	if err != nil {
		return fmt.Errorf("检查用户关联失败: %w", err)
	}
	if userCount > 0 {
		return fmt.Errorf("有 %d 个用户使用此角色，不能删除", userCount)
	}

	return s.roleDao.Delete(id)
}

// GetRoleList 获取角色列表
func (s *RoleService) GetRoleList(req *dto.RoleQueryRequest) (*dto.RoleListResponse, error) {
	roles, total, err := s.roleDao.List(req)
	if err != nil {
		return nil, err
	}

	// 转换为响应DTO
	roleResponses := dto.ConvertToRoleResponses(roles)

	// 设置默认分页参数
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	return &dto.RoleListResponse{
		Total: int64(total),
		Page:  page,
		Size:  pageSize,
		Items: roleResponses,
	}, nil
}

// GetRoleTree 获取角色树形结构
func (s *RoleService) GetRoleTree() (*dto.RoleTreeResponse, error) {
	roles, err := s.roleDao.GetRoleTree()
	if err != nil {
		return nil, err
	}

	roleResponses := dto.ConvertToRoleResponses(roles)
	return &dto.RoleTreeResponse{
		Roles: roleResponses,
	}, nil
}

// UpdateRoleStatus 更新角色状态
func (s *RoleService) UpdateRoleStatus(id uint, req *dto.RoleStatusUpdateRequest, currentUserID uint) error {
	// 检查角色是否存在
	role, err := s.roleDao.GetByID(id)
	if err != nil {
		return err
	}

	// 检查是否是系统角色
	if role.IsSystem() {
		return fmt.Errorf("系统角色不允许修改状态")
	}

	// 特别保护超级管理员角色
	if role.Code == "super_admin" {
		return fmt.Errorf("超级管理员角色不允许修改状态")
	}

	// 检查当前用户是否有权限操作此角色
	hasPermission, err := s.checkRoleOperationPermission(currentUserID, role)
	if err != nil {
		return fmt.Errorf("权限检查失败: %w", err)
	}
	if !hasPermission {
		return fmt.Errorf("权限不足，无法操作此角色")
	}

	// 更新状态
	updates := map[string]interface{}{
		"status": req.Status,
	}
	if req.Remark != "" {
		updates["remark"] = req.Remark
	}

	return s.roleDao.UpdateById(id, updates)
}

// BatchOperation 批量操作角色
func (s *RoleService) BatchOperation(req *dto.BatchRoleRequest, currentUserID uint) error {
	if len(req.RoleIDs) == 0 {
		return fmt.Errorf("角色ID列表不能为空")
	}

	// 检查是否包含系统角色和超级管理员角色，并验证权限
	for _, roleID := range req.RoleIDs {
		role, err := s.roleDao.GetByID(roleID)
		if err != nil {
			return fmt.Errorf("角色 %d 不存在", roleID)
		}
		if role.IsSystem() {
			return fmt.Errorf("系统角色不允许批量操作")
		}
		if role.Code == "super_admin" {
			return fmt.Errorf("超级管理员角色不允许批量操作")
		}

		// 检查当前用户是否有权限操作此角色
		hasPermission, err := s.checkRoleOperationPermission(currentUserID, role)
		if err != nil {
			return fmt.Errorf("权限检查失败: %w", err)
		}
		if !hasPermission {
			return fmt.Errorf("权限不足，无法操作角色 %d", roleID)
		}
	}

	switch req.Action {
	case "enable":
		return s.roleDao.BatchUpdateStatus(req.RoleIDs, model.RoleStatusEnabled)
	case "disable":
		return s.roleDao.BatchUpdateStatus(req.RoleIDs, model.RoleStatusDisabled)
	case "delete":
		// 检查是否有用户关联
		for _, roleID := range req.RoleIDs {
			userCount, err := s.getUserCountByRole(roleID)
			if err != nil {
				return fmt.Errorf("检查角色 %d 用户关联失败", roleID)
			}
			if userCount > 0 {
				return fmt.Errorf("角色 %d 有用户关联，不能删除", roleID)
			}
		}
		return s.roleDao.BatchDelete(req.RoleIDs)
	default:
		return fmt.Errorf("不支持的操作类型: %s", req.Action)
	}
}

// AssignPermissions 为角色分配权限
func (s *RoleService) AssignPermissions(roleID uint, req *dto.AssignPermissionsRequest, grantedBy uint) error {
	// 检查角色是否存在
	_, err := s.roleDao.GetByID(roleID)
	if err != nil {
		return err
	}

	// 检查权限是否存在
	for _, permissionID := range req.PermissionIDs {
		permissionDao := dao.NewPermissionDao(s.database)
		_, err := permissionDao.GetByID(permissionID)
		if err != nil {
			return fmt.Errorf("权限 %d 不存在", permissionID)
		}
	}

	return s.roleDao.AssignPermissions(roleID, req.PermissionIDs, grantedBy)
}

// GetRolePermissions 获取角色的权限列表
func (s *RoleService) GetRolePermissions(roleID uint) ([]*dto.PermissionResponse, error) {
	// 检查角色是否存在
	_, err := s.roleDao.GetByID(roleID)
	if err != nil {
		return nil, err
	}

	permissions, err := s.roleDao.GetRolePermissions(roleID)
	if err != nil {
		return nil, err
	}

	return dto.ConvertToPermissionResponses(permissions), nil
}

// GetRoleStats 获取角色统计信息
func (s *RoleService) GetRoleStats() (*dto.RoleStatsResponse, error) {
	statusCounts, err := s.roleDao.CountRolesByStatus()
	if err != nil {
		return nil, err
	}

	var total int64
	for _, count := range statusCounts {
		total += count
	}

	// 统计系统角色和自定义角色
	var systemRoles, customRoles int64
	var roles []*model.Role
	if err := s.database.Find(&roles).Error; err == nil {
		for _, role := range roles {
			if role.IsSystem() {
				systemRoles++
			} else {
				customRoles++
			}
		}
	}

	return &dto.RoleStatsResponse{
		TotalRoles:    total,
		EnabledRoles:  statusCounts["enabled"],
		DisabledRoles: statusCounts["disabled"],
		SystemRoles:   systemRoles,
		CustomRoles:   customRoles,
	}, nil
}

// getUserCountByRole 获取角色关联的用户数量
func (s *RoleService) getUserCountByRole(roleID uint) (int64, error) {
	var count int64
	if err := s.database.Model(&model.UserRole{}).
		Where("role_id = ? AND is_active = ? AND deleted_at IS NULL", roleID, true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ValidateRolePermission 验证角色是否有特定权限
func (s *RoleService) ValidateRolePermission(roleID uint, resource, action string) (bool, error) {
	permissions, err := s.roleDao.GetRolePermissions(roleID)
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

// GetUserRoles 获取用户的角色列表
func (s *RoleService) GetUserRoles(userID uint) ([]*dto.RoleResponse, error) {
	roles, err := s.roleDao.GetRolesByUserID(userID)
	if err != nil {
		return nil, err
	}

	return dto.ConvertToRoleResponses(roles), nil
}

// checkRoleOperationPermission 检查用户是否有权限操作目标角色
func (s *RoleService) checkRoleOperationPermission(currentUserID uint, targetRole *model.Role) (bool, error) {
	// 获取当前用户的所有有效角色
	var userRoles []model.UserRole
	if err := s.database.Where("user_id = ? AND is_active = ? AND approval_status = ? AND deleted_at IS NULL",
		currentUserID, true, 1).
		Preload("Role").Find(&userRoles).Error; err != nil {
		return false, err
	}

	if len(userRoles) == 0 {
		return false, nil
	}

	// 查找当前用户的最大数据权限范围（数值越小，权限越大）
	minDataScope := 4 // 默认最小权限（仅本人）
	hasSuperAdmin := false

	for i := range userRoles {
		userRole := &userRoles[i]
		if userRole.Role.ID == 0 || !userRole.Role.IsEnabled() {
			continue
		}

		// 检查是否是超级管理员
		if userRole.Role.Code == "super_admin" {
			hasSuperAdmin = true
			break
		}

		// 获取角色的数据权限范围
		roleDataScope := userRole.DataScope
		if roleDataScope == 0 && userRole.Role.DataScope != nil {
			roleDataScope = *userRole.Role.DataScope
		}

		if roleDataScope > 0 && roleDataScope < minDataScope {
			minDataScope = roleDataScope
		}
	}

	// 超级管理员可以操作所有非系统角色
	if hasSuperAdmin {
		return true, nil
	}

	// 如果用户有全部数据权限（DataScope = 1），可以操作所有自定义角色
	if minDataScope == 1 {
		return true, nil
	}

	// 其他情况下，检查角色层级关系
	// 如果目标角色是用户当前角色的子级角色，则允许操作
	for i := range userRoles {
		userRole := &userRoles[i]
		if userRole.Role.ID == 0 || !userRole.Role.IsEnabled() {
			continue
		}

		// 检查目标角色是否是当前角色的子级
		if targetRole.ParentID != nil && *targetRole.ParentID == userRole.Role.ID {
			return true, nil
		}

		// 如果数据权限是本部门或本部门及下级，还需要检查部门匹配
		// 这里简化处理，如果有 DataScope = 2 或 3 的权限，允许操作同级别或下级角色
		if minDataScope <= 3 {
			// 检查目标角色的层级是否大于等于当前用户角色的层级
			if targetRole.Level != nil && userRole.Role.Level != nil {
				if *targetRole.Level >= *userRole.Role.Level {
					return true, nil
				}
			}
		}
	}

	return false, nil
}
