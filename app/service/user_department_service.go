package service

import (
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

var (
	ErrUserDepartmentConflict           = errors.New("用户已关联该部门")
	ErrUserDepartmentDepartmentDisabled = errors.New("部门未启用")
	ErrUserDepartmentDepartmentNotFound = errors.New("部门不存在")
	ErrUserDepartmentNotFound           = errors.New("用户部门关联不存在")
	ErrUserDepartmentUserNotFound       = errors.New("用户不存在")
)

// UserDepartmentService 管理用户与组织部门之间的归属关系。
type UserDepartmentService struct {
	database      *gorm.DB
	departmentDao *dao.DepartmentDao
	userDao       *dao.UserDao
}

// NewUserDepartmentService 创建用户部门关系服务。
func NewUserDepartmentService(database *gorm.DB) *UserDepartmentService {
	return &UserDepartmentService{
		database:      database,
		departmentDao: dao.NewDepartmentDao(database),
		userDao:       dao.NewUserDao(database),
	}
}

// Create 创建或恢复用户部门关系。
func (s *UserDepartmentService) Create(req *dto.UserDepartmentCreateDto) (*dto.UserDepartmentResponseDto, error) {
	if _, err := s.userDao.GetByID(req.UserID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserDepartmentUserNotFound, err)
	}
	department, err := s.departmentDao.GetByID(req.DepartmentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserDepartmentDepartmentNotFound, err)
	}
	if !department.IsEnabled() {
		return nil, ErrUserDepartmentDepartmentDisabled
	}

	var response *dto.UserDepartmentResponseDto
	err = s.database.Transaction(func(tx *gorm.DB) error {
		var activeCount int64
		if err := tx.Model(&model.UserDepartment{}).
			Where("user_id = ? AND is_active = ?", req.UserID, true).
			Count(&activeCount).Error; err != nil {
			return fmt.Errorf("统计用户部门失败: %w", err)
		}
		makePrimary := req.IsPrimary || activeCount == 0

		var existing model.UserDepartment
		findErr := tx.Unscoped().
			Where("user_id = ? AND department_id = ?", req.UserID, req.DepartmentID).
			First(&existing).Error
		if findErr == nil && !existing.DeletedAt.Valid {
			return ErrUserDepartmentConflict
		}
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询用户部门关联失败: %w", findErr)
		}

		if makePrimary {
			if err := tx.Model(&model.UserDepartment{}).
				Where("user_id = ?", req.UserID).
				Update("is_primary", false).Error; err != nil {
				return fmt.Errorf("更新主部门失败: %w", err)
			}
		}

		now := time.Now()
		if findErr == nil {
			updates := map[string]any{
				"position":   req.Position,
				"title":      req.Title,
				"type":       model.UserDepartmentType(req.Type),
				"is_active":  true,
				"is_primary": makePrimary,
				"sort":       req.Sort,
				"joined_at":  &now,
				"left_at":    nil,
				"remark":     req.Remark,
				"deleted_at": nil,
			}
			if err := tx.Unscoped().Model(&model.UserDepartment{}).
				Where("user_id = ? AND department_id = ?", req.UserID, req.DepartmentID).
				Updates(updates).Error; err != nil {
				return fmt.Errorf("恢复用户部门关联失败: %w", err)
			}
		} else {
			relation := &model.UserDepartment{
				UserID:       req.UserID,
				DepartmentID: req.DepartmentID,
				Position:     req.Position,
				Title:        req.Title,
				Type:         model.UserDepartmentType(req.Type),
				IsActive:     true,
				IsPrimary:    makePrimary,
				Sort:         req.Sort,
				JoinedAt:     &now,
				Remark:       req.Remark,
			}
			if err := tx.Create(relation).Error; err != nil {
				return fmt.Errorf("创建用户部门关联失败: %w", err)
			}
		}

		relation, err := getUserDepartmentWithDepartment(tx, req.UserID, req.DepartmentID)
		if err != nil {
			return err
		}
		response = dto.ToUserDepartmentResponseDto(relation)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Delete 删除用户部门关系；删除主部门后自动选择剩余关系作为主部门。
func (s *UserDepartmentService) Delete(userID, departmentID uint) error {
	return s.database.Transaction(func(tx *gorm.DB) error {
		var relation model.UserDepartment
		if err := tx.Where("user_id = ? AND department_id = ?", userID, departmentID).
			First(&relation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserDepartmentNotFound
			}
			return fmt.Errorf("查询用户部门关联失败: %w", err)
		}

		wasPrimary := relation.IsPrimary
		now := time.Now()
		if err := tx.Model(&relation).Updates(map[string]any{
			"is_active":  false,
			"is_primary": false,
			"left_at":    &now,
		}).Error; err != nil {
			return fmt.Errorf("更新用户部门关联失败: %w", err)
		}
		if err := tx.Delete(&relation).Error; err != nil {
			return fmt.Errorf("删除用户部门关联失败: %w", err)
		}

		if !wasPrimary {
			return nil
		}
		var replacement model.UserDepartment
		err := tx.Where("user_id = ? AND is_active = ?", userID, true).
			Order("sort ASC, created_at ASC, department_id ASC").
			First(&replacement).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("查询候选主部门失败: %w", err)
		}
		if err := tx.Model(&replacement).Update("is_primary", true).Error; err != nil {
			return fmt.Errorf("设置候选主部门失败: %w", err)
		}
		return nil
	})
}

// SetPrimary 将指定的有效用户部门关系设为主部门。
func (s *UserDepartmentService) SetPrimary(userID, departmentID uint) (*dto.UserDepartmentResponseDto, error) {
	var response *dto.UserDepartmentResponseDto
	err := s.database.Transaction(func(tx *gorm.DB) error {
		var relation model.UserDepartment
		if err := tx.Where("user_id = ? AND department_id = ? AND is_active = ?", userID, departmentID, true).
			First(&relation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserDepartmentNotFound
			}
			return fmt.Errorf("查询用户部门关联失败: %w", err)
		}

		if err := tx.Model(&model.UserDepartment{}).
			Where("user_id = ?", userID).
			Update("is_primary", false).Error; err != nil {
			return fmt.Errorf("取消原主部门失败: %w", err)
		}
		if err := tx.Model(&relation).Update("is_primary", true).Error; err != nil {
			return fmt.Errorf("设置主部门失败: %w", err)
		}

		updated, err := getUserDepartmentWithDepartment(tx, userID, departmentID)
		if err != nil {
			return err
		}
		response = dto.ToUserDepartmentResponseDto(updated)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func getUserDepartmentWithDepartment(db *gorm.DB, userID, departmentID uint) (*model.UserDepartment, error) {
	var relation model.UserDepartment
	if err := db.Preload("Department").
		Where("user_id = ? AND department_id = ?", userID, departmentID).
		First(&relation).Error; err != nil {
		return nil, fmt.Errorf("读取用户部门关联失败: %w", err)
	}
	return &relation, nil
}
