package migration

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"gorm.io/datatypes"
)

type Migration struct {
	Helper    interfaces.HelperInterface
	Migration []any
}

func (receiver *Migration) Run() error {

	if len(receiver.Migration) > 0 {
		// Disable foreign key checks during migration
		db := receiver.Helper.GetDatabase()
		db.Exec("SET FOREIGN_KEY_CHECKS = 0;")

		err := db.AutoMigrate(receiver.Migration...)
		if err != nil {
			panic(err)
		}

		// Re-enable foreign key checks after migration
		db.Exec("SET FOREIGN_KEY_CHECKS = 1;")

		receiver.Helper.GetLogger().Info(fmt.Sprintf("[db migration success: %d models migrated]", len(receiver.Migration)))

		// 运行种子数据初始化
		if err := receiver.seedData(); err != nil {
			receiver.Helper.GetLogger().Error(fmt.Sprintf("[seed data error: %v]", err))
			return err
		}

		receiver.Helper.GetLogger().Info("[seed data initialized successfully]")
	}
	return nil
}

// Stop implements ServerInterface
func (receiver *Migration) Stop() error {
	// Migration service doesn't need to stop anything
	return nil
}

// seedData 初始化种子数据
func (receiver *Migration) seedData() error {
	db := receiver.Helper.GetDatabase()

	// 检查系统是否已通过安装接口安装
	installed := receiver.Helper.GetConfig().GetBool("system.installed", false)
	if installed {
		receiver.Helper.GetLogger().Info("[system already installed via install API, skipping seed data]")
		return nil
	}

	// 检查是否已经初始化过
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count > 0 {
		receiver.Helper.GetLogger().Info("[seed data already exists, skipping]")
		return nil
	}

	// 创建默认部门
	if err := receiver.createDefaultDepartment(db); err != nil {
		panic(fmt.Errorf("创建默认部门失败: %w", err))
	}

	// 创建默认角色
	_, err := receiver.createDefaultRoles(db)
	if err != nil {
		panic(fmt.Errorf("创建默认角色和权限失败: %w", err))
	}

	// 创建默认权限
	if err := receiver.createDefaultPermissions(db); err != nil {
		panic(fmt.Errorf("创建默认角色和权限失败: %w", err))
	}

	// 创建默认管理员账户
	if err := receiver.createDefaultAdmin(db); err != nil {
		panic(fmt.Errorf("创建默认管理员账户失败: %w", err))
	}

	return nil
}

// createDefaultDepartment 创建默认部门
func (receiver *Migration) createDefaultDepartment(db any) error {
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
		Attributes:  datatypes.JSON("{}"), // 使用空map
		Remark:      "系统默认根部门",
	}

	// 使用 GORM 接口
	gormDB := receiver.Helper.GetDatabase()
	if err := gormDB.Create(rootDept).Error; err != nil {
		return err
	}

	// 创建管理员部门
	adminDept := &model.Department{
		Name:        "管理员",
		Code:        "admin",
		ShortName:   "管理",
		Description: "系统管理员部门",
		Type:        model.DepartmentTypeDepartment,
		Status:      model.DepartmentStatusEnabled,
		ParentID:    &rootDept.ID,
		Level:       2,
		Path:        fmt.Sprintf("/1/%d", rootDept.ID),
		Sort:        1,
		DataScope:   1, // 全部数据权限
		IsLeaf:      true,
		HasChildren: false,
		Attributes:  datatypes.JSON("{}"), // 使用空map
		Remark:      "系统默认管理员部门",
	}

	if err := gormDB.Create(adminDept).Error; err != nil {
		return fmt.Errorf("创建管理员部门失败: %w", err)
	}

	return nil
}

// createDefaultRoles 创建默认角色和权限
func (receiver *Migration) createDefaultRoles(db any) (uint, error) {
	gormDB := receiver.Helper.GetDatabase()

	// 创建超级管理员角色
	superAdminRole := &model.Role{
		Name:        "超级管理员",
		Code:        "super_admin",
		Description: "系统超级管理员，拥有所有权限",
		Type:        model.RoleTypeSystem,
		Status:      model.RoleStatusEnabled,
		Level:       1,
		Path:        "/1",
		DataScope:   1,                    // 全部数据权限
		Attributes:  datatypes.JSON("{}"), // 使用空map
		Remark:      "系统默认超级管理员角色",
	}

	if err := gormDB.Create(superAdminRole).Error; err != nil {
		return 0, err
	}

	return superAdminRole.ID, nil
}

// createDefaultPermissions 创建默认角色和权限
func (receiver *Migration) createDefaultPermissions(db any) error {
	gormDB := receiver.Helper.GetDatabase()

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
	if err := gormDB.Create(&permissions).Error; err != nil {
		return err
	}

	return nil
}

// createDefaultAdmin 创建默认管理员账户
func (receiver *Migration) createDefaultAdmin(db any) error {
	gormDB := receiver.Helper.GetDatabase()

	// 获取管理员部门ID
	var adminDept model.Department
	if err := gormDB.Where("code = ?", "admin").First(&adminDept).Error; err != nil {
		return fmt.Errorf("获取管理员部门失败: %w", err)
	}

	// 获取超级管理员角色ID
	var superAdminRole model.Role
	if err := gormDB.Where("code = ?", "super_admin").First(&superAdminRole).Error; err != nil {
		return fmt.Errorf("获取超级管理员角色失败: %w", err)
	}

	// 生成默认密码的哈希值
	defaultPassword := "admin123"
	salt, passwordHash, err := receiver.generatePasswordHash(defaultPassword)
	if err != nil {
		return fmt.Errorf("生成默认密码哈希失败: %w", err)
	}

	// 创建默认管理员用户
	admin := &model.User{
		Username:     "admin",
		Email:        "admin@example.com",
		Nickname:     "系统管理员",
		RealName:     "系统管理员",
		Salt:         salt,
		PasswordHash: passwordHash,
		Status:       model.UserStatusActive,
		IsActivated:  true,
		IsVerified:   true,
		LoginCount:   0,
		FailureCount: 0,
		Remark:       "系统默认管理员账户",
	}

	if err := gormDB.Create(admin).Error; err != nil {
		return fmt.Errorf("创建默认管理员账户失败: %w", err)
	}

	// 建立用户和部门的关联关系（通过 user_departments 表）
	now := time.Now()
	userDepartment := &model.UserDepartment{
		UserID:       admin.ID,
		DepartmentID: adminDept.ID,
		Position:     "系统管理员",
		Type:         model.UserDepartmentTypeLeader,
		IsActive:     true,
		IsPrimary:    true,
		Sort:         0,
		JoinedAt:     &now,
		Remark:       "系统默认管理员部门关联",
	}

	if err := gormDB.Create(userDepartment).Error; err != nil {
		return fmt.Errorf("建立管理员部门关联失败: %w", err)
	}

	// 为管理员分配超级管理员角色
	userRole := &model.UserRole{
		UserID:         admin.ID,
		RoleID:         superAdminRole.ID,
		AssignedBy:     admin.ID, // 自己分配给自己
		AssignedAt:     time.Now(),
		IsActive:       true,
		IsPrimary:      true,
		ApplyScope:     1, // 全局应用
		ApprovalStatus: 1, // 已批准
		DataScope:      1, // 全部数据权限
	}

	if err := gormDB.Create(userRole).Error; err != nil {
		return fmt.Errorf("为管理员分配角色失败: %w", err)
	}

	receiver.Helper.GetLogger().Info(fmt.Sprintf("[默认管理员账户创建成功] 用户名: %s, 密码: %s", admin.Username, defaultPassword))

	return nil
}

// generatePasswordHash 生成密码哈希
func (receiver *Migration) generatePasswordHash(password string) (salt, hash string, err error) {
	// 生成盐值
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", err
	}
	salt = hex.EncodeToString(saltBytes)

	// 生成密码哈希
	h := sha256.New()
	h.Write([]byte(password + salt))
	hash = hex.EncodeToString(h.Sum(nil))

	return salt, hash, nil
}
