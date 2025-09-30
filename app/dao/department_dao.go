package dao

import (
	"errors"
	"fmt"

	"cnb.cool/mliev/examples/go-web/app/dto"
	"cnb.cool/mliev/examples/go-web/app/model"
	"gorm.io/gorm"
)

// DepartmentDao 部门数据访问层
type DepartmentDao struct {
	database *gorm.DB
}

// NewDepartmentDao 创建部门DAO实例
func NewDepartmentDao(database *gorm.DB) *DepartmentDao {
	return &DepartmentDao{
		database: database,
	}
}

// Create 创建部门
func (dao *DepartmentDao) Create(department *model.Department) error {
	if err := dao.database.Create(department).Error; err != nil {
		return fmt.Errorf("创建部门失败: %w", err)
	}
	return nil
}

// GetByID 根据ID获取部门
func (dao *DepartmentDao) GetByID(id uint) (*model.Department, error) {
	var department model.Department
	if err := dao.database.Preload("Manager").Preload("Parent").First(&department, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("部门不存在")
		}
		return nil, fmt.Errorf("查询部门失败: %w", err)
	}
	return &department, nil
}

// GetByCode 根据编码获取部门
func (dao *DepartmentDao) GetByCode(code string) (*model.Department, error) {
	var department model.Department
	if err := dao.database.Preload("Manager").Preload("Parent").Where("code = ?", code).First(&department).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("部门不存在")
		}
		return nil, fmt.Errorf("查询部门失败: %w", err)
	}
	return &department, nil
}

// GetByName 根据名称获取部门
func (dao *DepartmentDao) GetByName(name string) (*model.Department, error) {
	var department model.Department
	if err := dao.database.Preload("Manager").Preload("Parent").Where("name = ?", name).First(&department).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("部门不存在")
		}
		return nil, fmt.Errorf("查询部门失败: %w", err)
	}
	return &department, nil
}

// Update 更新部门
func (dao *DepartmentDao) Update(department *model.Department) error {
	if err := dao.database.Save(department).Error; err != nil {
		return fmt.Errorf("更新部门失败: %w", err)
	}
	return nil
}

// UpdateById 根据ID更新部门指定字段
func (dao *DepartmentDao) UpdateById(id uint, updates map[string]interface{}) error {
	if err := dao.database.Model(&model.Department{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新部门失败: %w", err)
	}
	return nil
}

// Delete 软删除部门
func (dao *DepartmentDao) Delete(id uint) error {
	// 检查是否有子部门
	var childCount int64
	if err := dao.database.Model(&model.Department{}).Where("parent_id = ?", id).Count(&childCount).Error; err != nil {
		return fmt.Errorf("检查子部门失败: %w", err)
	}
	if childCount > 0 {
		return fmt.Errorf("存在子部门，无法删除")
	}

	// 检查是否有用户关联
	var userCount int64
	if err := dao.database.Model(&model.User{}).Where("department_id = ?", id).Count(&userCount).Error; err != nil {
		return fmt.Errorf("检查关联用户失败: %w", err)
	}
	if userCount > 0 {
		return fmt.Errorf("存在关联用户，无法删除")
	}

	if err := dao.database.Delete(&model.Department{}, id).Error; err != nil {
		return fmt.Errorf("删除部门失败: %w", err)
	}
	return nil
}

// BatchDelete 批量软删除部门
func (dao *DepartmentDao) BatchDelete(ids []uint) error {
	if len(ids) == 0 {
		return fmt.Errorf("部门ID列表不能为空")
	}

	// 检查每个部门是否可以删除
	for _, id := range ids {
		// 检查是否有子部门
		var childCount int64
		if err := dao.database.Model(&model.Department{}).Where("parent_id = ?", id).Count(&childCount).Error; err != nil {
			return fmt.Errorf("检查子部门失败: %w", err)
		}
		if childCount > 0 {
			return fmt.Errorf("部门ID %d 存在子部门，无法删除", id)
		}

		// 检查是否有用户关联
		var userCount int64
		if err := dao.database.Model(&model.User{}).Where("department_id = ?", id).Count(&userCount).Error; err != nil {
			return fmt.Errorf("检查关联用户失败: %w", err)
		}
		if userCount > 0 {
			return fmt.Errorf("部门ID %d 存在关联用户，无法删除", id)
		}
	}

	if err := dao.database.Delete(&model.Department{}, ids).Error; err != nil {
		return fmt.Errorf("批量删除部门失败: %w", err)
	}
	return nil
}

// List 分页查询部门列表
func (dao *DepartmentDao) List(req *dto.DepartmentQueryRequest) ([]model.Department, int64, error) {
	var departments []model.Department
	var total int64

	// 构建查询条件
	query := dao.database.Model(&model.Department{}).Preload("Manager").Preload("Parent")

	// 添加查询条件
	if req.Name != "" {
		query = query.Where("name LIKE ?", "%"+req.Name+"%")
	}
	if req.Code != "" {
		query = query.Where("code LIKE ?", "%"+req.Code+"%")
	}
	if req.Type != nil {
		query = query.Where("type = ?", *req.Type)
	}
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}
	if req.ParentID != nil {
		query = query.Where("parent_id = ?", *req.ParentID)
	}
	if req.ManagerID != nil {
		query = query.Where("manager_id = ?", *req.ManagerID)
	}
	if req.Keyword != "" {
		keyword := "%" + req.Keyword + "%"
		query = query.Where(
			"name LIKE ? OR code LIKE ? OR short_name LIKE ? OR description LIKE ?",
			keyword, keyword, keyword, keyword,
		)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询部门总数失败: %w", err)
	}

	// 如果需要树形结构，不进行分页
	if req.IncludeTree {
		if err := query.Order("parent_id ASC, sort ASC, created_at DESC").Find(&departments).Error; err != nil {
			return nil, 0, fmt.Errorf("查询部门列表失败: %w", err)
		}
		return departments, total, nil
	}

	// 设置默认分页参数
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("sort ASC, created_at DESC").Find(&departments).Error; err != nil {
		return nil, 0, fmt.Errorf("查询部门列表失败: %w", err)
	}

	return departments, total, nil
}

// GetTree 获取部门树形结构
func (dao *DepartmentDao) GetTree(parentID *uint) ([]model.Department, error) {
	var departments []model.Department
	query := dao.database.Model(&model.Department{}).
		Preload("Manager").
		Preload("Children", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort ASC")
		}).
		Order("sort ASC")

	if parentID == nil {
		// 获取根节点
		query = query.Where("parent_id IS NULL")
	} else {
		// 获取指定父节点的子节点
		query = query.Where("parent_id = ?", *parentID)
	}

	if err := query.Find(&departments).Error; err != nil {
		return nil, fmt.Errorf("查询部门树形结构失败: %w", err)
	}

	return departments, nil
}

// GetChildren 获取指定部门的直接子部门
func (dao *DepartmentDao) GetChildren(parentID uint) ([]model.Department, error) {
	var departments []model.Department
	if err := dao.database.Preload("Manager").Where("parent_id = ?", parentID).Order("sort ASC, created_at DESC").Find(&departments).Error; err != nil {
		return nil, fmt.Errorf("查询子部门失败: %w", err)
	}
	return departments, nil
}

// GetDescendants 获取指定部门的所有后代部门
func (dao *DepartmentDao) GetDescendants(parentID uint) ([]model.Department, error) {
	// 先获取父部门的路径
	parent, err := dao.GetByID(parentID)
	if err != nil {
		return nil, err
	}

	var departments []model.Department
	if err := dao.database.Preload("Manager").Where("path LIKE ?", parent.Path+"/%").Order("level ASC, sort ASC").Find(&departments).Error; err != nil {
		return nil, fmt.Errorf("查询后代部门失败: %w", err)
	}
	return departments, nil
}

// GetRootDepartments 获取根部门列表
func (dao *DepartmentDao) GetRootDepartments() ([]model.Department, error) {
	var departments []model.Department
	if err := dao.database.Preload("Manager").Where("parent_id IS NULL").Order("sort ASC, created_at DESC").Find(&departments).Error; err != nil {
		return nil, fmt.Errorf("查询根部门失败: %w", err)
	}
	return departments, nil
}

// ExistsCode 检查编码是否存在
func (dao *DepartmentDao) ExistsCode(code string, excludeID ...uint) (bool, error) {
	var count int64
	query := dao.database.Model(&model.Department{}).Where("code = ?", code)

	// 排除指定ID（用于更新时检查）
	if len(excludeID) > 0 && excludeID[0] > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("检查部门编码是否存在失败: %w", err)
	}
	return count > 0, nil
}

// ExistsName 检查名称是否存在
func (dao *DepartmentDao) ExistsName(name string, parentID *uint, excludeID ...uint) (bool, error) {
	var count int64
	query := dao.database.Model(&model.Department{}).Where("name = ?", name)

	// 同级部门名称不能重复
	if parentID != nil {
		query = query.Where("parent_id = ?", *parentID)
	} else {
		query = query.Where("parent_id IS NULL")
	}

	// 排除指定ID（用于更新时检查）
	if len(excludeID) > 0 && excludeID[0] > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("检查部门名称是否存在失败: %w", err)
	}
	return count > 0, nil
}

// UpdateStatus 更新部门状态
func (dao *DepartmentDao) UpdateStatus(id uint, status model.DepartmentStatus) error {
	updates := map[string]interface{}{
		"status": status,
	}

	if err := dao.database.Model(&model.Department{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新部门状态失败: %w", err)
	}
	return nil
}

// BatchUpdateStatus 批量更新部门状态
func (dao *DepartmentDao) BatchUpdateStatus(ids []uint, status model.DepartmentStatus) error {
	if len(ids) == 0 {
		return fmt.Errorf("部门ID列表不能为空")
	}

	updates := map[string]interface{}{
		"status": status,
	}

	if err := dao.database.Model(&model.Department{}).Where("id IN ?", ids).Updates(updates).Error; err != nil {
		return fmt.Errorf("批量更新部门状态失败: %w", err)
	}
	return nil
}

// MoveDepartment 移动部门
func (dao *DepartmentDao) MoveDepartment(id uint, targetParentID *uint, sort int) error {
	// 获取要移动的部门
	department, err := dao.GetByID(id)
	if err != nil {
		return err
	}

	// 检查不能移动到自己或自己的子部门下
	if targetParentID != nil {
		targetParent, err := dao.GetByID(*targetParentID)
		if err != nil {
			return fmt.Errorf("目标父部门不存在")
		}

		// 检查目标父部门是否是当前部门的后代
		if targetParent.Path != "" && department.Path != "" {
			if len(targetParent.Path) > len(department.Path) &&
				targetParent.Path[:len(department.Path)] == department.Path {
				return fmt.Errorf("不能移动到子部门下")
			}
		}

		// 检查名称是否冲突
		exists, err := dao.ExistsName(department.Name, targetParentID, id)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("目标位置已存在同名部门")
		}
	}

	// 开始事务
	return dao.database.Transaction(func(tx *gorm.DB) error {
		// 更新部门信息
		oldParentID := department.ParentID
		department.ParentID = targetParentID
		department.Sort = sort

		// 重新计算层级和路径
		if targetParentID != nil {
			var targetParent model.Department
			if err := tx.First(&targetParent, *targetParentID).Error; err != nil {
				return fmt.Errorf("获取目标父部门失败: %w", err)
			}
			department.Level = targetParent.Level + 1
			department.Path = fmt.Sprintf("%s/%d", targetParent.Path, department.ID)
		} else {
			department.Level = 1
			department.Path = fmt.Sprintf("/%d", department.ID)
		}

		if err := tx.Save(department).Error; err != nil {
			return fmt.Errorf("更新部门信息失败: %w", err)
		}

		// 更新所有子部门的层级和路径
		if err := dao.updateDescendantsPath(tx, department); err != nil {
			return err
		}

		// 更新原父部门的hasChildren标志
		if oldParentID != nil {
			var childCount int64
			if err := tx.Model(&model.Department{}).Where("parent_id = ?", *oldParentID).Count(&childCount).Error; err != nil {
				return fmt.Errorf("检查原父部门子部门数量失败: %w", err)
			}

			hasChildren := childCount > 0
			isLeaf := !hasChildren
			if err := tx.Model(&model.Department{}).Where("id = ?", *oldParentID).Updates(map[string]interface{}{
				"has_children": hasChildren,
				"is_leaf":      isLeaf,
			}).Error; err != nil {
				return fmt.Errorf("更新原父部门标志失败: %w", err)
			}
		}

		// 更新新父部门的hasChildren标志
		if targetParentID != nil {
			if err := tx.Model(&model.Department{}).Where("id = ?", *targetParentID).Updates(map[string]interface{}{
				"has_children": true,
				"is_leaf":      false,
			}).Error; err != nil {
				return fmt.Errorf("更新新父部门标志失败: %w", err)
			}
		}

		return nil
	})
}

// updateDescendantsPath 更新所有后代部门的路径和层级
func (dao *DepartmentDao) updateDescendantsPath(tx *gorm.DB, parent *model.Department) error {
	var children []model.Department
	if err := tx.Where("parent_id = ?", parent.ID).Find(&children).Error; err != nil {
		return fmt.Errorf("获取子部门失败: %w", err)
	}

	for _, child := range children {
		child.Level = parent.Level + 1
		child.Path = fmt.Sprintf("%s/%d", parent.Path, child.ID)

		if err := tx.Save(&child).Error; err != nil {
			return fmt.Errorf("更新子部门路径失败: %w", err)
		}

		// 递归更新子部门的后代
		if err := dao.updateDescendantsPath(tx, &child); err != nil {
			return err
		}
	}

	return nil
}

// GetByManager 根据负责人ID获取部门
func (dao *DepartmentDao) GetByManager(managerID uint) ([]model.Department, error) {
	var departments []model.Department
	if err := dao.database.Preload("Manager").Where("manager_id = ?", managerID).Find(&departments).Error; err != nil {
		return nil, fmt.Errorf("查询负责人部门失败: %w", err)
	}
	return departments, nil
}

// GetActiveDepartmentsCount 获取启用部门数量
func (dao *DepartmentDao) GetActiveDepartmentsCount() (int64, error) {
	var count int64
	if err := dao.database.Model(&model.Department{}).
		Where("status = ?", model.DepartmentStatusEnabled).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("查询启用部门数量失败: %w", err)
	}
	return count, nil
}
