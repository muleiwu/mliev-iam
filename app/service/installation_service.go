package service

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/migrations"
	"cnb.cool/mliev/iam/mliev-iam/pkg/installation"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

var installMu sync.Mutex
var ErrAlreadyInstalled = errors.New("系统已安装，无法重复安装")

func validateDatabase(config dto.DatabaseConfig) error {
	if config.Driver == "sqlite" {
		if strings.TrimSpace(config.FilePath) == "" || config.FilePath == ":memory:" || strings.Contains(config.FilePath, "mode=memory") {
			return errors.New("请选择持久化 SQLite 文件路径")
		}
		return nil
	}
	if config.Driver != "" && config.Driver != "mysql" && config.Driver != "postgresql" {
		return errors.New("不支持的数据库驱动")
	}
	if config.Host == "" || config.Database == "" || config.Username == "" || config.Port < 1 || config.Port > 65535 {
		return errors.New("请填写数据库地址、端口、用户名和数据库名称")
	}
	return nil
}

func databaseSettings(config dto.DatabaseConfig) map[string]any {
	driver := config.Driver
	if driver == "" {
		driver = "mysql"
	}
	charset := config.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	return map[string]any{
		"database.driver": driver, "database.host": config.Host,
		"database.port": config.Port, "database.username": config.Username,
		"database.password": config.Password, "database.database": config.Database,
		"database.charset": charset, "database.filepath": config.FilePath,
	}
}

// CompleteInstallation serializes submission and persists the final config
// once. A DB marker, committed with the seed data, makes a failed file write
// retryable without resetting the administrator's password.
func CompleteInstallation(req dto.InstallSubmitRequest) error {
	installMu.Lock()
	defer installMu.Unlock()
	if viper.GetBool("app.installed") {
		return ErrAlreadyInstalled
	}
	if err := validateDatabase(req.Database); err != nil {
		return err
	}
	if req.Redis.Host == "" || req.Redis.Port < 1 || req.Redis.Port > 65535 || req.Redis.DB < 0 {
		return errors.New("Redis 配置无效")
	}
	if req.Database.Driver == "sqlite" {
		if err := os.MkdirAll(filepath.Dir(req.Database.FilePath), 0700); err != nil {
			return err
		}
	}
	installer := NewInstallService(nil)
	db, err := installer.TestDatabaseConnection(req.Database)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	redis, err := installer.TestRedisConnection(req.Redis)
	if err != nil {
		return err
	}
	defer redis.Close()
	if err := installation.EnsureCredentialKey(false); err != nil {
		return err
	}
	id, err := installation.PendingID()
	if err != nil {
		return err
	}
	driver := req.Database.Driver
	if driver == "" {
		driver = "mysql"
	}
	if err := migrations.RunGooseMigrationsWithGorm(sqlDB, db, driver, nil); err != nil {
		return err
	}
	if err := initializeInstallationOnce(db, req.Admin, id); err != nil {
		return err
	}
	values := databaseSettings(req.Database)
	values["redis.host"], values["redis.port"] = req.Redis.Host, req.Redis.Port
	values["redis.password"], values["redis.db"] = req.Redis.Password, req.Redis.DB
	values["app.installed"] = true
	if os.Getenv("JWT_SECRET_KEY") == "" && viper.GetString("jwt.secret") == "" {
		secret := make([]byte, 48)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		values["jwt.secret"] = base64.StdEncoding.EncodeToString(secret)
	}
	if err := installation.PersistSettings(values); err != nil {
		return fmt.Errorf("保存安装配置失败，可修复文件权限后重试: %w", err)
	}
	return nil
}

func initializeInstallationOnce(db *gorm.DB, admin dto.AdminAccountInfo, id string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var marker model.Setting
		err := tx.Where("setting_key = ?", "system.installation_id").First(&marker).Error
		if err == nil {
			if marker.Value != id {
				return errors.New("数据库已由其他安装初始化，请使用原配置启动")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := tx.Unscoped().Model(&model.User{}).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("数据库已有用户，请使用原配置升级；安装向导不会覆盖已有账户")
		}
		if err := NewInstallService(tx).CreateInitialData(admin); err != nil {
			return err
		}
		return tx.Create(&model.Setting{Key: "system.installation_id", Value: id, Category: "system"}).Error
	})
}
