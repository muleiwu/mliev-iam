package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// OIDCClientDAO OIDC客户端数据访问对象
type OIDCClientDAO struct {
	db *gorm.DB
}

// NewOIDCClientDAO 创建新的OIDC客户端DAO
func NewOIDCClientDAO(db *gorm.DB) *OIDCClientDAO {
	return &OIDCClientDAO{db: db}
}

// Create 创建OIDC客户端
func (dao *OIDCClientDAO) Create(client *model.OIDCClient) error {
	return dao.db.Create(client).Error
}

// GetByID 根据ID获取OIDC客户端
func (dao *OIDCClientDAO) GetByID(id uint) (*model.OIDCClient, error) {
	var client model.OIDCClient
	err := dao.db.First(&client, id).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// GetByClientID 根据ClientID获取OIDC客户端
func (dao *OIDCClientDAO) GetByClientID(clientID string) (*model.OIDCClient, error) {
	var client model.OIDCClient
	err := dao.db.Where("client_id = ?", clientID).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// Update 更新OIDC客户端
func (dao *OIDCClientDAO) Update(client *model.OIDCClient) error {
	return dao.db.Save(client).Error
}

// Delete 删除OIDC客户端
func (dao *OIDCClientDAO) Delete(id uint) error {
	return dao.db.Delete(&model.OIDCClient{}, id).Error
}

// List 获取OIDC客户端列表
func (dao *OIDCClientDAO) List(offset, limit int, isActive *bool) ([]*model.OIDCClient, int64, error) {
	var clients []*model.OIDCClient
	var total int64

	query := dao.db.Model(&model.OIDCClient{})
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	if err := query.Offset(offset).Limit(limit).Find(&clients).Error; err != nil {
		return nil, 0, err
	}

	return clients, total, nil
}

// ListByCreator 根据创建者获取OIDC客户端列表
func (dao *OIDCClientDAO) ListByCreator(creatorID uint, offset, limit int) ([]*model.OIDCClient, int64, error) {
	var clients []*model.OIDCClient
	var total int64

	query := dao.db.Model(&model.OIDCClient{}).Where("created_by_user_id = ?", creatorID)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	if err := query.Offset(offset).Limit(limit).Find(&clients).Error; err != nil {
		return nil, 0, err
	}

	return clients, total, nil
}

// CheckClientIDExists 检查ClientID是否已存在
func (dao *OIDCClientDAO) CheckClientIDExists(clientID string, excludeID *uint) (bool, error) {
	var count int64
	query := dao.db.Model(&model.OIDCClient{}).Where("client_id = ?", clientID)
	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}

	err := query.Count(&count).Error
	return count > 0, err
}

// UpdateStatus 更新客户端状态
func (dao *OIDCClientDAO) UpdateStatus(id uint, isActive bool) error {
	return dao.db.Model(&model.OIDCClient{}).Where("id = ?", id).Update("is_active", isActive).Error
}

// Verify 验证客户端
func (dao *OIDCClientDAO) Verify(id uint) error {
	return dao.db.Model(&model.OIDCClient{}).Where("id = ?", id).Update("is_verified", true).Error
}

// BatchDelete 批量删除客户端
func (dao *OIDCClientDAO) BatchDelete(ids []uint) error {
	return dao.db.Delete(&model.OIDCClient{}, ids).Error
}
