package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// MigrationSourceDao 迁移源数据访问。
type MigrationSourceDao struct {
	db *gorm.DB
}

func NewMigrationSourceDao(db *gorm.DB) *MigrationSourceDao {
	return &MigrationSourceDao{db: db}
}

func (dao *MigrationSourceDao) Create(source *model.MigrationSource) error {
	return dao.db.Create(source).Error
}

func (dao *MigrationSourceDao) Update(source *model.MigrationSource) error {
	return dao.db.Save(source).Error
}

func (dao *MigrationSourceDao) UpdateFields(id uint, fields map[string]interface{}) error {
	return dao.db.Model(&model.MigrationSource{}).Where("id = ?", id).Updates(fields).Error
}

func (dao *MigrationSourceDao) Delete(id uint) error {
	return dao.db.Delete(&model.MigrationSource{}, id).Error
}

func (dao *MigrationSourceDao) GetByID(id uint) (*model.MigrationSource, error) {
	var source model.MigrationSource
	if err := dao.db.First(&source, id).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

// List 返回全部迁移源，按 sort、id 升序。
func (dao *MigrationSourceDao) List() ([]model.MigrationSource, error) {
	var sources []model.MigrationSource
	err := dao.db.Order("sort ASC, id ASC").Find(&sources).Error
	return sources, err
}

// ListEnabled 返回启用的迁移源，按 sort、id 升序（登录钩子按此顺序尝试）。
func (dao *MigrationSourceDao) ListEnabled() ([]model.MigrationSource, error) {
	var sources []model.MigrationSource
	err := dao.db.Where("enabled = ?", true).Order("sort ASC, id ASC").Find(&sources).Error
	return sources, err
}
