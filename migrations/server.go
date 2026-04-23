package migrations

import (
	"fmt"

	"cnb.cool/mliev/open/go-web/pkg/helper"
	"cnb.cool/mliev/open/go-web/pkg/interfaces"
)

// compile-time 校验实现 ServerInterface
var _ interfaces.ServerInterface = (*Server)(nil)

// Server 把 RunGooseMigrationsWithGorm 包装成每次启动都跑的服务。
//
// 历史背景：框架自带的 migration.Migration 服务用 goose.Up(db, "migrations") 读
// 文件系统根目录；我们的迁移按方言放在 migrations/{mysql,postgresql,sqlite}/
// 且通过 //go:embed 打包，框架那个版本扫不到 SQL 文件，升级时老库就错过新迁移。
// 这里接管：每次启动按当前 driver 选对应子目录，用 embed FS 跑 goose.Up。
type Server struct{}

func (s *Server) Run() error {
	gormDB := helper.GetDatabase()
	if gormDB == nil {
		return fmt.Errorf("[migration] 数据库未初始化")
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("[migration] 获取 sql.DB 失败: %w", err)
	}

	driver := helper.GetConfig().GetString("database.driver", "mysql")
	if driver == "memory" {
		helper.GetLogger().Warn("[migration] memory 驱动不支持版本化迁移，已跳过")
		return nil
	}

	return RunGooseMigrationsWithGorm(sqlDB, gormDB, driver, helperLogger{})
}

func (*Server) Stop() error { return nil }

// helperLogger 把 runner 的 Logger 接口桥接到 framework 的 logger。
type helperLogger struct{}

func (helperLogger) Info(m string)  { helper.GetLogger().Info(m) }
func (helperLogger) Warn(m string)  { helper.GetLogger().Warn(m) }
func (helperLogger) Error(m string) { helper.GetLogger().Error(m) }
