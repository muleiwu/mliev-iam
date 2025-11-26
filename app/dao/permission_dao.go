package dao

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// PermissionDao 权限数据访问层
type PermissionDao struct {
	db *gorm.DB
}

// NewPermissionDao 创建权限DAO实例
func NewPermissionDao(db *gorm.DB) *PermissionDao {
	return &PermissionDao{db: db}
}

// Create 创建权限
func (d *PermissionDao) Create(permission *model.Permission) error {
	return d.db.Create(permission).Error
}

// GetByID 根据ID获取权限
func (d *PermissionDao) GetByID(id uint) (*model.Permission, error) {
	var permission model.Permission
	if err := d.db.Preload("Parent").Preload("Children").First(&permission, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("权限不存在")
		}
		return nil, err
	}
	return &permission, nil
}

// GetByCode 根据权限代码获取权限
func (d *PermissionDao) GetByCode(code string) (*model.Permission, error) {
	var permission model.Permission
	if err := d.db.Where("code = ?", code).First(&permission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("权限不存在")
		}
		return nil, err
	}
	return &permission, nil
}

// Update 更新权限
func (d *PermissionDao) Update(permission *model.Permission) error {
	return d.db.Save(permission).Error
}

// UpdateById 根据ID更新权限字段
func (d *PermissionDao) UpdateById(id uint, updates map[string]interface{}) error {
	return d.db.Model(&model.Permission{}).Where("id = ?", id).Updates(updates).Error
}

// Delete 删除权限
func (d *PermissionDao) Delete(id uint) error {
	return d.db.Delete(&model.Permission{}, id).Error
}

// List 获取权限列表
func (d *PermissionDao) List(req *dto.PermissionQueryRequest) ([]*model.Permission, int64, error) {
	var permissions []*model.Permission
	var total int64

	query := d.db.Model(&model.Permission{})

	// 名称模糊查询
	if req.Name != "" {
		query = query.Where("name LIKE ?", "%"+req.Name+"%")
	}

	// 权限代码查询
	if req.Code != "" {
		query = query.Where("code LIKE ?", "%"+req.Code+"%")
	}

	// 权限类型过滤
	if req.Type != nil {
		query = query.Where("type = ?", *req.Type)
	}

	// 权限状态过滤
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}

	// 父权限过滤
	if req.ParentID != nil {
		if *req.ParentID == 0 {
			// 查询根权限
			query = query.Where("parent_id IS NULL")
		} else {
			query = query.Where("parent_id = ?", *req.ParentID)
		}
	}

	// 资源过滤
	if req.Resource != "" {
		query = query.Where("resource LIKE ?", "%"+req.Resource+"%")
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	if req.Page > 0 && req.PageSize > 0 {
		offset := (req.Page - 1) * req.PageSize
		query = query.Offset(offset).Limit(req.PageSize)
	}

	// 排序
	if req.OrderBy != "" {
		query = query.Order(req.OrderBy)
	} else {
		query = query.Order("level ASC, sort ASC, created_at DESC")
	}

	// 查询权限列表（不预加载 Children，返回纯扁平列表）
	if err := query.Find(&permissions).Error; err != nil {
		return nil, 0, err
	}

	return permissions, total, nil
}

// GetPermissionTree 获取权限树形结构
func (d *PermissionDao) GetPermissionTree() ([]*model.Permission, error) {
	// 查询所有权限（不预加载，避免 N+1 问题和深度限制）
	var allPermissions []*model.Permission
	if err := d.db.Order("level ASC, sort ASC").Find(&allPermissions).Error; err != nil {
		return nil, err
	}

	// 构建权限映射表，用于快速查找
	permissionMap := make(map[uint]*model.Permission)
	for _, perm := range allPermissions {
		// 创建副本，避免修改原始数据
		permCopy := *perm
		permCopy.Children = make([]model.Permission, 0)
		permissionMap[perm.ID] = &permCopy
	}

	// 构建树形结构
	var rootPermissions []*model.Permission
	for _, perm := range allPermissions {
		permNode := permissionMap[perm.ID]
		if perm.ParentID == nil {
			// 根节点
			rootPermissions = append(rootPermissions, permNode)
		} else {
			// 子节点，添加到父节点的 Children 中
			if parent, exists := permissionMap[*perm.ParentID]; exists {
				parent.Children = append(parent.Children, *permNode)
			}
		}
	}

	return rootPermissions, nil
}

// GetChildrenIDs 获取权限的所有子权限ID（递归）
func (d *PermissionDao) GetChildrenIDs(parentID uint) ([]uint, error) {
	var childrenIDs []uint

	// 递归查询子权限
	var findChildren func(pid uint) error
	findChildren = func(pid uint) error {
		var children []model.Permission
		if err := d.db.Where("parent_id = ?", pid).Find(&children).Error; err != nil {
			return err
		}

		for _, child := range children {
			childrenIDs = append(childrenIDs, child.ID)
			// 递归查询子权限的子权限
			if err := findChildren(child.ID); err != nil {
				return err
			}
		}
		return nil
	}

	if err := findChildren(parentID); err != nil {
		return nil, err
	}

	return childrenIDs, nil
}

// ExistsName 检查权限名称是否存在
func (d *PermissionDao) ExistsName(name string, excludeID ...uint) (bool, error) {
	query := d.db.Model(&model.Permission{}).Where("name = ?", name)
	if len(excludeID) > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// ExistsCode 检查权限代码是否存在
func (d *PermissionDao) ExistsCode(code string, excludeID ...uint) (bool, error) {
	query := d.db.Model(&model.Permission{}).Where("code = ?", code)
	if len(excludeID) > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// BatchUpdateStatus 批量更新权限状态
func (d *PermissionDao) BatchUpdateStatus(permissionIDs []uint, status model.PermissionStatus) error {
	return d.db.Model(&model.Permission{}).Where("id IN ?", permissionIDs).Update("status", status).Error
}

// BatchDelete 批量删除权限
func (d *PermissionDao) BatchDelete(permissionIDs []uint) error {
	return d.db.Where("id IN ?", permissionIDs).Delete(&model.Permission{}).Error
}

// GetPermissionsByUserID 获取用户的权限列表（通过角色关联）
func (d *PermissionDao) GetPermissionsByUserID(userID uint) ([]*model.Permission, error) {
	var permissions []*model.Permission
	if err := d.db.Table("permissions").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Joins("JOIN user_roles ON role_permissions.role_id = user_roles.role_id").
		Where("user_roles.user_id = ? AND user_roles.is_active = ? AND user_roles.deleted_at IS NULL", userID, true).
		Where("role_permissions.deleted_at IS NULL AND permissions.status = ?", model.PermissionStatusEnabled).
		Group("permissions.id").
		Find(&permissions).Error; err != nil {
		return nil, err
	}
	return permissions, nil
}

// GetPermissionsByRoleID 获取角色的权限列表
func (d *PermissionDao) GetPermissionsByRoleID(roleID uint) ([]*model.Permission, error) {
	var permissions []*model.Permission
	if err := d.db.Table("permissions").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id = ? AND role_permissions.deleted_at IS NULL", roleID).
		Where("permissions.status = ?", model.PermissionStatusEnabled).
		Find(&permissions).Error; err != nil {
		return nil, err
	}
	return permissions, nil
}

// GetMenuPermissions 获取菜单权限列表
func (d *PermissionDao) GetMenuPermissions() ([]*model.Permission, error) {
	var permissions []*model.Permission
	if err := d.db.Where("type = ? AND status = ? AND show_in_menu = ?",
		model.PermissionTypeMenu, model.PermissionStatusEnabled, true).
		Order("level ASC, sort ASC").
		Find(&permissions).Error; err != nil {
		return nil, err
	}
	return permissions, nil
}

// GetAPIPermissions 获取API权限列表
func (d *PermissionDao) GetAPIPermissions() ([]*model.Permission, error) {
	var permissions []*model.Permission
	if err := d.db.Where("type = ? AND status = ?",
		model.PermissionTypeAPI, model.PermissionStatusEnabled).
		Find(&permissions).Error; err != nil {
		return nil, err
	}
	return permissions, nil
}

// CheckPermissionByResource 检查资源权限
func (d *PermissionDao) CheckPermissionByResource(resource, action string) (*model.Permission, error) {
	var permission model.Permission
	if err := d.db.Where("resource = ? AND action = ? AND status = ?",
		resource, action, model.PermissionStatusEnabled).
		Or("resource = ? AND action = ? AND status = ?",
			resource, "*", model.PermissionStatusEnabled).
		First(&permission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("权限不存在")
		}
		return nil, err
	}
	return &permission, nil
}

// CheckPermissionByURL 检查URL权限
func (d *PermissionDao) CheckPermissionByURL(url, method string) (*model.Permission, error) {
	var permission model.Permission
	if err := d.db.Where("url = ? AND method = ? AND status = ?",
		url, method, model.PermissionStatusEnabled).
		Or("url = ? AND method = ? AND status = ?",
			url, "*", model.PermissionStatusEnabled).
		First(&permission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("权限不存在")
		}
		return nil, err
	}
	return &permission, nil
}

// CountPermissionsByType 按类型统计权限数量
func (d *PermissionDao) CountPermissionsByType() (map[string]int64, error) {
	type TypeCount struct {
		Type  int   `json:"type"`
		Count int64 `json:"count"`
	}

	var results []TypeCount
	if err := d.db.Model(&model.Permission{}).
		Select("type, COUNT(*) as count").
		Group("type").
		Find(&results).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, result := range results {
		switch result.Type {
		case int(model.PermissionTypeMenu):
			counts["menu"] = result.Count
		case int(model.PermissionTypeButton):
			counts["button"] = result.Count
		case int(model.PermissionTypeAPI):
			counts["api"] = result.Count
		case int(model.PermissionTypeData):
			counts["data"] = result.Count
		}
	}

	return counts, nil
}

// CountPermissionsByStatus 按状态统计权限数量
func (d *PermissionDao) CountPermissionsByStatus() (map[string]int64, error) {
	type StatusCount struct {
		Status int   `json:"status"`
		Count  int64 `json:"count"`
	}

	var results []StatusCount
	if err := d.db.Model(&model.Permission{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Find(&results).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, result := range results {
		switch result.Status {
		case int(model.PermissionStatusEnabled):
			counts["enabled"] = result.Count
		case int(model.PermissionStatusDisabled):
			counts["disabled"] = result.Count
		}
	}

	return counts, nil
}

// CheckRoleURLPermission 检查角色是否有URL权限
func (d *PermissionDao) CheckRoleURLPermission(roleIDs []uint, path, method string) (bool, error) {
	var count int64

	// 查询是否存在匹配的权限
	query := d.db.Model(&model.Permission{}).
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id IN ?", roleIDs).
		Where("permissions.status = ?", model.PermissionStatusEnabled).
		Where("permissions.type = ?", model.PermissionTypeAPI)

	// URL匹配 - 支持精确匹配和通配符匹配
	if method != "" {
		query = query.Where("(permissions.url = ? AND permissions.method = ?) OR permissions.url = '*'", path, method)
	} else {
		query = query.Where("permissions.url = ? OR permissions.url = '*'", path)
	}

	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// CheckRolePermission 检查角色是否有特定权限代码
func (d *PermissionDao) CheckRolePermission(roleIDs []uint, permissionCode string) (bool, error) {
	var count int64

	// 查询是否存在匹配的权限
	if err := d.db.Model(&model.Permission{}).
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id IN ?", roleIDs).
		Where("permissions.status = ?", model.PermissionStatusEnabled).
		Where("permissions.code = ?", permissionCode).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetByRoleIDs 根据角色ID列表获取权限列表
func (d *PermissionDao) GetByRoleIDs(roleIDs []uint) ([]*model.Permission, error) {
	var permissions []*model.Permission

	if err := d.db.Table("permissions").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id IN ?", roleIDs).
		Where("permissions.status = ?", model.PermissionStatusEnabled).
		Group("permissions.id").
		Find(&permissions).Error; err != nil {
		return nil, err
	}

	return permissions, nil
}
