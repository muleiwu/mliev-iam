package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

type IdentityProviderDAO struct {
	db *gorm.DB
}

func NewIdentityProviderDAO(db *gorm.DB) *IdentityProviderDAO {
	return &IdentityProviderDAO{db: db}
}

func (dao *IdentityProviderDAO) Create(provider *model.IdentityProvider) error {
	return dao.db.Create(provider).Error
}

func (dao *IdentityProviderDAO) GetByID(id uint) (*model.IdentityProvider, error) {
	var provider model.IdentityProvider
	if err := dao.db.First(&provider, id).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

func (dao *IdentityProviderDAO) GetByIdentifier(identifier string) (*model.IdentityProvider, error) {
	var provider model.IdentityProvider
	if err := dao.db.Where("identifier = ?", identifier).First(&provider).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

func (dao *IdentityProviderDAO) Update(provider *model.IdentityProvider) error {
	return dao.db.Save(provider).Error
}

func (dao *IdentityProviderDAO) Delete(id uint) error {
	return dao.db.Delete(&model.IdentityProvider{}, id).Error
}

func (dao *IdentityProviderDAO) BatchDelete(ids []uint) error {
	return dao.db.Delete(&model.IdentityProvider{}, ids).Error
}

func (dao *IdentityProviderDAO) List(page, pageSize int, status *model.IdentityProviderStatus) ([]*model.IdentityProvider, int64, error) {
	var providers []*model.IdentityProvider
	var total int64

	query := dao.db.Model(&model.IdentityProvider{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("sort_order ASC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&providers).Error
	return providers, total, err
}

func (dao *IdentityProviderDAO) GetEnabledProviders() ([]*model.IdentityProvider, error) {
	var providers []*model.IdentityProvider
	err := dao.db.Where("status = ?", model.IdentityProviderStatusEnabled).
		Order("sort_order ASC, id DESC").
		Find(&providers).Error
	return providers, err
}

func (dao *IdentityProviderDAO) BatchUpdateStatus(ids []uint, status model.IdentityProviderStatus) error {
	return dao.db.Model(&model.IdentityProvider{}).
		Where("id IN ?", ids).
		Update("status", status).Error
}

func (dao *IdentityProviderDAO) ExistsByIdentifier(identifier string, excludeID ...uint) (bool, error) {
	query := dao.db.Model(&model.IdentityProvider{}).Where("identifier = ?", identifier)
	if len(excludeID) > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

func (dao *IdentityProviderDAO) Count(status *model.IdentityProviderStatus) (int64, error) {
	query := dao.db.Model(&model.IdentityProvider{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}
