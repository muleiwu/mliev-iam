package service

import (
	"context"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/script"
	"gorm.io/gorm"
)

// MigrationSourceService 迁移源业务逻辑。
type MigrationSourceService struct {
	db        *gorm.DB
	sourceDao *dao.MigrationSourceDao
	engine    *script.Engine
}

func NewMigrationSourceService(db *gorm.DB) *MigrationSourceService {
	return &MigrationSourceService{
		db:        db,
		sourceDao: dao.NewMigrationSourceDao(db),
		engine:    script.NewEngine(0),
	}
}

// Create 创建迁移源。
func (s *MigrationSourceService) Create(req *dto.CreateMigrationSourceRequest) (*model.MigrationSource, error) {
	uuid, err := utils.GenerateUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("生成 UUID 失败: %w", err)
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	source := &model.MigrationSource{
		UUID:             uuid,
		Name:             req.Name,
		Type:             model.MigrationSourceType(req.Type),
		Enabled:          enabled,
		Sort:             req.Sort,
		ConnectionConfig: model.JSONObject(req.ConnectionConfig),
		LoginScript:      req.LoginScript,
		TransformScript:  req.TransformScript,
		Remark:           req.Remark,
	}
	if err := s.sourceDao.Create(source); err != nil {
		return nil, fmt.Errorf("创建迁移源失败: %w", err)
	}
	return source, nil
}

// Update 更新迁移源（仅更新提供的字段）。
func (s *MigrationSourceService) Update(id uint, req *dto.UpdateMigrationSourceRequest) (*model.MigrationSource, error) {
	source, err := s.sourceDao.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("迁移源不存在")
	}

	if req.Name != "" {
		source.Name = req.Name
	}
	if req.Enabled != nil {
		source.Enabled = *req.Enabled
	}
	if req.Sort != nil {
		source.Sort = *req.Sort
	}
	if req.ConnectionConfig != nil {
		source.ConnectionConfig = model.JSONObject(req.ConnectionConfig)
	}
	if req.LoginScript != nil {
		source.LoginScript = *req.LoginScript
	}
	if req.TransformScript != nil {
		source.TransformScript = *req.TransformScript
	}
	if req.Remark != nil {
		source.Remark = *req.Remark
	}

	if err := s.sourceDao.Update(source); err != nil {
		return nil, fmt.Errorf("更新迁移源失败: %w", err)
	}
	return source, nil
}

func (s *MigrationSourceService) Delete(id uint) error {
	return s.sourceDao.Delete(id)
}

func (s *MigrationSourceService) Get(id uint) (*model.MigrationSource, error) {
	return s.sourceDao.GetByID(id)
}

func (s *MigrationSourceService) List() ([]model.MigrationSource, error) {
	return s.sourceDao.List()
}

// TestScript 试运行脚本，返回脚本输出，便于管理员在后台调试。
func (s *MigrationSourceService) TestScript(req *dto.TestScriptRequest) (any, error) {
	input := req.Input
	if input == nil {
		input = map[string]any{}
	}
	return s.engine.Run(context.Background(), req.Script, input)
}

// TestConnection 测试旧库连通性并返回探测到的行数（如配置了 query）。
func (s *MigrationSourceService) TestConnection(config map[string]any) (map[string]any, error) {
	legacyDB, err := openLegacyDB(config)
	if err != nil {
		return nil, err
	}
	defer closeLegacyDB(legacyDB)

	sqlDB, err := legacyDB.DB()
	if err != nil {
		return nil, fmt.Errorf("获取连接失败: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}

	result := map[string]any{"connected": true}
	if query := asConfigString(config["query"]); query != "" {
		var rows []map[string]any
		// 仅取一行做样例预览，避免拉全表。
		if err := legacyDB.Raw(query).Limit(1).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("执行查询失败: %w", err)
		}
		if len(rows) > 0 {
			result["sampleRow"] = rows[0]
		}
	}
	return result, nil
}
