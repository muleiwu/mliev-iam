package dao

import (
	"time"

	"cnb.cool/mliev/examples/go-web/app/model"
	"gorm.io/gorm"
)

// AuthorizationCodeDAO 授权码数据访问对象
type AuthorizationCodeDAO struct {
	db *gorm.DB
}

// NewAuthorizationCodeDAO 创建新的授权码DAO
func NewAuthorizationCodeDAO(db *gorm.DB) *AuthorizationCodeDAO {
	return &AuthorizationCodeDAO{db: db}
}

// Create 创建授权码
func (dao *AuthorizationCodeDAO) Create(code *model.AuthorizationCode) error {
	return dao.db.Create(code).Error
}

// GetByCode 根据授权码获取记录
func (dao *AuthorizationCodeDAO) GetByCode(code string) (*model.AuthorizationCode, error) {
	var authCode model.AuthorizationCode
	err := dao.db.Where("code = ?", code).First(&authCode).Error
	if err != nil {
		return nil, err
	}
	return &authCode, nil
}

// Update 更新授权码
func (dao *AuthorizationCodeDAO) Update(code *model.AuthorizationCode) error {
	return dao.db.Save(code).Error
}

// MarkAsUsed 标记授权码为已使用
func (dao *AuthorizationCodeDAO) MarkAsUsed(code string) error {
	return dao.db.Model(&model.AuthorizationCode{}).
		Where("code = ?", code).
		Update("is_used", true).Error
}

// DeleteExpired 删除过期的授权码
func (dao *AuthorizationCodeDAO) DeleteExpired() error {
	return dao.db.Where("expires_at < ?", time.Now()).
		Delete(&model.AuthorizationCode{}).Error
}

// GetByClientAndUser 根据客户端ID和用户ID获取授权码列表
func (dao *AuthorizationCodeDAO) GetByClientAndUser(clientID string, userID uint, limit int) ([]*model.AuthorizationCode, error) {
	var codes []*model.AuthorizationCode
	err := dao.db.Where("client_id = ? AND user_id = ?", clientID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&codes).Error
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// GetValidByCode 获取有效的授权码（未使用且未过期）
func (dao *AuthorizationCodeDAO) GetValidByCode(code string) (*model.AuthorizationCode, error) {
	var authCode model.AuthorizationCode
	err := dao.db.Where("code = ? AND is_used = ? AND expires_at > ?",
		code, false, time.Now()).First(&authCode).Error
	if err != nil {
		return nil, err
	}
	return &authCode, nil
}

// Delete 删除授权码
func (dao *AuthorizationCodeDAO) Delete(id uint) error {
	return dao.db.Delete(&model.AuthorizationCode{}, id).Error
}

// DeleteByCode 根据授权码删除记录
func (dao *AuthorizationCodeDAO) DeleteByCode(code string) error {
	return dao.db.Where("code = ?", code).Delete(&model.AuthorizationCode{}).Error
}

// CountByClient 统计客户端的授权码数量
func (dao *AuthorizationCodeDAO) CountByClient(clientID string, since time.Time) (int64, error) {
	var count int64
	err := dao.db.Model(&model.AuthorizationCode{}).
		Where("client_id = ? AND created_at > ?", clientID, since).
		Count(&count).Error
	return count, err
}

// CountByUser 统计用户的授权码数量
func (dao *AuthorizationCodeDAO) CountByUser(userID uint, since time.Time) (int64, error) {
	var count int64
	err := dao.db.Model(&model.AuthorizationCode{}).
		Where("user_id = ? AND created_at > ?", userID, since).
		Count(&count).Error
	return count, err
}

// List 获取授权码列表（用于管理）
func (dao *AuthorizationCodeDAO) List(offset, limit int, clientID string, userID *uint) ([]*model.AuthorizationCode, int64, error) {
	var codes []*model.AuthorizationCode
	var total int64

	query := dao.db.Model(&model.AuthorizationCode{})

	if clientID != "" {
		query = query.Where("client_id = ?", clientID)
	}
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&codes).Error; err != nil {
		return nil, 0, err
	}

	return codes, total, nil
}
