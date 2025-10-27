package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// SocialProviderDAO 社会化登录提供商数据访问对象
type SocialProviderDAO struct {
	db *gorm.DB
}

// NewSocialProviderDAO 创建SocialProviderDAO实例
func NewSocialProviderDAO(db *gorm.DB) *SocialProviderDAO {
	return &SocialProviderDAO{db: db}
}

// Create 创建提供商
func (dao *SocialProviderDAO) Create(provider *model.SocialProvider) error {
	return dao.db.Create(provider).Error
}

// GetByID 根据ID获取提供商
func (dao *SocialProviderDAO) GetByID(id uint) (*model.SocialProvider, error) {
	var provider model.SocialProvider
	err := dao.db.First(&provider, id).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// GetByIdentifier 根据标识符获取提供商
func (dao *SocialProviderDAO) GetByIdentifier(identifier string) (*model.SocialProvider, error) {
	var provider model.SocialProvider
	err := dao.db.Where("identifier = ?", identifier).First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// Update 更新提供商
func (dao *SocialProviderDAO) Update(provider *model.SocialProvider) error {
	return dao.db.Save(provider).Error
}

// Delete 删除提供商（软删除）
func (dao *SocialProviderDAO) Delete(id uint) error {
	return dao.db.Delete(&model.SocialProvider{}, id).Error
}

// BatchDelete 批量删除提供商
func (dao *SocialProviderDAO) BatchDelete(ids []uint) error {
	return dao.db.Delete(&model.SocialProvider{}, ids).Error
}

// List 获取提供商列表（分页）
func (dao *SocialProviderDAO) List(page, pageSize int, status *model.ProviderStatus) ([]*model.SocialProvider, int64, error) {
	var providers []*model.SocialProvider
	var total int64

	query := dao.db.Model(&model.SocialProvider{})

	// 状态筛选
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := query.Order("sort_order ASC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&providers).Error

	return providers, total, err
}

// GetAll 获取所有提供商
func (dao *SocialProviderDAO) GetAll() ([]*model.SocialProvider, error) {
	var providers []*model.SocialProvider
	err := dao.db.Order("sort_order ASC, id DESC").Find(&providers).Error
	return providers, err
}

// GetEnabledProviders 获取所有启用的提供商
func (dao *SocialProviderDAO) GetEnabledProviders() ([]*model.SocialProvider, error) {
	var providers []*model.SocialProvider
	err := dao.db.Where("status = ?", model.ProviderStatusEnabled).
		Order("sort_order ASC, id DESC").
		Find(&providers).Error
	return providers, err
}

// UpdateStatus 更新提供商状态
func (dao *SocialProviderDAO) UpdateStatus(id uint, status model.ProviderStatus) error {
	return dao.db.Model(&model.SocialProvider{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// BatchUpdateStatus 批量更新提供商状态
func (dao *SocialProviderDAO) BatchUpdateStatus(ids []uint, status model.ProviderStatus) error {
	return dao.db.Model(&model.SocialProvider{}).
		Where("id IN ?", ids).
		Update("status", status).Error
}

// ExistsByIdentifier 检查标识符是否已存在
func (dao *SocialProviderDAO) ExistsByIdentifier(identifier string, excludeID ...uint) (bool, error) {
	query := dao.db.Model(&model.SocialProvider{}).Where("identifier = ?", identifier)
	if len(excludeID) > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

// Count 统计提供商数量
func (dao *SocialProviderDAO) Count(status *model.ProviderStatus) (int64, error) {
	query := dao.db.Model(&model.SocialProvider{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var count int64
	err := query.Count(&count).Error
	return count, err
}
