package dao

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// RoleDao 角色数据访问层
type RoleDao struct {
	db *gorm.DB
}

// NewRoleDao 创建角色DAO实例
func NewRoleDao(db *gorm.DB) *RoleDao {
	return &RoleDao{db: db}
}

// Create 创建角色
func (d *RoleDao) Create(role *model.Role) error {
	return d.db.Create(role).Error
}

// GetByID 根据ID获取角色
func (d *RoleDao) GetByID(id uint) (*model.Role, error) {
	var role model.Role
	if err := d.db.Preload("Parent").Preload("Children").Preload("Permissions").First(&role, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("角色不存在")
		}
		return nil, err
	}
	return &role, nil
}

// GetByIDs 根据ID列表获取角色列表
func (d *RoleDao) GetByIDs(ids []uint) ([]model.Role, error) {
	var roles []model.Role
	if err := d.db.Where("id IN ?", ids).Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// GetByCode 根据角色代码获取角色
func (d *RoleDao) GetByCode(code string) (*model.Role, error) {
	var role model.Role
	if err := d.db.Where("code = ?", code).First(&role).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("角色不存在")
		}
		return nil, err
	}
	return &role, nil
}

// Update 更新角色
func (d *RoleDao) Update(role *model.Role) error {
	return d.db.Save(role).Error
}

// UpdateById 根据ID更新角色字段
func (d *RoleDao) UpdateById(id uint, updates map[string]interface{}) error {
	return d.db.Model(&model.Role{}).Where("id = ?", id).Updates(updates).Error
}

// Delete 删除角色
func (d *RoleDao) Delete(id uint) error {
	return d.db.Delete(&model.Role{}, id).Error
}

// List 获取角色列表
func (d *RoleDao) List(req *dto.RoleQueryRequest) ([]*model.Role, int64, error) {
	var roles []*model.Role
	var total int64

	query := d.db.Model(&model.Role{})

	// 名称模糊查询
	if req.Name != "" {
		query = query.Where("name LIKE ?", "%"+req.Name+"%")
	}

	// 角色代码查询
	if req.Code != "" {
		query = query.Where("code LIKE ?", "%"+req.Code+"%")
	}

	// 角色类型过滤
	if req.Type != nil {
		query = query.Where("type = ?", *req.Type)
	}

	// 角色状态过滤
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}

	// 父角色过滤
	if req.ParentID != nil {
		if *req.ParentID == 0 {
			// 查询根角色
			query = query.Where("parent_id IS NULL")
		} else {
			query = query.Where("parent_id = ?", *req.ParentID)
		}
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

	// 预加载关联数据
	if err := query.Preload("Parent").Preload("Children").Find(&roles).Error; err != nil {
		return nil, 0, err
	}

	return roles, total, nil
}

// GetRoleTree 获取角色树形结构
func (d *RoleDao) GetRoleTree() ([]*model.Role, error) {
	var roles []*model.Role
	if err := d.db.Where("parent_id IS NULL").
		Preload("Children").
		Preload("Children.Children").
		Order("level ASC, sort ASC").
		Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// GetChildrenIDs 获取角色的所有子角色ID（递归）
func (d *RoleDao) GetChildrenIDs(parentID uint) ([]uint, error) {
	var childrenIDs []uint

	// 递归查询子角色
	var findChildren func(pid uint) error
	findChildren = func(pid uint) error {
		var children []model.Role
		if err := d.db.Where("parent_id = ?", pid).Find(&children).Error; err != nil {
			return err
		}

		for _, child := range children {
			childrenIDs = append(childrenIDs, child.ID)
			// 递归查询子角色的子角色
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

// ExistsName 检查角色名称是否存在
func (d *RoleDao) ExistsName(name string, excludeID ...uint) (bool, error) {
	query := d.db.Model(&model.Role{}).Where("name = ?", name)
	if len(excludeID) > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// ExistsCode 检查角色代码是否存在
func (d *RoleDao) ExistsCode(code string, excludeID ...uint) (bool, error) {
	query := d.db.Model(&model.Role{}).Where("code = ?", code)
	if len(excludeID) > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// BatchUpdateStatus 批量更新角色状态
func (d *RoleDao) BatchUpdateStatus(roleIDs []uint, status model.RoleStatus) error {
	return d.db.Model(&model.Role{}).Where("id IN ?", roleIDs).Update("status", status).Error
}

// BatchDelete 批量删除角色
func (d *RoleDao) BatchDelete(roleIDs []uint) error {
	return d.db.Where("id IN ?", roleIDs).Delete(&model.Role{}).Error
}

// GetRolesByUserID 获取用户的角色列表
func (d *RoleDao) GetRolesByUserID(userID uint) ([]*model.Role, error) {
	var roles []*model.Role
	if err := d.db.Table("roles").
		Joins("JOIN user_roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND user_roles.is_active = ? AND user_roles.deleted_at IS NULL", userID, true).
		Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// AssignPermissions 为角色分配权限
func (d *RoleDao) AssignPermissions(roleID uint, permissionIDs []uint) error {
	// 开启事务
	return d.db.Transaction(func(tx *gorm.DB) error {
		// 先删除现有权限关联
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		// 创建新的权限关联
		for _, permissionID := range permissionIDs {
			rolePermission := &model.RolePermission{
				RoleID:       roleID,
				PermissionID: permissionID,
			}
			if err := tx.Create(rolePermission).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// GetRolePermissions 获取角色的权限列表
func (d *RoleDao) GetRolePermissions(roleID uint) ([]*model.Permission, error) {
	var permissions []*model.Permission
	if err := d.db.Table("permissions").
		Joins("JOIN role_permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id = ? AND role_permissions.deleted_at IS NULL", roleID).
		Find(&permissions).Error; err != nil {
		return nil, err
	}
	return permissions, nil
}

// CountRolesByStatus 按状态统计角色数量
func (d *RoleDao) CountRolesByStatus() (map[string]int64, error) {
	type StatusCount struct {
		Status int   `json:"status"`
		Count  int64 `json:"count"`
	}

	var results []StatusCount
	if err := d.db.Model(&model.Role{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Find(&results).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, result := range results {
		switch result.Status {
		case int(model.RoleStatusEnabled):
			counts["enabled"] = result.Count
		case int(model.RoleStatusDisabled):
			counts["disabled"] = result.Count
		}
	}

	return counts, nil
}
