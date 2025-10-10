package dao

import (
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// derefInt 处理int指针，提供默认值
func derefInt(ptr *int, defaultVal int) int {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

// UserRoleDao 用户角色关联数据访问层
type UserRoleDao struct {
	db *gorm.DB
}

// NewUserRoleDao 创建用户角色关联DAO实例
func NewUserRoleDao(db *gorm.DB) *UserRoleDao {
	return &UserRoleDao{db: db}
}

// Create 创建用户角色关联
func (d *UserRoleDao) Create(userRole *model.UserRole) error {
	return d.db.Create(userRole).Error
}

// GetByUserIDAndRoleID 根据用户ID和角色ID获取关联
func (d *UserRoleDao) GetByUserIDAndRoleID(userID, roleID uint) (*model.UserRole, error) {
	var userRole model.UserRole
	if err := d.db.Where("user_id = ? AND role_id = ?", userID, roleID).
		Preload("User").Preload("Role").First(&userRole).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("用户角色关联不存在")
		}
		return nil, err
	}
	return &userRole, nil
}

// GetByUserID 根据用户ID获取所有角色关联
func (d *UserRoleDao) GetByUserID(userID uint) ([]*model.UserRole, error) {
	var userRoles []*model.UserRole
	if err := d.db.Where("user_id = ?", userID).
		Preload("Role").
		Find(&userRoles).Error; err != nil {
		return nil, err
	}
	return userRoles, nil
}

// Update 更新用户角色关联
func (d *UserRoleDao) Update(userRole *model.UserRole) error {
	return d.db.Save(userRole).Error
}

// Delete 删除用户角色关联
func (d *UserRoleDao) Delete(userID, roleID uint) error {
	return d.db.Where("user_id = ? AND role_id = ?", userID, roleID).Delete(&model.UserRole{}).Error
}

// List 获取用户角色关联列表
func (d *UserRoleDao) List(req *dto.UserRoleQueryRequest) ([]*model.UserRole, int64, error) {
	var userRoles []*model.UserRole
	var total int64

	query := d.db.Model(&model.UserRole{})

	// 用户ID过滤
	if req.UserID != nil {
		query = query.Where("user_id = ?", *req.UserID)
	}

	// 角色ID过滤
	if req.RoleID != nil {
		query = query.Where("role_id = ?", *req.RoleID)
	}

	// 激活状态过滤
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}

	// 主要角色过滤
	if req.IsPrimary != nil {
		query = query.Where("is_primary = ?", *req.IsPrimary)
	}

	// 审批状态过滤
	if req.ApprovalStatus != nil {
		query = query.Where("approval_status = ?", *req.ApprovalStatus)
	}

	// 部门ID过滤
	if req.DepartmentID != nil {
		query = query.Where("department_id = ?", *req.DepartmentID)
	}

	// 分配人过滤
	if req.AssignedBy != nil {
		query = query.Where("assigned_by = ?", *req.AssignedBy)
	}

	// 过期时间过滤
	if req.ShowExpired != nil && !*req.ShowExpired {
		query = query.Where("expires_at IS NULL OR expires_at > ?", time.Now())
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
		query = query.Order("is_primary DESC, assigned_at DESC")
	}

	// 预加载关联数据
	if err := query.Preload("User").Preload("Role").Preload("AssignUser").
		Preload("Approver").Preload("Department").Find(&userRoles).Error; err != nil {
		return nil, 0, err
	}

	return userRoles, total, nil
}

// GetUserRoles 获取用户的角色列表
func (d *UserRoleDao) GetUserRoles(userID uint) ([]*model.UserRole, error) {
	var userRoles []*model.UserRole
	if err := d.db.Where("user_id = ? AND is_active = ? AND deleted_at IS NULL", userID, true).
		Preload("Role").Find(&userRoles).Error; err != nil {
		return nil, err
	}
	return userRoles, nil
}

// GetRoleUsers 获取角色的用户列表
func (d *UserRoleDao) GetRoleUsers(roleID uint) ([]*model.UserRole, error) {
	var userRoles []*model.UserRole
	if err := d.db.Where("role_id = ? AND is_active = ? AND deleted_at IS NULL", roleID, true).
		Preload("User").Find(&userRoles).Error; err != nil {
		return nil, err
	}
	return userRoles, nil
}

// AssignRole 为用户分配角色
func (d *UserRoleDao) AssignRole(userID, roleID, assignedBy uint, req *dto.AssignUserRoleRequest) error {
	// 检查是否已存在关联
	var count int64
	if err := d.db.Model(&model.UserRole{}).
		Where("user_id = ? AND role_id = ? AND deleted_at IS NULL", userID, roleID).
		Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return fmt.Errorf("用户已分配该角色")
	}

	userRole := &model.UserRole{
		UserID:         userID,
		RoleID:         roleID,
		AssignedBy:     assignedBy,
		AssignedAt:     time.Now(),
		IsActive:       true,
		IsPrimary:      req.IsPrimary,
		DepartmentID:   req.DepartmentID,
		OrganizationID: req.OrganizationID,
		ApplyScope:     derefInt(req.ApplyScope, 1), // 使用辅助函数处理指针
		DataScope:      derefInt(req.DataScope, 1),  // 使用辅助函数处理指针
		ApprovalStatus: 1,                           // 默认已批准
		Constraints:    req.Constraints,
	}

	// 设置过期时间
	if req.ExpiresAt != nil {
		userRole.ExpiresAt = req.ExpiresAt
	}

	return d.db.Create(userRole).Error
}

// RemoveRole 移除用户角色
func (d *UserRoleDao) RemoveRole(userID, roleID uint) error {
	return d.db.Where("user_id = ? AND role_id = ?", userID, roleID).Delete(&model.UserRole{}).Error
}

// BatchAssignRoles 批量分配角色给用户
func (d *UserRoleDao) BatchAssignRoles(userID uint, roleIDs []uint, assignedBy uint) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		for _, roleID := range roleIDs {
			// 检查是否已存在
			var count int64
			if err := tx.Model(&model.UserRole{}).
				Where("user_id = ? AND role_id = ? AND deleted_at IS NULL", userID, roleID).
				Count(&count).Error; err != nil {
				return err
			}

			if count == 0 {
				userRole := &model.UserRole{
					UserID:         userID,
					RoleID:         roleID,
					AssignedBy:     assignedBy,
					AssignedAt:     time.Now(),
					IsActive:       true,
					ApprovalStatus: 1,
					DataScope:      1,
					ApplyScope:     1,
				}
				if err := tx.Create(userRole).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// BatchRemoveRoles 批量移除用户角色
func (d *UserRoleDao) BatchRemoveRoles(userID uint, roleIDs []uint) error {
	return d.db.Where("user_id = ? AND role_id IN ?", userID, roleIDs).Delete(&model.UserRole{}).Error
}

// BatchAssignUsers 批量分配用户给角色
func (d *UserRoleDao) BatchAssignUsers(roleID uint, userIDs []uint, assignedBy uint) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		for _, userID := range userIDs {
			// 检查是否已存在
			var count int64
			if err := tx.Model(&model.UserRole{}).
				Where("user_id = ? AND role_id = ? AND deleted_at IS NULL", userID, roleID).
				Count(&count).Error; err != nil {
				return err
			}

			if count == 0 {
				userRole := &model.UserRole{
					UserID:         userID,
					RoleID:         roleID,
					AssignedBy:     assignedBy,
					AssignedAt:     time.Now(),
					IsActive:       true,
					ApprovalStatus: 1,
					DataScope:      1,
					ApplyScope:     1,
				}
				if err := tx.Create(userRole).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// UpdateStatus 更新用户角色状态
func (d *UserRoleDao) UpdateStatus(userID, roleID uint, isActive bool) error {
	return d.db.Model(&model.UserRole{}).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Update("is_active", isActive).Error
}

// SetPrimaryRole 设置主要角色
func (d *UserRoleDao) SetPrimaryRole(userID, roleID uint) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		// 先取消所有主要角色
		if err := tx.Model(&model.UserRole{}).
			Where("user_id = ?", userID).
			Update("is_primary", false).Error; err != nil {
			return err
		}

		// 设置新的主要角色
		return tx.Model(&model.UserRole{}).
			Where("user_id = ? AND role_id = ?", userID, roleID).
			Update("is_primary", true).Error
	})
}

// GetPrimaryRole 获取用户的主要角色
func (d *UserRoleDao) GetPrimaryRole(userID uint) (*model.UserRole, error) {
	var userRole model.UserRole
	if err := d.db.Where("user_id = ? AND is_primary = ? AND is_active = ? AND deleted_at IS NULL",
		userID, true, true).Preload("Role").First(&userRole).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("用户没有主要角色")
		}
		return nil, err
	}
	return &userRole, nil
}

// UpdateExpiry 更新角色过期时间
func (d *UserRoleDao) UpdateExpiry(userID, roleID uint, expiresAt *time.Time) error {
	return d.db.Model(&model.UserRole{}).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Update("expires_at", expiresAt).Error
}

// GetExpiredUserRoles 获取过期的用户角色
func (d *UserRoleDao) GetExpiredUserRoles() ([]*model.UserRole, error) {
	var userRoles []*model.UserRole
	if err := d.db.Where("expires_at IS NOT NULL AND expires_at <= ? AND is_active = ?",
		time.Now(), true).Preload("User").Preload("Role").Find(&userRoles).Error; err != nil {
		return nil, err
	}
	return userRoles, nil
}

// ApproveUserRole 审批用户角色
func (d *UserRoleDao) ApproveUserRole(userID, roleID, approverID uint, approved bool, remark string) error {
	status := 1 // 已批准
	if !approved {
		status = 2 // 已拒绝
	}

	updates := map[string]interface{}{
		"approval_status": status,
		"approved_by":     approverID,
		"approved_at":     time.Now(),
		"approval_remark": remark,
	}

	if !approved {
		updates["is_active"] = false
	}

	return d.db.Model(&model.UserRole{}).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Updates(updates).Error
}

// GetPendingApprovalUserRoles 获取待审批的用户角色
func (d *UserRoleDao) GetPendingApprovalUserRoles() ([]*model.UserRole, error) {
	var userRoles []*model.UserRole
	if err := d.db.Where("approval_status = ?", 0).
		Preload("User").Preload("Role").Preload("AssignUser").Find(&userRoles).Error; err != nil {
		return nil, err
	}
	return userRoles, nil
}

// CountUsersByRole 统计角色的用户数量
func (d *UserRoleDao) CountUsersByRole(roleID uint) (int64, error) {
	var count int64
	if err := d.db.Model(&model.UserRole{}).
		Where("role_id = ? AND is_active = ? AND deleted_at IS NULL", roleID, true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountRolesByUser 统计用户的角色数量
func (d *UserRoleDao) CountRolesByUser(userID uint) (int64, error) {
	var count int64
	if err := d.db.Model(&model.UserRole{}).
		Where("user_id = ? AND is_active = ? AND deleted_at IS NULL", userID, true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetUserRoleStats 获取用户角色统计信息
func (d *UserRoleDao) GetUserRoleStats() (map[string]int64, error) {
	type StatusCount struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}

	var results []StatusCount
	var stats = make(map[string]int64)

	// 统计总数
	var total int64
	if err := d.db.Model(&model.UserRole{}).Count(&total).Error; err != nil {
		return nil, err
	}
	stats["total"] = total

	// 统计激活状态
	if err := d.db.Model(&model.UserRole{}).
		Select("CASE WHEN is_active = 1 THEN 'active' ELSE 'inactive' END as status, COUNT(*) as count").
		Group("is_active").Find(&results).Error; err != nil {
		return nil, err
	}

	for _, result := range results {
		stats[result.Status] = result.Count
	}

	// 统计审批状态
	if err := d.db.Model(&model.UserRole{}).
		Select("CASE WHEN approval_status = 0 THEN 'pending' WHEN approval_status = 1 THEN 'approved' ELSE 'rejected' END as status, COUNT(*) as count").
		Group("approval_status").Find(&results).Error; err != nil {
		return nil, err
	}

	for _, result := range results {
		stats[result.Status] = result.Count
	}

	// 统计过期数量
	var expiredCount int64
	if err := d.db.Model(&model.UserRole{}).
		Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
		Count(&expiredCount).Error; err != nil {
		return nil, err
	}
	stats["expired"] = expiredCount

	return stats, nil
}
