package service

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/script"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// runningImports 记录进行中的导入任务，支持取消。key=taskID。
var runningImports sync.Map // map[uint]context.CancelFunc

// MigrationImportService 一次性批量导入服务。
type MigrationImportService struct {
	db        *gorm.DB
	sourceDao *dao.MigrationSourceDao
	legacyDao *dao.LegacyUserDao
	taskDao   *dao.MigrationTaskDao
}

func NewMigrationImportService(db *gorm.DB) *MigrationImportService {
	return &MigrationImportService{
		db:        db,
		sourceDao: dao.NewMigrationSourceDao(db),
		legacyDao: dao.NewLegacyUserDao(db),
		taskDao:   dao.NewMigrationTaskDao(db),
	}
}

// StartImport 校验后创建任务并起后台 goroutine，立即返回任务记录。
func (s *MigrationImportService) StartImport(sourceID uint) (*model.MigrationTask, error) {
	source, err := s.sourceDao.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("迁移源不存在")
	}
	if source.Type != model.MigrationSourceTypeDatabase {
		return nil, fmt.Errorf("仅数据库类型的迁移源支持批量导入")
	}
	if asConfigString(source.ConnectionConfig["query"]) == "" {
		return nil, fmt.Errorf("迁移源未配置查询语句 (connectionConfig.query)")
	}

	running, err := s.taskDao.HasRunningTask(sourceID)
	if err != nil {
		return nil, err
	}
	if running {
		return nil, fmt.Errorf("该迁移源已有进行中的导入任务")
	}

	task := &model.MigrationTask{
		SourceID: sourceID,
		Status:   model.MigrationTaskStatusPending,
	}
	if err := s.taskDao.Create(task); err != nil {
		return nil, fmt.Errorf("创建导入任务失败: %w", err)
	}

	// 后台执行。复制源以避免外部修改影响。参考 install_controller 的 goroutine 用法。
	go s.runImport(task.ID, *source)

	return task, nil
}

// CancelImport 请求取消进行中的任务。
func (s *MigrationImportService) CancelImport(taskID uint) error {
	if cancel, ok := runningImports.Load(taskID); ok {
		cancel.(context.CancelFunc)()
		return nil
	}
	return fmt.Errorf("任务不在运行中")
}

// runImport 是后台导入主循环：连旧库 → 逐行跑 transform_script → upsert legacy_users。
func (s *MigrationImportService) runImport(taskID uint, source model.MigrationSource) {
	ctx, cancel := context.WithCancel(context.Background())
	runningImports.Store(taskID, cancel)
	defer func() {
		cancel()
		runningImports.Delete(taskID)
		if r := recover(); r != nil {
			s.failTask(taskID, fmt.Sprintf("导入异常: %v", r))
		}
	}()

	now := time.Now()
	_ = s.taskDao.UpdateFields(taskID, map[string]interface{}{
		"status":     model.MigrationTaskStatusRunning,
		"started_at": now,
	})

	legacyDB, err := openLegacyDB(map[string]any(source.ConnectionConfig))
	if err != nil {
		s.failTask(taskID, "连接旧库失败: "+err.Error())
		return
	}
	defer closeLegacyDB(legacyDB)

	sqlDB, err := legacyDB.DB()
	if err != nil {
		s.failTask(taskID, "获取旧库连接失败: "+err.Error())
		return
	}

	query := asConfigString(source.ConnectionConfig["query"])
	rows, err := sqlDB.QueryContext(ctx, query)
	if err != nil {
		s.failTask(taskID, "执行查询失败: "+err.Error())
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		s.failTask(taskID, "读取列信息失败: "+err.Error())
		return
	}

	engine := script.NewEngine(0)
	var processed, succeeded, failed int64

	for rows.Next() {
		select {
		case <-ctx.Done():
			s.finishTask(taskID, model.MigrationTaskStatusCanceled, processed, succeeded, failed, "任务已取消")
			return
		default:
		}

		rowMap, scanErr := scanRowToMap(rows, columns)
		if scanErr != nil {
			failed++
			processed++
			continue
		}

		legacyUser, mapErr := s.mapRow(engine, &source, rowMap)
		if mapErr != nil {
			failed++
		} else if err := s.legacyDao.Upsert(legacyUser); err != nil {
			failed++
		} else {
			succeeded++
		}
		processed++

		if processed%100 == 0 {
			_ = s.taskDao.UpdateFields(taskID, map[string]interface{}{
				"processed": processed,
				"succeeded": succeeded,
				"failed":    failed,
			})
		}
	}
	if err := rows.Err(); err != nil {
		s.finishTask(taskID, model.MigrationTaskStatusFailed, processed, succeeded, failed, "遍历结果集出错: "+err.Error())
		return
	}

	s.finishTask(taskID, model.MigrationTaskStatusSucceeded, processed, succeeded, failed, "")
}

// mapRow 用 transform_script 把旧行映射为 LegacyUser；未配置脚本时按列名直取。
func (s *MigrationImportService) mapRow(engine *script.Engine, source *model.MigrationSource, rowMap map[string]any) (*model.LegacyUser, error) {
	mapped := rowMap
	if source.TransformScript != "" {
		out, err := engine.RunForMap(context.Background(), source.TransformScript, map[string]any{"row": rowMap})
		if err != nil {
			return nil, err
		}
		mapped = out
	}

	username := asConfigString(mapped["username"])
	if username == "" {
		return nil, fmt.Errorf("映射结果缺少 username")
	}

	u := &model.LegacyUser{
		SourceID:     source.ID,
		Username:     username,
		ExternalID:   asConfigString(mapped["externalId"]),
		Nickname:     asConfigString(mapped["nickname"]),
		PasswordHash: asConfigString(mapped["passwordHash"]),
		Salt:         asConfigString(mapped["salt"]),
		RawData:      model.JSONObject(rowMap),
		Status:       model.LegacyUserStatusPending,
	}
	if email := asConfigString(mapped["email"]); email != "" {
		u.Email = &email
	}
	if phone := asConfigString(mapped["phone"]); phone != "" {
		u.Phone = &phone
	}
	return u, nil
}

func (s *MigrationImportService) failTask(taskID uint, msg string) {
	now := time.Now()
	_ = s.taskDao.UpdateFields(taskID, map[string]interface{}{
		"status":      model.MigrationTaskStatusFailed,
		"error":       msg,
		"finished_at": now,
	})
}

func (s *MigrationImportService) finishTask(taskID uint, status model.MigrationTaskStatus, processed, succeeded, failed int64, msg string) {
	now := time.Now()
	_ = s.taskDao.UpdateFields(taskID, map[string]interface{}{
		"status":      status,
		"processed":   processed,
		"succeeded":   succeeded,
		"failed":      failed,
		"error":       msg,
		"finished_at": now,
	})
	helper.GetLogger().Info(fmt.Sprintf("[migration] 导入任务 #%d 结束 status=%s processed=%d succeeded=%d failed=%d",
		taskID, status, processed, succeeded, failed))
}

// ---- 旧库连接与行扫描辅助 ----

// openLegacyDB 按 connection_config 打开一个独立的旧库 GORM 连接。
// 支持 {"driver":"mysql|postgresql|sqlite","dsn":"..."}。
func openLegacyDB(config map[string]any) (*gorm.DB, error) {
	driver := asConfigString(config["driver"])
	if driver == "" {
		driver = "mysql"
	}
	dsn := asConfigString(config["dsn"])
	if dsn == "" {
		return nil, fmt.Errorf("connectionConfig 缺少 dsn")
	}

	var dialector gorm.Dialector
	switch driver {
	case "mysql", "mariadb":
		dialector = mysql.Open(dsn)
	case "postgres", "postgresql":
		dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
	case "sqlite", "sqlite3":
		dialector = sqlite.Open(dsn)
	default:
		return nil, fmt.Errorf("不支持的旧库驱动: %s", driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("打开旧库失败: %w", err)
	}
	return db, nil
}

func closeLegacyDB(db *gorm.DB) {
	if db == nil {
		return
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// scanRowToMap 把当前 *sql.Rows 的一行扫描为 map[string]any，[]byte 统一转字符串。
func scanRowToMap(rows *sql.Rows, columns []string) (map[string]any, error) {
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := rows.Scan(pointers...); err != nil {
		return nil, err
	}
	m := make(map[string]any, len(columns))
	for i, col := range columns {
		v := values[i]
		if b, ok := v.([]byte); ok {
			m[col] = string(b)
		} else {
			m[col] = v
		}
	}
	return m, nil
}

func asConfigString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}
