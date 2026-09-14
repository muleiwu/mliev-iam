package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/script"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"cnb.cool/mliev/open/go-web/pkg/server/database/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// runningImports 记录本实例进行中任务的本地 cancel（同实例取消的快路径）。
// 跨实例取消依赖 Redis 取消标志 + watcher，见 migration_import_lock.go。
var runningImports sync.Map // map[uint]context.CancelFunc

// MigrationImportService 一次性批量导入服务。
type MigrationImportService struct {
	db        *gorm.DB
	rdb       *redis.Client
	sourceDao *dao.MigrationSourceDao
	legacyDao *dao.LegacyUserDao
	taskDao   *dao.MigrationTaskDao
}

func NewMigrationImportService(db *gorm.DB) *MigrationImportService {
	return &MigrationImportService{
		db:        db,
		rdb:       helper.GetRedis(),
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

	// DB 层兜底检查（无 Redis 的单实例场景仍可去重）。
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

	// 抢占源级分布式锁（跨实例互斥）。token 用 taskID，释放/续期时校验持有者。
	lockToken := fmt.Sprintf("%d", task.ID)
	ok, lockErr := acquireImportLock(s.rdb, sourceID, lockToken)
	if lockErr != nil {
		s.failTask(task.ID, "获取分布式锁失败: "+lockErr.Error())
		return nil, fmt.Errorf("获取分布式锁失败: %w", lockErr)
	}
	if !ok {
		_ = s.taskDao.UpdateFields(task.ID, map[string]interface{}{
			"status": model.MigrationTaskStatusFailed,
			"error":  "该迁移源已有进行中的导入任务（分布式锁）",
		})
		return nil, fmt.Errorf("该迁移源已有进行中的导入任务")
	}

	// 后台执行。复制源以避免外部修改影响。参考 install_controller 的 goroutine 用法。
	go s.runImport(task.ID, *source, lockToken)

	return task, nil
}

// CancelImport 请求取消任务（跨实例）。写 Redis 取消标志，运行该任务的实例
// 上的 watcher 会感知并停止；同实例再走本地 cancel 快路径。
func (s *MigrationImportService) CancelImport(taskID uint) error {
	task, err := s.taskDao.GetByID(taskID)
	if err != nil {
		return fmt.Errorf("任务不存在")
	}
	if task.Status != model.MigrationTaskStatusPending && task.Status != model.MigrationTaskStatusRunning {
		return fmt.Errorf("任务不在运行中")
	}

	setImportCancelFlag(s.rdb, taskID)
	if cancel, ok := runningImports.Load(taskID); ok {
		cancel.(context.CancelFunc)()
	}
	return nil
}

// runImport 是后台导入主循环：连旧库 → 逐行跑 transform_script → upsert legacy_users。
// lockToken 为本任务持有的源级分布式锁令牌，结束时据此安全释放。
func (s *MigrationImportService) runImport(taskID uint, source model.MigrationSource, lockToken string) {
	ctx, cancel := context.WithCancel(context.Background())
	runningImports.Store(taskID, cancel)

	// 看门狗：定期续期分布式锁，防止长任务期间锁过期被他人抢占。
	stopWatchdog := make(chan struct{})
	go func() {
		ticker := time.NewTicker(importLockRenew)
		defer ticker.Stop()
		for {
			select {
			case <-stopWatchdog:
				return
			case <-ticker.C:
				renewImportLock(s.rdb, source.ID, lockToken)
			}
		}
	}()

	// 取消 watcher：轮询 Redis 取消标志，感知到则本地 cancel（支持跨实例取消）。
	go func() {
		ticker := time.NewTicker(importCancelPoll)
		defer ticker.Stop()
		for {
			select {
			case <-stopWatchdog:
				return
			case <-ticker.C:
				if isImportCancelRequested(s.rdb, taskID) {
					cancel()
					return
				}
			}
		}
	}()

	defer func() {
		cancel()
		close(stopWatchdog)
		runningImports.Delete(taskID)
		releaseImportLock(s.rdb, source.ID, lockToken)
		clearImportCancelFlag(s.rdb, taskID)
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
	errSampler := newErrorSampler()

	for rows.Next() {
		select {
		case <-ctx.Done():
			s.finishTask(taskID, model.MigrationTaskStatusCanceled, processed, succeeded, failed, errSampler.summary())
			return
		default:
		}

		rowMap, scanErr := scanRowToMap(rows, columns)
		if scanErr != nil {
			failed++
			processed++
			errSampler.add("扫描数据行失败: " + scanErr.Error())
			continue
		}

		legacyUser, mapErr := s.mapRow(engine, &source, rowMap)
		if mapErr != nil {
			failed++
			errSampler.add("字段映射失败: " + mapErr.Error())
		} else if err := s.legacyDao.Upsert(legacyUser); err != nil {
			failed++
			errSampler.add("写入失败: " + err.Error())
		} else {
			succeeded++
		}
		processed++

		if processed%100 == 0 {
			_ = s.taskDao.UpdateFields(taskID, map[string]interface{}{
				"processed": processed,
				"succeeded": succeeded,
				"failed":    failed,
				"error":     errSampler.summary(),
			})
		}
	}
	if err := rows.Err(); err != nil {
		s.finishTask(taskID, model.MigrationTaskStatusFailed, processed, succeeded, failed, "遍历结果集出错: "+err.Error())
		return
	}

	s.finishTask(taskID, model.MigrationTaskStatusSucceeded, processed, succeeded, failed, errSampler.summary())
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

// errorSampler 聚合导入过程中的逐行错误：按错误信息归类计数，仅保留前若干种，
// 用于在任务结束时回填一份可读的失败原因摘要（否则失败只有计数、无从排查）。
type errorSampler struct {
	counts map[string]int64
	order  []string
	total  int64
}

const errorSamplerMaxKinds = 5

func newErrorSampler() *errorSampler {
	return &errorSampler{counts: make(map[string]int64)}
}

func (s *errorSampler) add(msg string) {
	s.total++
	// 截断单条过长的错误，避免摘要膨胀。
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	if _, ok := s.counts[msg]; !ok {
		if len(s.order) >= errorSamplerMaxKinds {
			return // 种类已满，仅累计 total，不再记新种类
		}
		s.order = append(s.order, msg)
	}
	s.counts[msg]++
}

// summary 返回形如 "共 N 条失败；映射失败: 缺少 username (×N) | 写入失败: ... (×M)"。
func (s *errorSampler) summary() string {
	if s.total == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "共 %d 条失败", s.total)
	for i, msg := range s.order {
		if i == 0 {
			b.WriteString("；")
		} else {
			b.WriteString(" | ")
		}
		fmt.Fprintf(&b, "%s (×%d)", msg, s.counts[msg])
	}
	if int64(len(s.order)) >= errorSamplerMaxKinds {
		b.WriteString(" | …更多错误已省略")
	}
	return b.String()
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
