package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// UserSocialAccountDAO 用户社交账号数据访问对象
type UserSocialAccountDAO struct {
	db *gorm.DB
}

// NewUserSocialAccountDAO 创建UserSocialAccountDAO实例
func NewUserSocialAccountDAO(db *gorm.DB) *UserSocialAccountDAO {
	return &UserSocialAccountDAO{db: db}
}

// Create 创建用户社交账号绑定
func (dao *UserSocialAccountDAO) Create(account *model.UserSocialAccount) error {
	return dao.db.Create(account).Error
}

// GetByID 根据ID获取社交账号
func (dao *UserSocialAccountDAO) GetByID(id uint) (*model.UserSocialAccount, error) {
	var account model.UserSocialAccount
	err := dao.db.First(&account, id).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// GetByProviderAndOpenID 根据提供商和OpenID获取社交账号
func (dao *UserSocialAccountDAO) GetByProviderAndOpenID(providerID uint, openID string) (*model.UserSocialAccount, error) {
	var account model.UserSocialAccount
	err := dao.db.Where("provider_id = ? AND open_id = ?", providerID, openID).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// GetByUserAndProvider 根据用户ID和提供商ID获取社交账号
func (dao *UserSocialAccountDAO) GetByUserAndProvider(userID, providerID uint) (*model.UserSocialAccount, error) {
	var account model.UserSocialAccount
	err := dao.db.Where("user_id = ? AND provider_id = ?", userID, providerID).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// GetByUserID 获取用户的所有社交账号
func (dao *UserSocialAccountDAO) GetByUserID(userID uint) ([]*model.UserSocialAccount, error) {
	var accounts []*model.UserSocialAccount
	err := dao.db.Where("user_id = ?", userID).
		Preload("Provider").
		Order("created_at DESC").
		Find(&accounts).Error
	return accounts, err
}

// Update 更新社交账号信息
func (dao *UserSocialAccountDAO) Update(account *model.UserSocialAccount) error {
	return dao.db.Save(account).Error
}

// Delete 删除社交账号绑定（软删除）
func (dao *UserSocialAccountDAO) Delete(id uint) error {
	return dao.db.Delete(&model.UserSocialAccount{}, id).Error
}

// DeleteByUserAndProvider 根据用户ID和提供商ID删除绑定
func (dao *UserSocialAccountDAO) DeleteByUserAndProvider(userID, providerID uint) error {
	return dao.db.Where("user_id = ? AND provider_id = ?", userID, providerID).
		Delete(&model.UserSocialAccount{}).Error
}

// ExistsByProviderAndOpenID 检查OpenID是否已被其他用户绑定
func (dao *UserSocialAccountDAO) ExistsByProviderAndOpenID(providerID uint, openID string, excludeUserID ...uint) (bool, error) {
	query := dao.db.Model(&model.UserSocialAccount{}).
		Where("provider_id = ? AND open_id = ?", providerID, openID)

	if len(excludeUserID) > 0 {
		query = query.Where("user_id != ?", excludeUserID[0])
	}

	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

// CountByProvider 统计某个提供商的绑定数量
func (dao *UserSocialAccountDAO) CountByProvider(providerID uint) (int64, error) {
	var count int64
	err := dao.db.Model(&model.UserSocialAccount{}).
		Where("provider_id = ?", providerID).
		Count(&count).Error
	return count, err
}

// CountByUser 统计某个用户的社交账号绑定数量
func (dao *UserSocialAccountDAO) CountByUser(userID uint) (int64, error) {
	var count int64
	err := dao.db.Model(&model.UserSocialAccount{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return count, err
}

// GetTotalCount 获取总绑定数
func (dao *UserSocialAccountDAO) GetTotalCount() (int64, error) {
	var count int64
	err := dao.db.Model(&model.UserSocialAccount{}).Count(&count).Error
	return count, err
}

// GetProviderUsageStats 获取各提供商的使用统计
func (dao *UserSocialAccountDAO) GetProviderUsageStats() ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	err := dao.db.Model(&model.UserSocialAccount{}).
		Select("provider_id, COUNT(*) as binding_count, SUM(login_count) as total_login_count").
		Group("provider_id").
		Find(&results).Error
	return results, err
}

// UpdateLastUsed 更新最后使用时间和登录次数
func (dao *UserSocialAccountDAO) UpdateLastUsed(id uint) error {
	return dao.db.Model(&model.UserSocialAccount{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_used_at": gorm.Expr("NOW()"),
			"login_count":  gorm.Expr("login_count + 1"),
		}).Error
}

// UpdateTokenInfo 更新令牌信息
func (dao *UserSocialAccountDAO) UpdateTokenInfo(id uint, accessToken, refreshToken string, expiresAt interface{}) error {
	updates := map[string]interface{}{
		"access_token": accessToken,
	}
	if refreshToken != "" {
		updates["refresh_token"] = refreshToken
	}
	if expiresAt != nil {
		updates["expires_at"] = expiresAt
	}

	return dao.db.Model(&model.UserSocialAccount{}).
		Where("id = ?", id).
		Updates(updates).Error
}
