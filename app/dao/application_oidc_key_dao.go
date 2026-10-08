package dao

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// ApplicationOIDCKeyDAO OIDC应用密钥数据访问对象
type ApplicationOIDCKeyDAO struct {
	db *gorm.DB
}

// NewApplicationOIDCKeyDAO 创建新的应用密钥DAO
func NewApplicationOIDCKeyDAO(db *gorm.DB) *ApplicationOIDCKeyDAO {
	return &ApplicationOIDCKeyDAO{db: db}
}

// Create 创建密钥
func (dao *ApplicationOIDCKeyDAO) Create(key *model.ApplicationOIDCKey) error {
	return dao.db.Create(key).Error
}

// GetByID 根据ID获取密钥
func (dao *ApplicationOIDCKeyDAO) GetByID(id uint) (*model.ApplicationOIDCKey, error) {
	var key model.ApplicationOIDCKey
	err := dao.db.First(&key, id).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// GetByKeyID 根据KeyID获取密钥
func (dao *ApplicationOIDCKeyDAO) GetByKeyID(keyID string) (*model.ApplicationOIDCKey, error) {
	var key model.ApplicationOIDCKey
	err := dao.db.Where("key_id = ?", keyID).First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// GetPrimaryByApplicationID 获取应用的主密钥
func (dao *ApplicationOIDCKeyDAO) GetPrimaryByApplicationID(appID uint) (*model.ApplicationOIDCKey, error) {
	var key model.ApplicationOIDCKey
	err := dao.db.Where("application_id = ? AND is_primary = ?", appID, true).First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// ListByApplicationID 获取应用的所有密钥
func (dao *ApplicationOIDCKeyDAO) ListByApplicationID(appID uint) ([]*model.ApplicationOIDCKey, error) {
	var keys []*model.ApplicationOIDCKey
	err := dao.db.Where("application_id = ?", appID).Order("is_primary DESC, created_at DESC").Find(&keys).Error
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// ListValidKeysByApplicationID 获取应用的所有有效密钥（未过期或过期时间为null）
func (dao *ApplicationOIDCKeyDAO) ListValidKeysByApplicationID(appID uint) ([]*model.ApplicationOIDCKey, error) {
	var keys []*model.ApplicationOIDCKey
	err := dao.db.Where("application_id = ? AND (expires_at IS NULL OR expires_at > ?)", appID, time.Now()).
		Order("is_primary DESC, created_at DESC").
		Find(&keys).Error
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// Update 更新密钥
func (dao *ApplicationOIDCKeyDAO) Update(key *model.ApplicationOIDCKey) error {
	return dao.db.Save(key).Error
}

// SetPrimary 设置主密钥（事务操作：先取消其他主密钥，再设置新主密钥）
func (dao *ApplicationOIDCKeyDAO) SetPrimary(appID uint, keyID uint) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		// 1. 取消该应用下所有密钥的主密钥状态
		if err := tx.Model(&model.ApplicationOIDCKey{}).
			Where("application_id = ?", appID).
			Update("is_primary", false).Error; err != nil {
			return err
		}

		// 2. 设置指定密钥为主密钥
		if err := tx.Model(&model.ApplicationOIDCKey{}).
			Where("id = ? AND application_id = ?", keyID, appID).
			Update("is_primary", true).Error; err != nil {
			return err
		}

		return nil
	})
}

// Delete 删除密钥
func (dao *ApplicationOIDCKeyDAO) Delete(id uint) error {
	return dao.db.Delete(&model.ApplicationOIDCKey{}, id).Error
}

// DeleteByApplicationID 删除应用的所有密钥
func (dao *ApplicationOIDCKeyDAO) DeleteByApplicationID(appID uint) error {
	return dao.db.Where("application_id = ?", appID).Delete(&model.ApplicationOIDCKey{}).Error
}

// CountByApplicationID 统计应用的密钥数量
func (dao *ApplicationOIDCKeyDAO) CountByApplicationID(appID uint) (int64, error) {
	var count int64
	err := dao.db.Model(&model.ApplicationOIDCKey{}).Where("application_id = ?", appID).Count(&count).Error
	return count, err
}

// ListExpiredKeys 获取所有已过期的密钥
func (dao *ApplicationOIDCKeyDAO) ListExpiredKeys() ([]*model.ApplicationOIDCKey, error) {
	var keys []*model.ApplicationOIDCKey
	err := dao.db.Where("expires_at IS NOT NULL AND expires_at < ?", gorm.Expr("NOW()")).Find(&keys).Error
	if err != nil {
		return nil, err
	}
	return keys, nil
}
