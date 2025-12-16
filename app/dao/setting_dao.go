package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// SettingDao 设置数据访问对象
type SettingDao struct {
	db *gorm.DB
}

// NewSettingDao 创建设置DAO实例
func NewSettingDao(db *gorm.DB) *SettingDao {
	return &SettingDao{db: db}
}

// Create 创建设置
func (d *SettingDao) Create(setting *model.Setting) error {
	return d.db.Create(setting).Error
}

// GetByKey 根据key获取设置
func (d *SettingDao) GetByKey(key string) (*model.Setting, error) {
	var setting model.Setting
	if err := d.db.Where("key = ?", key).First(&setting).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

// GetByCategory 根据分类获取设置列表
func (d *SettingDao) GetByCategory(category string) ([]model.Setting, error) {
	var settings []model.Setting
	if err := d.db.Where("category = ?", category).Find(&settings).Error; err != nil {
		return nil, err
	}
	return settings, nil
}

// GetAll 获取所有设置
func (d *SettingDao) GetAll() ([]model.Setting, error) {
	var settings []model.Setting
	if err := d.db.Find(&settings).Error; err != nil {
		return nil, err
	}
	return settings, nil
}

// Update 更新设置
func (d *SettingDao) Update(setting *model.Setting) error {
	return d.db.Save(setting).Error
}

// UpdateByKey 根据key更新设置值
func (d *SettingDao) UpdateByKey(key, value string) error {
	return d.db.Model(&model.Setting{}).Where("key = ?", key).Update("value", value).Error
}

// Delete 删除设置
func (d *SettingDao) Delete(id uint) error {
	return d.db.Delete(&model.Setting{}, id).Error
}

// DeleteByKey 根据key删除设置
func (d *SettingDao) DeleteByKey(key string) error {
	return d.db.Where("key = ?", key).Delete(&model.Setting{}).Error
}

// Exists 检查key是否存在
func (d *SettingDao) Exists(key string) (bool, error) {
	var count int64
	if err := d.db.Model(&model.Setting{}).Where("key = ?", key).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateOrUpdate 创建或更新设置
func (d *SettingDao) CreateOrUpdate(setting *model.Setting) error {
	exists, err := d.Exists(setting.Key)
	if err != nil {
		return err
	}

	if exists {
		// 更新现有设置
		return d.db.Model(&model.Setting{}).Where("key = ?", setting.Key).Updates(map[string]interface{}{
			"value":    setting.Value,
			"category": setting.Category,
			"remark":   setting.Remark,
		}).Error
	}

	// 创建新设置
	return d.Create(setting)
}
