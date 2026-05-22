package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

type UserIdentityAccountDAO struct {
	db *gorm.DB
}

func NewUserIdentityAccountDAO(db *gorm.DB) *UserIdentityAccountDAO {
	return &UserIdentityAccountDAO{db: db}
}

func (dao *UserIdentityAccountDAO) Create(account *model.UserIdentityAccount) error {
	return dao.db.Create(account).Error
}

func (dao *UserIdentityAccountDAO) GetByID(id uint) (*model.UserIdentityAccount, error) {
	var account model.UserIdentityAccount
	if err := dao.db.First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (dao *UserIdentityAccountDAO) GetByProviderAndExternalUserID(providerID uint, externalUserID string) (*model.UserIdentityAccount, error) {
	var account model.UserIdentityAccount
	if err := dao.db.Where("provider_id = ? AND external_user_id = ?", providerID, externalUserID).
		First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (dao *UserIdentityAccountDAO) GetByUserAndProvider(userID, providerID uint) (*model.UserIdentityAccount, error) {
	var account model.UserIdentityAccount
	if err := dao.db.Where("user_id = ? AND provider_id = ?", userID, providerID).
		First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (dao *UserIdentityAccountDAO) GetByUserID(userID uint) ([]*model.UserIdentityAccount, error) {
	var accounts []*model.UserIdentityAccount
	err := dao.db.Where("user_id = ?", userID).
		Preload("Provider").
		Order("created_at DESC").
		Find(&accounts).Error
	return accounts, err
}

func (dao *UserIdentityAccountDAO) Delete(id uint) error {
	return dao.db.Delete(&model.UserIdentityAccount{}, id).Error
}

func (dao *UserIdentityAccountDAO) ExistsByProviderAndExternalUserID(providerID uint, externalUserID string, excludeUserID ...uint) (bool, error) {
	query := dao.db.Model(&model.UserIdentityAccount{}).
		Where("provider_id = ? AND external_user_id = ?", providerID, externalUserID)
	if len(excludeUserID) > 0 {
		query = query.Where("user_id != ?", excludeUserID[0])
	}

	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

func (dao *UserIdentityAccountDAO) CountByProvider(providerID uint) (int64, error) {
	var count int64
	err := dao.db.Model(&model.UserIdentityAccount{}).
		Where("provider_id = ?", providerID).
		Count(&count).Error
	return count, err
}

func (dao *UserIdentityAccountDAO) CountByUser(userID uint) (int64, error) {
	var count int64
	err := dao.db.Model(&model.UserIdentityAccount{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return count, err
}

func (dao *UserIdentityAccountDAO) GetTotalCount() (int64, error) {
	var count int64
	err := dao.db.Model(&model.UserIdentityAccount{}).Count(&count).Error
	return count, err
}

func (dao *UserIdentityAccountDAO) UpdateLastUsed(id uint) error {
	return dao.db.Model(&model.UserIdentityAccount{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_used_at": gorm.Expr("NOW()"),
			"login_count":  gorm.Expr("login_count + 1"),
		}).Error
}
