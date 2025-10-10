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
	role := &model.Role{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		ParentID:    req.ParentID,
		Status:      model.RoleStatusEnabled,
		Attributes:  req.Attributes,
		Remark:      req.Remark,
	}

	// 设置角色类型
	if req.Type != nil {
		role.Type = *req.Type
	} else {
		role.Type = model.RoleTypeCustom
	}

	// 设置数据权限范围
	if req.DataScope != nil {
		role.DataScope = *req.DataScope
	} else {
		role.DataScope = 2 // 默认本部门数据
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
func (s *RoleService) UpdateRole(id uint, req *dto.UpdateRoleRequest) (*dto.RoleResponse, error) {
	// 获取角色
	role, err := s.roleDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 检查是否是系统角色
	if role.IsSystem() {
		return nil, fmt.Errorf("系统角色不允许修改")
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

	// 更新字段
	if req.Description != "" {
		role.Description = req.Description
	}
	if req.DataScope != nil {
		role.DataScope = *req.DataScope
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
func (s *RoleService) DeleteRole(id uint) error {
	// 检查角色是否存在
	role, err := s.roleDao.GetByID(id)
	if err != nil {
		return err
	}

	// 检查是否是系统角色
	if role.IsSystem() {
		return fmt.Errorf("系统角色不允许删除")
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
func (s *RoleService) UpdateRoleStatus(id uint, req *dto.RoleStatusUpdateRequest) error {
	// 检查角色是否存在
	role, err := s.roleDao.GetByID(id)
	if err != nil {
		return err
	}

	// 检查是否是系统角色
	if role.IsSystem() {
		return fmt.Errorf("系统角色不允许修改状态")
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
func (s *RoleService) BatchOperation(req *dto.BatchRoleRequest) error {
	if len(req.RoleIDs) == 0 {
		return fmt.Errorf("角色ID列表不能为空")
	}

	// 检查是否包含系统角色
	for _, roleID := range req.RoleIDs {
		role, err := s.roleDao.GetByID(roleID)
		if err != nil {
			return fmt.Errorf("角色 %d 不存在", roleID)
		}
		if role.IsSystem() {
			return fmt.Errorf("系统角色不允许批量操作")
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
func (s *RoleService) AssignPermissions(roleID uint, req *dto.AssignPermissionsRequest) error {
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

	return s.roleDao.AssignPermissions(roleID, req.PermissionIDs)
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
