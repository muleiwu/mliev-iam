package dao

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// OIDCTokenDAO OIDC令牌数据访问对象
type OIDCTokenDAO struct {
	db *gorm.DB
}

// NewOIDCTokenDAO 创建新的OIDC令牌DAO
func NewOIDCTokenDAO(db *gorm.DB) *OIDCTokenDAO {
	return &OIDCTokenDAO{db: db}
}

// Create 创建OIDC令牌
func (dao *OIDCTokenDAO) Create(token *model.OIDCToken) error {
	return dao.db.Create(token).Error
}

// GetByAccessToken 根据访问令牌获取记录
func (dao *OIDCTokenDAO) GetByAccessToken(accessToken string) (*model.OIDCToken, error) {
	var token model.OIDCToken
	tokenHash := hashAccessToken(accessToken)
	err := dao.db.Where("access_token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// GetByRefreshToken 根据刷新令牌获取记录
func (dao *OIDCTokenDAO) GetByRefreshToken(refreshToken string) (*model.OIDCToken, error) {
	var token model.OIDCToken
	err := dao.db.Where("refresh_token = ?", refreshToken).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// GetValidByAccessToken 获取有效的访问令牌（未撤销且未过期）
func (dao *OIDCTokenDAO) GetValidByAccessToken(accessToken string) (*model.OIDCToken, error) {
	var token model.OIDCToken
	tokenHash := hashAccessToken(accessToken)
	err := dao.db.Where("access_token_hash = ? AND is_revoked = ? AND expires_at > ?",
		tokenHash, false, time.Now()).First(&token).Error
	if err != nil {
		return nil, err
	}
	if token.UserID > 0 {
		if _, err := NewLoginSessionDAO(dao.db).Valid(token.LoginSessionID, token.UserID); err != nil {
			return nil, err
		}
	}
	return &token, nil
}

// GetValidByRefreshToken 获取有效的刷新令牌
func (dao *OIDCTokenDAO) GetValidByRefreshToken(refreshToken string) (*model.OIDCToken, error) {
	var token model.OIDCToken
	err := dao.db.Where("refresh_token = ? AND is_revoked = ? AND (refresh_expires_at IS NULL OR refresh_expires_at > ?)",
		refreshToken, false, time.Now()).First(&token).Error
	if err != nil {
		return nil, err
	}
	if token.UserID > 0 {
		if _, err := NewLoginSessionDAO(dao.db).Valid(token.LoginSessionID, token.UserID); err != nil {
			return nil, err
		}
	}
	return &token, nil
}

// Update 更新OIDC令牌
func (dao *OIDCTokenDAO) Update(token *model.OIDCToken) error {
	return dao.db.Save(token).Error
}

// RevokeToken 撤销令牌
func (dao *OIDCTokenDAO) RevokeToken(accessToken string) error {
	tokenHash := hashAccessToken(accessToken)
	return dao.db.Model(&model.OIDCToken{}).
		Where("access_token_hash = ?", tokenHash).
		Update("is_revoked", true).Error
}

// RevokeByRefreshToken 根据刷新令牌撤销
func (dao *OIDCTokenDAO) RevokeByRefreshToken(refreshToken string) error {
	result := dao.db.Model(&model.OIDCToken{}).
		Where("refresh_token = ?", refreshToken).
		Update("is_revoked", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// RevokeByClient 撤销客户端的所有令牌
func (dao *OIDCTokenDAO) RevokeByClient(clientID string) error {
	return dao.db.Model(&model.OIDCToken{}).
		Where("client_id = ?", clientID).
		Update("is_revoked", true).Error
}

// RevokeByUser 撤销用户的所有令牌
func (dao *OIDCTokenDAO) RevokeByUser(userID uint) error {
	return dao.db.Model(&model.OIDCToken{}).
		Where("user_id = ?", userID).
		Update("is_revoked", true).Error
}

// RevokeByUsers 批量撤销多个用户的所有令牌。
func (dao *OIDCTokenDAO) RevokeByUsers(userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}
	return dao.db.Model(&model.OIDCToken{}).
		Where("user_id IN ?", userIDs).
		Update("is_revoked", true).Error
}

// RevokeByClientAndUser 撤销特定客户端和用户的令牌
func (dao *OIDCTokenDAO) RevokeByClientAndUser(clientID string, userID uint) error {
	return dao.db.Model(&model.OIDCToken{}).
		Where("client_id = ? AND user_id = ?", clientID, userID).
		Update("is_revoked", true).Error
}

// RevokeByAuthorizationCode 撤销所有由指定授权码换出的令牌
// 用于 code 重放检测后的 token 失效清理（RFC 6749 §4.1.2 要求）
func (dao *OIDCTokenDAO) RevokeByAuthorizationCode(authCodeID uint) error {
	return dao.db.Model(&model.OIDCToken{}).
		Where("authorization_code_id = ?", authCodeID).
		Update("is_revoked", true).Error
}

// DeleteExpired 删除过期的令牌
func (dao *OIDCTokenDAO) DeleteExpired() error {
	// 删除访问令牌过期且刷新令牌也过期的记录
	return dao.db.Where("expires_at < ? AND (refresh_expires_at IS NULL OR refresh_expires_at < ?)",
		time.Now(), time.Now()).Delete(&model.OIDCToken{}).Error
}

// GetByClient 根据客户端ID获取令牌列表
func (dao *OIDCTokenDAO) GetByClient(clientID string, limit int) ([]*model.OIDCToken, error) {
	var tokens []*model.OIDCToken
	err := dao.db.Where("client_id = ?", clientID).
		Order("created_at DESC").
		Limit(limit).
		Find(&tokens).Error
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// GetByUser 根据用户ID获取令牌列表
func (dao *OIDCTokenDAO) GetByUser(userID uint, limit int) ([]*model.OIDCToken, error) {
	var tokens []*model.OIDCToken
	err := dao.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&tokens).Error
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// GetByClientAndUser 根据客户端ID和用户ID获取令牌列表
func (dao *OIDCTokenDAO) GetByClientAndUser(clientID string, userID uint, limit int) ([]*model.OIDCToken, error) {
	var tokens []*model.OIDCToken
	err := dao.db.Where("client_id = ? AND user_id = ?", clientID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&tokens).Error
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// CountByClient 统计客户端的令牌数量
func (dao *OIDCTokenDAO) CountByClient(clientID string, since time.Time) (int64, error) {
	var count int64
	err := dao.db.Model(&model.OIDCToken{}).
		Where("client_id = ? AND created_at > ?", clientID, since).
		Count(&count).Error
	return count, err
}

// CountByUser 统计用户的令牌数量
func (dao *OIDCTokenDAO) CountByUser(userID uint, since time.Time) (int64, error) {
	var count int64
	err := dao.db.Model(&model.OIDCToken{}).
		Where("user_id = ? AND created_at > ?", userID, since).
		Count(&count).Error
	return count, err
}

// CountActiveTokens 统计活跃令牌数量
func (dao *OIDCTokenDAO) CountActiveTokens() (int64, error) {
	var count int64
	err := dao.db.Model(&model.OIDCToken{}).
		Where("is_revoked = ? AND expires_at > ?", false, time.Now()).
		Count(&count).Error
	return count, err
}

// List 获取令牌列表（用于管理）
func (dao *OIDCTokenDAO) List(offset, limit int, clientID string, userID *uint, isRevoked *bool) ([]*model.OIDCToken, int64, error) {
	var tokens []*model.OIDCToken
	var total int64

	query := dao.db.Model(&model.OIDCToken{})

	if clientID != "" {
		query = query.Where("client_id = ?", clientID)
	}
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if isRevoked != nil {
		query = query.Where("is_revoked = ?", *isRevoked)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&tokens).Error; err != nil {
		return nil, 0, err
	}

	return tokens, total, nil
}

// Delete 删除令牌
func (dao *OIDCTokenDAO) Delete(id uint) error {
	return dao.db.Delete(&model.OIDCToken{}, id).Error
}

// hashAccessToken 计算 access_token 的 SHA256 hash
func hashAccessToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
