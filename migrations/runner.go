package migrations

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	glebarezsqlite "github.com/glebarez/sqlite"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// dialectMap 归一化驱动名到 goose 方言名。
var dialectMap = map[string]string{
	"mysql":      "mysql",
	"mariadb":    "mysql",
	"postgresql": "postgres",
	"postgres":   "postgres",
	"sqlite":     "sqlite3",
	"sqlite3":    "sqlite3",
}

// dirByDriver 按驱动选择 embed 目录。
var dirByDriver = map[string]string{
	"mysql":      "mysql",
	"mariadb":    "mysql",
	"postgresql": "postgresql",
	"postgres":   "postgresql",
	"sqlite":     "sqlite",
	"sqlite3":    "sqlite",
}

// legacySchemaSentinelTable 用来识别"已经存在非 goose 管理的 schema"。
// users 表自 initial_schema 起就存在，后续所有版本都依赖它，实际上不会被删除。
const legacySchemaSentinelTable = "users"

const (
	userFailureCountMigrationVersion         int64 = 20260529000001
	userFailureCountPreviousMigrationVersion int64 = 20260524000001
	userFailureCountColumn                         = "failure_count"
	identityConnectionCreateVersion          int64 = 20260720130000
	identityConnectionCutoverVersion         int64 = 20260720131000
)

// gooseVersionRecord 用于通过 GORM 写入 goose 的版本表，避免手工拼接表名和 SQL。
type gooseVersionRecord struct {
	VersionID int64     `gorm:"column:version_id"`
	IsApplied bool      `gorm:"column:is_applied"`
	Timestamp time.Time `gorm:"column:tstamp"`
}

// baselineLegacyVersions 是本次重构之前已在生产 MySQL 上部署过的迁移版本集合。
// 如果检测到 sentinel 表存在但 goose_db_version 里无任何记录（历史上通过
// AutoMigrate 起家、还未引入 goose），把这些版本标记为 applied，避免
// goose.Up 重跑撞 "table already exists"。
//
// 仅在 MySQL 驱动下起作用——PostgreSQL / SQLite 本次才新增支持，绝无"遗留 schema"。
var baselineLegacyVersions = []int64{
	20241201000001, // initial_schema
	20241225000001, // add_user_uuid
	20260114000001, // create_captcha_configs
}

// Logger 迁移执行期使用的最小日志接口。
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
// 不做 baseline 兼容，仅用于纯新装。
func RunGooseMigrations(db *sql.DB, driver string, logger Logger) error {
	return RunGooseMigrationsWithGorm(db, nil, driver, logger)
}

// RunGooseMigrationsWithGorm 是完整版本：gormDB 非空时可做"遗留 schema 感知"的 baseline。
// install_controller 会传 gormDB，以便识别老 MySQL 部署避免撞库。
func RunGooseMigrationsWithGorm(db *sql.DB, gormDB *gorm.DB, driver string, logger Logger) error {
	if logger == nil {
		logger = stdoutLogger{}
	}
	// The unified connection schema always stores encrypted credentials. Check
	// the deployment key before goose can change any schema, including on an
	// already-migrated database, so a bad rotation cannot surface at login time.
	if _, err := credentialcrypto.FromEnvironment(); err != nil {
		return fmt.Errorf("凭据主密钥检查失败: %w", err)
	}

	dialect, ok := dialectMap[driver]
	if !ok {
		return fmt.Errorf("不支持的数据库驱动: %s", driver)
	}
	dir, ok := dirByDriver[driver]
	if !ok {
		return fmt.Errorf("缺少驱动 %s 的迁移目录映射", driver)
	}

	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("设置 goose 方言失败 (%s -> %s): %w", driver, dialect, err)
	}
	goose.SetBaseFS(FS)

	if gormDB != nil && dialect == "mysql" {
		if err := baselineIfLegacyMySQL(db, gormDB, logger); err != nil {
			logger.Warn("baseline 失败: " + err.Error())
		}
		if err := reconcileKnownMySQLSchemaDrift(db, gormDB, dir, logger); err != nil {
			return err
		}
	}

	currentVersion, _ := goose.GetDBVersion(db)
	if currentVersion < identityConnectionCreateVersion {
		if err := goose.UpTo(db, dir, identityConnectionCreateVersion); err != nil {
			return fmt.Errorf("创建统一身份连接表失败: %w", err)
		}
		currentVersion, _ = goose.GetDBVersion(db)
	}

	if currentVersion >= identityConnectionCreateVersion && currentVersion < identityConnectionCutoverVersion {
		migrationDB := gormDB
		if migrationDB == nil {
			var err error
			migrationDB, err = openGormOnSQLDB(db, driver)
			if err != nil {
				return fmt.Errorf("初始化身份连接数据迁移器失败: %w", err)
			}
		}
		if err := migrateLegacyIdentityData(migrationDB); err != nil {
			return fmt.Errorf("统一身份连接数据迁移失败（旧表已保留）: %w", err)
		}
		if err := goose.UpTo(db, dir, identityConnectionCutoverVersion); err != nil {
			return fmt.Errorf("删除旧身份连接表失败: %w", err)
		}
	}

	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("goose 迁移失败: %w", err)
	}
	newVersion, _ := goose.GetDBVersion(db)
	if newVersion != currentVersion {
		logger.Info(fmt.Sprintf("[db migration] goose 迁移完成: %d -> %d (driver=%s)", currentVersion, newVersion, driver))
	} else {
		logger.Info(fmt.Sprintf("[db migration] goose 已是最新版本 (driver=%s)", driver))
	}

	return nil
}

func openGormOnSQLDB(db *sql.DB, driver string) (*gorm.DB, error) {
	config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	switch driver {
	case "mysql", "mariadb":
		return gorm.Open(mysql.New(mysql.Config{Conn: db}), config)
	case "postgres", "postgresql":
		return gorm.Open(postgres.New(postgres.Config{Conn: db}), config)
	case "sqlite", "sqlite3":
		return gorm.Open(&glebarezsqlite.Dialector{Conn: db}, config)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", driver)
	}
}

// reconcileKnownMySQLSchemaDrift 修复"字段已存在但 goose 版本未记录"的历史漂移。
// 这类库不能直接重跑 ADD COLUMN，也不能直接把最高版本写入，否则会跳过中间迁移。
func reconcileKnownMySQLSchemaDrift(db *sql.DB, gormDB *gorm.DB, dir string, logger Logger) error {
	if !gormDB.Migrator().HasColumn(legacySchemaSentinelTable, userFailureCountColumn) {
		return nil
	}

	currentVersion, err := goose.GetDBVersion(db)
	if err != nil {
		return fmt.Errorf("检查 goose 当前版本失败: %w", err)
	}
	if currentVersion >= userFailureCountMigrationVersion {
		return nil
	}

	if currentVersion < userFailureCountPreviousMigrationVersion {
		if err := goose.UpTo(db, dir, userFailureCountPreviousMigrationVersion); err != nil {
			return fmt.Errorf("补跑 %s 前置迁移失败: %w", userFailureCountColumn, err)
		}
	}

	return markGooseVersionAppliedIfMissing(
		gormDB,
		userFailureCountMigrationVersion,
		fmt.Sprintf("%s.%s 已存在", legacySchemaSentinelTable, userFailureCountColumn),
		logger,
	)
}

func markGooseVersionAppliedIfMissing(gormDB *gorm.DB, version int64, reason string, logger Logger) error {
	var count int64
	if err := gormDB.Table(goose.TableName()).
		Where("version_id = ? AND is_applied = ?", version, true).
		Count(&count).Error; err != nil {
		return fmt.Errorf("检查 goose version=%d 失败: %w", version, err)
	}
	if count > 0 {
		return nil
	}

	if err := gormDB.Table(goose.TableName()).Create(&gooseVersionRecord{
		VersionID: version,
		IsApplied: true,
		Timestamp: time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("写入 goose version=%d 失败: %w", version, err)
	}

	logger.Warn(fmt.Sprintf("[db migration] 检测到 %s 但 goose 缺少 version=%d，已标记为 applied", reason, version))
	return nil
}

// baselineIfLegacyMySQL 处理"老 MySQL 库通过 AutoMigrate 起家、从未跑过 goose"的场景。
//
// 条件（全部满足才 baseline）：
//  1. goose_db_version 不存在 或 当前版本 == 0
//  2. legacy sentinel 表（users）已存在
//
// fresh install 时条件 2 为假，直接让 goose.Up 跑全套 SQL 建表。
// 正常管理中的 MySQL 库条件 1 为假（currentVersion 已是最近那个版本），跳过。
func baselineIfLegacyMySQL(db *sql.DB, gormDB *gorm.DB, logger Logger) error {
	currentVersion, err := goose.EnsureDBVersion(db)
	if err != nil && !errors.Is(err, goose.ErrNoNextVersion) {
		return fmt.Errorf("EnsureDBVersion: %w", err)
	}
	if currentVersion > 0 {
		return nil
	}
	if !gormDB.Migrator().HasTable(legacySchemaSentinelTable) {
		return nil
	}

	now := time.Now()
	for _, v := range baselineLegacyVersions {
		// 使用 GORM 借其占位符抽象；MySQL 走 "?" 没问题。
		var count int64
		if err := gormDB.Table(goose.TableName()).
			Where("version_id = ?", v).Count(&count).Error; err != nil {
			return fmt.Errorf("检查已有 baseline version=%d: %w", v, err)
		}
		if count > 0 {
			continue
		}
		if err := gormDB.Table(goose.TableName()).Create(&gooseVersionRecord{
			VersionID: v,
			IsApplied: true,
			Timestamp: now,
		}).Error; err != nil {
			return fmt.Errorf("写入 baseline version=%d: %w", v, err)
		}
	}
	logger.Info(fmt.Sprintf(
		"[db migration] 检测到遗留 MySQL schema（%s 表已存在、goose 无记录），已标记 %d 个历史版本为 applied",
		legacySchemaSentinelTable, len(baselineLegacyVersions),
	))
	return nil
}
