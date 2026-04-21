package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
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

// CheckInstallStatus 检查安装状态
func (s *InstallService) CheckInstallStatus() *dto.InstallCheckResponse {
	// 读取配置文件中的 installed 标记
	installed := viper.GetBool("app.installed")

	response := &dto.InstallCheckResponse{
		Installed:          installed,
		DatabaseConnected:  false,
		DatabaseConfigured: false,
	}

	// 检查数据库配置是否存在
	dbHost := viper.GetString("database.host")
	dbName := viper.GetString("database.database")
	if dbHost != "" && dbName != "" {
		response.DatabaseConfigured = true
		response.CurrentDatabase = dbName
	}

	// 检查数据库连接状态
	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err == nil {
			if err := sqlDB.Ping(); err == nil {
				response.DatabaseConnected = true
			}
		}
	}

	if installed {
		response.Message = "系统已安装，无法重复安装"
	} else if !response.DatabaseConfigured {
		response.Message = "请配置数据库连接信息"
	} else if !response.DatabaseConnected {
		response.Message = "数据库连接失败，请检查配置"
	} else {
		response.Message = "系统未安装，可以开始安装"
	}

	return response
}

// TestDatabaseConnection 测试数据库连接
func (s *InstallService) TestDatabaseConnection(config dto.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	driver := config.Driver
	if driver == "" {
		driver = "mysql" // 默认使用 MySQL
	}

	switch driver {
	case "mysql":
		// 设置默认字符集
		charset := config.Charset
		if charset == "" {
			charset = "utf8mb4"
		}

		// 构建MySQL DSN
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
			config.Username,
			config.Password,
			config.Host,
			config.Port,
			config.Database,
			charset,
		)
		dialector = mysql.Open(dsn)

	case "postgresql":
		// 构建PostgreSQL DSN
		dsn := fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
			config.Username,
			config.Password,
			config.Host,
			config.Port,
			config.Database,
		)
		dialector = postgres.New(postgres.Config{
			DSN:                  dsn,
			PreferSimpleProtocol: true,
		})

	case "sqlite":
		// SQLite 使用文件路径
		filePath := config.FilePath
		if filePath == "" {
			filePath = "mliev_iam.db"
		}
		dialector = sqlite.Open(filePath)

	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", driver)
	}

	// 尝试连接数据库
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	// 测试连接
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库实例失败: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库ping失败: %w", err)
	}

	// 返回数据库连接供后续使用
	return db, nil
}

// TestRedisConnection 测试Redis连接
func (s *InstallService) TestRedisConnection(config dto.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password: config.Password,
		DB:       config.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis连接失败: %w", err)
	}

	return client, nil
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

	// 创建管理员用户
	admin := &model.User{
		Username:     adminInfo.Username,
		Email:        email,
		Phone:        phone,
		Nickname:     adminInfo.RealName,
		RealName:     adminInfo.RealName,
		Salt:         salt,
		PasswordHash: passwordHash,
		Status:       model.UserStatusActive,
		IsActivated:  true,
		IsVerified:   true,
		LoginCount:   0,
		Remark:       "系统管理员账户",
	}

	if err := tx.Create(admin).Error; err != nil {
		return fmt.Errorf("创建管理员账户失败: %w", err)
	}

	// 建立用户和部门的关联关系
	now := time.Now()
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

// generatePasswordHash 生成密码哈希
func (s *InstallService) generatePasswordHash(password string) (salt, hash string, err error) {
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

// UpdateDatabaseConfig 更新数据库配置到配置文件
func (s *InstallService) UpdateDatabaseConfig(config dto.DatabaseConfig) error {
	// 设置驱动类型，默认为 mysql
	driver := config.Driver
	if driver == "" {
		driver = "mysql"
	}

	// 更新配置
	viper.Set("database.driver", driver)

	if driver == "sqlite" {
		// SQLite 只需要文件路径
		filePath := config.FilePath
		if filePath == "" {
			filePath = "mliev_iam.db"
		}
		viper.Set("database.filepath", filePath)
	} else {
		// MySQL 和 PostgreSQL 需要连接信息
		viper.Set("database.host", config.Host)
		viper.Set("database.port", config.Port)
		viper.Set("database.username", config.Username)
		viper.Set("database.password", config.Password)
		viper.Set("database.database", config.Database)

		// MySQL 需要字符集配置
		if driver == "mysql" {
			charset := config.Charset
			if charset == "" {
				charset = "utf8mb4"
			}
			viper.Set("database.charset", charset)
		}
	}

	// 写入配置文件
	// 先尝试 SafeWriteConfig（如果文件不存在会创建）
	viper.SetConfigFile("./config/config.yaml")
	if err := viper.SafeWriteConfig(); err != nil {
		// 如果文件已存在，使用 WriteConfig 更新
		if err := viper.WriteConfig(); err != nil {
			return fmt.Errorf("写入数据库配置失败: %w", err)
		}
	}

	return nil
}

// UpdateRedisConfig 更新Redis配置到配置文件
func (s *InstallService) UpdateRedisConfig(config dto.RedisConfig) error {
	// 更新配置
	viper.Set("redis.host", config.Host)
	viper.Set("redis.port", config.Port)
	viper.Set("redis.password", config.Password)
	viper.Set("redis.db", config.DB)

	// 写入配置文件
	viper.SetConfigFile("./config/config.yaml")
	if err := viper.SafeWriteConfig(); err != nil {
		// 如果文件已存在，使用 WriteConfig 更新
		if err := viper.WriteConfig(); err != nil {
			return fmt.Errorf("写入Redis配置失败: %w", err)
		}
	}

	return nil
}

// MarkAsInstalled 标记系统为已安装
func (s *InstallService) MarkAsInstalled() error {
	// 设置安装标记
	viper.Set("app.installed", true)

	// 写入配置文件
	// 先尝试 SafeWriteConfig（如果文件不存在会创建）
	viper.SetConfigFile("./config/config.yaml")
	if err := viper.SafeWriteConfig(); err != nil {
		// 如果文件已存在，使用 WriteConfig 更新
		if err := viper.WriteConfig(); err != nil {
			return fmt.Errorf("标记系统已安装失败: %w", err)
		}
	}

	return nil
}

// createDatabaseTables 创建数据库表结构
func (s *InstallService) createDatabaseTables() error {
	cfg := helper.GetConfig()
	migrationConfig := cfg.Get("database.migration", []any{})
	if migrationConfig == nil {
		return fmt.Errorf("未找到数据库迁移配置")
	}

	models, ok := migrationConfig.([]any)
	if !ok {
		if typed, okTyped := migrationConfig.([]interface{}); okTyped {
			models = typed
		} else {
			return fmt.Errorf("数据库迁移配置格式错误")
		}
	}

	if len(models) == 0 {
		return fmt.Errorf("数据库迁移配置为空")
	}

	driver := cfg.GetString("database.driver", "mysql")

	if driver == "mysql" {
		s.db.Exec("SET FOREIGN_KEY_CHECKS = 0;")
	}

	helper.GetLogger().Warn("[install] 执行 GORM AutoMigrate 建表；后续 schema 变更请用 migrations/*.go 而非 AutoMigrate")
	if err := s.db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("数据库表结构迁移失败: %w", err)
	}

	if driver == "mysql" {
		s.db.Exec("SET FOREIGN_KEY_CHECKS = 1;")
	}

	return nil
}
