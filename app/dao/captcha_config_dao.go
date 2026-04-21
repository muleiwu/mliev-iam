package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// CaptchaConfigDao 验证码配置数据访问对象
type CaptchaConfigDao struct {
	db *gorm.DB
}

// NewCaptchaConfigDao 创建验证码配置DAO实例
func NewCaptchaConfigDao(db *gorm.DB) *CaptchaConfigDao {
	return &CaptchaConfigDao{db: db}
}

// Create 创建验证码配置
func (d *CaptchaConfigDao) Create(config *model.CaptchaConfig) error {
	return d.db.Create(config).Error
}

// GetByID 根据ID获取配置
func (d *CaptchaConfigDao) GetByID(id uint) (*model.CaptchaConfig, error) {
	var config model.CaptchaConfig
	if err := d.db.First(&config, id).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

// GetAll 获取所有配置
func (d *CaptchaConfigDao) GetAll() ([]model.CaptchaConfig, error) {
	var configs []model.CaptchaConfig
	if err := d.db.Order("id DESC").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

// GetByProvider 根据提供商类型获取配置列表
func (d *CaptchaConfigDao) GetByProvider(provider string) ([]model.CaptchaConfig, error) {
	var configs []model.CaptchaConfig
	if err := d.db.Where("provider = ?", provider).Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

// GetEnabled 获取所有启用的配置
func (d *CaptchaConfigDao) GetEnabled() ([]model.CaptchaConfig, error) {
	var configs []model.CaptchaConfig
	if err := d.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

// Update 更新配置
func (d *CaptchaConfigDao) Update(config *model.CaptchaConfig) error {
	return d.db.Save(config).Error
}

// UpdateEnabled 更新启用状态
func (d *CaptchaConfigDao) UpdateEnabled(id uint, enabled bool) error {
	return d.db.Model(&model.CaptchaConfig{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// Delete 删除配置
func (d *CaptchaConfigDao) Delete(id uint) error {
	return d.db.Delete(&model.CaptchaConfig{}, id).Error
}

// Exists 检查配置是否存在
func (d *CaptchaConfigDao) Exists(id uint) (bool, error) {
	var count int64
	if err := d.db.Model(&model.CaptchaConfig{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByName 检查名称是否存在
func (d *CaptchaConfigDao) ExistsByName(name string, excludeID uint) (bool, error) {
	query := d.db.Model(&model.CaptchaConfig{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
