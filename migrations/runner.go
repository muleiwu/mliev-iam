package migrations

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// Logger 迁移执行期使用的最小日志接口（避免把 gsr 引入到本包）。
type Logger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}

type stdoutLogger struct{}

func (stdoutLogger) Info(m string)  { fmt.Println("[INFO]", m) }
func (stdoutLogger) Warn(m string)  { fmt.Println("[WARN]", m) }
func (stdoutLogger) Error(m string) { fmt.Println("[ERROR]", m) }

// RunGooseMigrations 执行 goose 版本化迁移（供 install 流程使用）。
// 本函数不依赖项目内部 helper/interfaces，只需 *sql.DB 与驱动名即可。
func RunGooseMigrations(db *sql.DB, dialect string, logger Logger) error {
	return RunGooseMigrationsWithGorm(db, nil, dialect, logger)
}

// RunGooseMigrationsWithGorm 是完整版本：gormDB 非空时会通过 SetExternalDB
// 注入给 initial_schema 里的 AutoMigrate 使用。
func RunGooseMigrationsWithGorm(db *sql.DB, gormDB *gorm.DB, dialect string, logger Logger) error {
	if logger == nil {
		logger = stdoutLogger{}
	}

	if gormDB != nil {
		SetExternalDB(gormDB)
		defer ClearExternalDB()
	}

	gooseDialect := mapDriverToGooseDialect(dialect)
	if err := goose.SetDialect(gooseDialect); err != nil {
		return fmt.Errorf("设置 goose 方言失败 (%s -> %s): %w", dialect, gooseDialect, err)
	}

	currentVersion, err := goose.GetDBVersion(db)
	if err != nil {
		logger.Info("[db migration] 初始化 goose 迁移表...")
		currentVersion = 0
	} else {
		logger.Info(fmt.Sprintf("[db migration] 当前数据库版本: %d", currentVersion))
	}

	for {
		if err := goose.UpByOne(db, "."); err != nil {
			if err == goose.ErrNoNextVersion {
				break
			}
			return fmt.Errorf("执行迁移失败: %w", err)
		}
	}

	newVersion, _ := goose.GetDBVersion(db)
	if newVersion != currentVersion {
		logger.Info(fmt.Sprintf("[db migration] goose 迁移完成: 版本 %d -> %d", currentVersion, newVersion))
	} else {
		logger.Info("[db migration] goose 迁移已是最新版本")
	}

	return nil
}

// mapDriverToGooseDialect 驱动名到 goose 方言的映射：
//   - postgres/postgresql → postgres
//   - mysql/mariadb → mysql
//   - sqlite/sqlite3 → sqlite3
//   - mssql/sqlserver → mssql
func mapDriverToGooseDialect(driver string) string {
	switch driver {
	case "postgres", "postgresql":
		return "postgres"
	case "mysql", "mariadb":
		return "mysql"
	case "sqlite", "sqlite3":
		return "sqlite3"
	case "mssql", "sqlserver":
		return "mssql"
	default:
		if driver == "" {
			return "mysql"
		}
		return driver
	}
}
