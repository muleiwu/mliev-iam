package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// MigrationTaskDao 导入任务数据访问。
type MigrationTaskDao struct {
	db *gorm.DB
}

func NewMigrationTaskDao(db *gorm.DB) *MigrationTaskDao {
	return &MigrationTaskDao{db: db}
}

func (dao *MigrationTaskDao) Create(t *model.MigrationTask) error {
	return dao.db.Create(t).Error
}

func (dao *MigrationTaskDao) GetByID(id uint) (*model.MigrationTask, error) {
	var t model.MigrationTask
	if err := dao.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (dao *MigrationTaskDao) UpdateFields(id uint, fields map[string]interface{}) error {
	return dao.db.Model(&model.MigrationTask{}).Where("id = ?", id).Updates(fields).Error
}

// List 列出任务，可按 sourceID 过滤，按 id 倒序。
func (dao *MigrationTaskDao) List(sourceID uint, page, pageSize int) ([]model.MigrationTask, int64, error) {
	var tasks []model.MigrationTask
	var total int64

	query := dao.db.Model(&model.MigrationTask{})
	if sourceID > 0 {
		query = query.Where("source_id = ?", sourceID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// HasRunningTask 判断某源是否已有进行中的导入任务，避免并发重复导入。
func (dao *MigrationTaskDao) HasRunningTask(sourceID uint) (bool, error) {
	var count int64
	err := dao.db.Model(&model.MigrationTask{}).
		Where("source_id = ? AND status IN ?", sourceID,
			[]model.MigrationTaskStatus{model.MigrationTaskStatusPending, model.MigrationTaskStatusRunning}).
		Count(&count).Error
	return count > 0, err
}
