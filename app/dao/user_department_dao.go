package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"

	"gorm.io/gorm"
)

// UserDepartmentDao handles database operations for user department relationships
type UserDepartmentDao struct {
	db *gorm.DB
}

// NewUserDepartmentDao creates a new instance of UserDepartmentDao
func NewUserDepartmentDao(db *gorm.DB) *UserDepartmentDao {
	return &UserDepartmentDao{db: db}
}

// Create creates a new user department relationship
func (d *UserDepartmentDao) Create(userDept *model.UserDepartment) error {
	return d.db.Create(userDept).Error
}

// Update updates a user department relationship
func (d *UserDepartmentDao) Update(userDept *model.UserDepartment) error {
	return d.db.Save(userDept).Error
}

// Delete deletes a user department relationship
func (d *UserDepartmentDao) Delete(userID, departmentID uint) error {
	return d.db.Where("user_id = ? AND department_id = ?", userID, departmentID).
		Delete(&model.UserDepartment{}).Error
}

// FindByUserAndDepartment finds a user department relationship by user ID and department ID
func (d *UserDepartmentDao) FindByUserAndDepartment(userID, departmentID uint) (*model.UserDepartment, error) {
	var userDept model.UserDepartment
	err := d.db.Where("user_id = ? AND department_id = ?", userID, departmentID).
		First(&userDept).Error
	if err != nil {
		return nil, err
	}
	return &userDept, nil
}

// FindByUser finds all department relationships for a user
func (d *UserDepartmentDao) FindByUser(userID uint) ([]model.UserDepartment, error) {
	var userDepts []model.UserDepartment
	err := d.db.Where("user_id = ?", userID).
		Order("is_primary DESC, sort ASC").
		Find(&userDepts).Error
	return userDepts, err
}

// FindByUserWithDepartment finds all department relationships for a user with department info
func (d *UserDepartmentDao) FindByUserWithDepartment(userID uint) ([]model.UserDepartment, error) {
	var userDepts []model.UserDepartment
	err := d.db.Where("user_id = ?", userID).
		Preload("Department").
		Order("is_primary DESC, sort ASC").
		Find(&userDepts).Error
	return userDepts, err
}

// FindByDepartment finds all user relationships for a department
func (d *UserDepartmentDao) FindByDepartment(departmentID uint) ([]model.UserDepartment, error) {
	var userDepts []model.UserDepartment
	err := d.db.Where("department_id = ?", departmentID).
		Order("type ASC, sort ASC").
		Find(&userDepts).Error
	return userDepts, err
}

// FindByDepartmentWithUser finds all user relationships for a department with user info
func (d *UserDepartmentDao) FindByDepartmentWithUser(departmentID uint) ([]model.UserDepartment, error) {
	var userDepts []model.UserDepartment
	err := d.db.Where("department_id = ?", departmentID).
		Preload("User").
		Order("type ASC, sort ASC").
		Find(&userDepts).Error
	return userDepts, err
}

// FindPrimaryDepartmentByUser finds the primary department for a user
func (d *UserDepartmentDao) FindPrimaryDepartmentByUser(userID uint) (*model.UserDepartment, error) {
	var userDept model.UserDepartment
	err := d.db.Where("user_id = ? AND is_primary = ?", userID, true).
		First(&userDept).Error
	if err != nil {
		return nil, err
	}
	return &userDept, nil
}

// FindActiveDepartmentsByUser finds all active departments for a user
func (d *UserDepartmentDao) FindActiveDepartmentsByUser(userID uint) ([]model.UserDepartment, error) {
	var userDepts []model.UserDepartment
	err := d.db.Where("user_id = ? AND is_active = ? AND left_at IS NULL", userID, true).
		Preload("Department").
		Order("is_primary DESC, sort ASC").
		Find(&userDepts).Error
	return userDepts, err
}

// List lists user department relationships based on query parameters
func (d *UserDepartmentDao) List(req *dto.UserDepartmentQueryDto) ([]model.UserDepartment, int64, error) {
	var userDepts []model.UserDepartment
	var total int64

	query := d.db.Model(&model.UserDepartment{})

	// Apply filters
	if req.UserID != nil {
		query = query.Where("user_id = ?", *req.UserID)
	}
	if req.DepartmentID != nil {
		query = query.Where("department_id = ?", *req.DepartmentID)
	}
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}
	if req.IsPrimary != nil {
		query = query.Where("is_primary = ?", *req.IsPrimary)
	}
	if req.Type != nil {
		query = query.Where("type = ?", *req.Type)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination and ordering
	offset := (req.Page - 1) * req.PageSize
	query = query.Offset(offset).Limit(req.PageSize).
		Order("is_primary DESC, sort ASC")

	// Preload associations if needed
	if req.WithDepartment {
		query = query.Preload("Department")
	}
	if req.WithUser {
		query = query.Preload("User")
	}

	err := query.Find(&userDepts).Error
	return userDepts, total, err
}

// SetPrimaryDepartment sets a department as primary for a user
// This will unset other primary departments for the user
func (d *UserDepartmentDao) SetPrimaryDepartment(userID, departmentID uint) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		// Unset all primary departments for the user
		if err := tx.Model(&model.UserDepartment{}).
			Where("user_id = ?", userID).
			Update("is_primary", false).Error; err != nil {
			return err
		}

		// Set the specified department as primary
		return tx.Model(&model.UserDepartment{}).
			Where("user_id = ? AND department_id = ?", userID, departmentID).
			Update("is_primary", true).Error
	})
}

// BatchCreate creates multiple user department relationships
func (d *UserDepartmentDao) BatchCreate(userDepts []model.UserDepartment) error {
	return d.db.Create(&userDepts).Error
}

// BatchDelete deletes multiple user department relationships
func (d *UserDepartmentDao) BatchDelete(userID uint, departmentIDs []uint) error {
	return d.db.Where("user_id = ? AND department_id IN ?", userID, departmentIDs).
		Delete(&model.UserDepartment{}).Error
}

// DeleteByUser deletes all department relationships for a user
func (d *UserDepartmentDao) DeleteByUser(userID uint) error {
	return d.db.Where("user_id = ?", userID).
		Delete(&model.UserDepartment{}).Error
}

// DeleteByDepartment deletes all user relationships for a department
func (d *UserDepartmentDao) DeleteByDepartment(departmentID uint) error {
	return d.db.Where("department_id = ?", departmentID).
		Delete(&model.UserDepartment{}).Error
}

// CountByUser counts the number of departments for a user
func (d *UserDepartmentDao) CountByUser(userID uint) (int64, error) {
	var count int64
	err := d.db.Model(&model.UserDepartment{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&count).Error
	return count, err
}

// CountByDepartment counts the number of users in a department
func (d *UserDepartmentDao) CountByDepartment(departmentID uint) (int64, error) {
	var count int64
	err := d.db.Model(&model.UserDepartment{}).
		Where("department_id = ? AND is_active = ?", departmentID, true).
		Count(&count).Error
	return count, err
}

// ExistsByUserAndDepartment checks if a user department relationship exists
func (d *UserDepartmentDao) ExistsByUserAndDepartment(userID, departmentID uint) (bool, error) {
	var count int64
	err := d.db.Model(&model.UserDepartment{}).
		Where("user_id = ? AND department_id = ?", userID, departmentID).
		Count(&count).Error
	return count > 0, err
}
