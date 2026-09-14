package service

import (
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/passhash"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// InstallService 安装服务
type InstallService struct {
	db *gorm.DB
}

// NewInstallService 创建安装服务实例
func NewInstallService(db *gorm.DB) *InstallService {
	return &InstallService{db: db}
}

// CreateInitialData 创建初始数据（部门、角色、权限、管理员）
func (s *InstallService) CreateInitialData(adminInfo dto.AdminAccountInfo) error {
	// 开启事务
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 创建默认部门
		adminDeptID, err := s.createDefaultDepartment(tx)
		if err != nil {
			return fmt.Errorf("创建默认部门失败: %w", err)
		}

		// 2. 创建默认角色
		superAdminRoleID, err := s.createDefaultRole(tx)
		if err != nil {
			return fmt.Errorf("创建默认角色失败: %w", err)
		}

		// 3. 创建默认权限
		if err := s.createDefaultPermissions(tx); err != nil {
			return fmt.Errorf("创建默认权限失败: %w", err)
		}

		// 4. 创建管理员账户
		if err := s.createAdminAccount(tx, adminInfo, adminDeptID, superAdminRoleID); err != nil {
			return fmt.Errorf("创建管理员账户失败: %w", err)
		}

		// 5. 创建系统内置管理后台 OIDC 应用；与初始账号数据同事务提交。
		if _, err := EnsureAdminConsoleApplication(tx, superAdminRoleID); err != nil {
			return fmt.Errorf("创建默认管理后台应用失败: %w", err)
		}

		return nil
	})
}

// createDefaultDepartment 创建默认部门
func (s *InstallService) createDefaultDepartment(tx *gorm.DB) (uint, error) {
	// 创建根部门 - 系统管理
	rootDept := &model.Department{
		Name:        "系统管理",
		Code:        "system",
		ShortName:   "系统",
		Description: "系统管理部门",
		Type:        model.DepartmentTypeCompany,
		Status:      model.DepartmentStatusEnabled,
		Level:       1,
		Path:        "/1",
		Sort:        1,
		DataScope:   1, // 全部数据权限
		IsLeaf:      false,
		HasChildren: true,
		Attributes:  datatypes.JSON("{}"),
		Remark:      "系统默认根部门",
	}

	if err := tx.Create(rootDept).Error; err != nil {
		return 0, err
	}

	// 创建系统管理部门
	adminDept := &model.Department{
		Name:        "系统管理部门",
		Code:        "admin",
		ShortName:   "系统管理",
		Description: "系统管理部门",
		Type:        model.DepartmentTypeDepartment,
		Status:      model.DepartmentStatusEnabled,
		ParentID:    &rootDept.ID,
		Level:       2,
		Path:        fmt.Sprintf("/1/%d", rootDept.ID),
		Sort:        1,
		DataScope:   1, // 全部数据权限
		IsLeaf:      true,
		HasChildren: false,
		Attributes:  datatypes.JSON("{}"),
		Remark:      "系统默认管理员部门",
	}

	if err := tx.Create(adminDept).Error; err != nil {
		return 0, err
	}

	return adminDept.ID, nil
}

// createDefaultRole 创建默认角色
func (s *InstallService) createDefaultRole(tx *gorm.DB) (uint, error) {
	// 创建超级管理员角色
	roleType := model.RoleTypeSystem
	roleStatus := model.RoleStatusEnabled
	level := 1
	dataScope := 1

	superAdminRole := &model.Role{
		Name:        "超级管理员",
		Code:        "super_admin",
		Description: "系统超级管理员，拥有所有权限",
		Type:        &roleType,
		Status:      &roleStatus,
		Level:       &level,
		Path:        "/1",
		DataScope:   &dataScope, // 全部数据权限
		Attributes:  datatypes.JSON("{}"),
		Remark:      "系统默认超级管理员角色",
	}

	if err := tx.Create(superAdminRole).Error; err != nil {
		return 0, err
	}

	return superAdminRole.ID, nil
}

// createDefaultPermissions 创建默认权限
func (s *InstallService) createDefaultPermissions(tx *gorm.DB) error {
	// 创建基础权限
	permissions := []model.Permission{
		// 系统管理权限
		{
			Name:        "系统管理",
			Code:        "system:manage",
			Description: "系统管理权限",
			Type:        model.PermissionTypeMenu,
			Status:      model.PermissionStatusEnabled,
			Level:       1,
			Path:        "/1",
			Sort:        1,
			Resource:    "system",
			Action:      "*",
			ShowInMenu:  true,
			Attributes:  datatypes.JSON("{}"),
			Remark:      "系统管理菜单权限",
		},
		// 用户管理权限
		{
			Name:        "用户管理",
			Code:        "user:manage",
			Description: "用户管理权限",
			Type:        model.PermissionTypeMenu,
			Status:      model.PermissionStatusEnabled,
			Level:       1,
			Path:        "/2",
			Sort:        2,
			Resource:    "user",
			Action:      "*",
			ShowInMenu:  true,
			Attributes:  datatypes.JSON("{}"),
			Remark:      "用户管理菜单权限",
		},
		// 角色管理权限
		{
			Name:        "角色管理",
			Code:        "role:manage",
			Description: "角色管理权限",
			Type:        model.PermissionTypeMenu,
			Status:      model.PermissionStatusEnabled,
			Level:       1,
			Path:        "/3",
			Sort:        3,
			Resource:    "role",
			Action:      "*",
			ShowInMenu:  true,
			Attributes:  datatypes.JSON("{}"),
			Remark:      "角色管理菜单权限",
		},
		// 部门管理权限
		{
			Name:        "部门管理",
			Code:        "department:manage",
			Description: "部门管理权限",
			Type:        model.PermissionTypeMenu,
			Status:      model.PermissionStatusEnabled,
			Level:       1,
			Path:        "/4",
			Sort:        4,
			Resource:    "department",
			Action:      "*",
			ShowInMenu:  true,
			Attributes:  datatypes.JSON("{}"),
			Remark:      "部门管理菜单权限",
		},
		// 权限管理权限
		{
			Name:        "权限管理",
			Code:        "permission:manage",
			Description: "权限管理权限",
			Type:        model.PermissionTypeMenu,
			Status:      model.PermissionStatusEnabled,
			Level:       1,
			Path:        "/5",
			Sort:        5,
			Resource:    "permission",
			Action:      "*",
			ShowInMenu:  true,
			Attributes:  datatypes.JSON("{}"),
			Remark:      "权限管理菜单权限",
		},
	}

	// 批量创建权限
	if err := tx.Create(&permissions).Error; err != nil {
		return err
	}

	return nil
}

// createAdminAccount 创建管理员账户
func (s *InstallService) createAdminAccount(tx *gorm.DB, adminInfo dto.AdminAccountInfo, deptID, roleID uint) error {
	// 生成密码哈希
	salt, passwordHash, err := s.generatePasswordHash(adminInfo.Password)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}

	// 处理可选字段 - 将string转换为*string
	var email, phone *string
	if adminInfo.Email != "" {
		email = &adminInfo.Email
	}
	if adminInfo.Phone != "" {
		phone = &adminInfo.Phone
	}

	// 创建管理员用户，管理员首帐号默认视为已验证
	now := time.Now()
	var adminEmailVerifiedAt, adminPhoneVerifiedAt *time.Time
	if email != nil {
		adminEmailVerifiedAt = &now
	}
	if phone != nil {
		adminPhoneVerifiedAt = &now
	}
	admin := &model.User{
		Username:        adminInfo.Username,
		Email:           email,
		Phone:           phone,
		Nickname:        adminInfo.RealName,
		RealName:        adminInfo.RealName,
		Salt:            salt,
		PasswordHash:    passwordHash,
		Status:          model.UserStatusActive,
		IsActivated:     true,
		EmailVerifiedAt: adminEmailVerifiedAt,
		PhoneVerifiedAt: adminPhoneVerifiedAt,
		LoginCount:      0,
		Remark:          "系统管理员账户",
	}

	if err := tx.Create(admin).Error; err != nil {
		return fmt.Errorf("创建管理员账户失败: %w", err)
	}

	// 建立用户和部门的关联关系
	userDepartment := &model.UserDepartment{
		UserID:       admin.ID,
		DepartmentID: deptID,
		Position:     "系统管理员",
		Type:         model.UserDepartmentTypeLeader,
		IsActive:     true,
		IsPrimary:    true,
		Sort:         0,
		JoinedAt:     &now,
		Remark:       "系统默认管理员部门关联",
	}

	if err := tx.Create(userDepartment).Error; err != nil {
		return fmt.Errorf("建立管理员部门关联失败: %w", err)
	}

	// 为管理员分配超级管理员角色
	userRole := &model.UserRole{
		UserID:         admin.ID,
		RoleID:         roleID,
		AssignedBy:     admin.ID, // 自己分配给自己
		AssignedAt:     time.Now(),
		IsActive:       true,
		IsPrimary:      true,
		ApplyScope:     1, // 全局应用
		ApprovalStatus: 1, // 已批准
		DataScope:      1, // 全部数据权限
	}

	if err := tx.Create(userRole).Error; err != nil {
		return fmt.Errorf("为管理员分配角色失败: %w", err)
	}

	return nil
}

// generatePasswordHash 生成密码哈希。
// 与 UserService 保持一致：统一经 passhash（argon2id）生成，salt 留空。
func (s *InstallService) generatePasswordHash(password string) (salt, hash string, err error) {
	h, err := passhash.Hash(password)
	if err != nil {
		return "", "", fmt.Errorf("生成密码哈希失败: %w", err)
	}
	return "", h, nil
}
