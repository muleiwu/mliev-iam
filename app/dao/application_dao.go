package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// ApplicationDAO 应用数据访问对象
type ApplicationDAO struct {
	db *gorm.DB
}

// NewApplicationDAO 创建新的应用DAO
func NewApplicationDAO(db *gorm.DB) *ApplicationDAO {
	return &ApplicationDAO{db: db}
}

// Create 创建应用
func (dao *ApplicationDAO) Create(app *model.Application) error {
	return dao.db.Create(app).Error
}

// GetByID 根据ID获取应用
func (dao *ApplicationDAO) GetByID(id uint) (*model.Application, error) {
	var app model.Application
	err := dao.db.First(&app, id).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// GetByUUID 根据UUID获取应用
func (dao *ApplicationDAO) GetByUUID(uuid string) (*model.Application, error) {
	var app model.Application
	err := dao.db.Where("uuid = ?", uuid).First(&app).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// GetByClientID 根据ClientID获取应用（用于OIDC和OAuth2协议）
// 优化：直接使用uuid字段查询，而非JSON提取
func (dao *ApplicationDAO) GetByClientID(clientID string) (*model.Application, error) {
	var app model.Application
	// 使用uuid字段进行高效查询（uuid即为clientId）
	err := dao.db.Where("uuid = ? AND protocol_type IN (?)", clientID, []string{"oidc", "oauth2"}).First(&app).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// Update 更新应用
func (dao *ApplicationDAO) Update(app *model.Application) error {
	return dao.db.Save(app).Error
}

// Delete 删除应用
func (dao *ApplicationDAO) Delete(id uint) error {
	return dao.db.Delete(&model.Application{}, id).Error
}

// List 获取应用列表
func (dao *ApplicationDAO) List(offset, limit int, protocolType *string, isActive *bool) ([]*model.Application, int64, error) {
	var apps []*model.Application
	var total int64

	query := dao.db.Model(&model.Application{})

	// 按协议类型过滤
	if protocolType != nil && *protocolType != "" {
		query = query.Where("protocol_type = ?", *protocolType)
	}

	// 按状态过滤
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&apps).Error; err != nil {
		return nil, 0, err
	}

	return apps, total, nil
}

// ListByCreator 根据创建者获取应用列表
func (dao *ApplicationDAO) ListByCreator(creatorID uint, offset, limit int) ([]*model.Application, int64, error) {
	var apps []*model.Application
	var total int64

	query := dao.db.Model(&model.Application{}).Where("created_by_user_id = ?", creatorID)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&apps).Error; err != nil {
		return nil, 0, err
	}

	return apps, total, nil
}

// ListByProtocolType 根据协议类型获取应用列表
func (dao *ApplicationDAO) ListByProtocolType(protocolType string, offset, limit int) ([]*model.Application, int64, error) {
	var apps []*model.Application
	var total int64

	query := dao.db.Model(&model.Application{}).Where("protocol_type = ?", protocolType)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&apps).Error; err != nil {
		return nil, 0, err
	}

	return apps, total, nil
}

// UpdateStatus 更新应用状态
func (dao *ApplicationDAO) UpdateStatus(id uint, isActive bool) error {
	return dao.db.Model(&model.Application{}).Where("id = ?", id).Update("is_active", isActive).Error
}

// Verify 验证应用
func (dao *ApplicationDAO) Verify(id uint) error {
	return dao.db.Model(&model.Application{}).Where("id = ?", id).Update("is_verified", true).Error
}

// BatchDelete 批量删除应用
func (dao *ApplicationDAO) BatchDelete(ids []uint) error {
	return dao.db.Delete(&model.Application{}, ids).Error
}

// CountByProtocolType 统计指定协议类型的应用数量
func (dao *ApplicationDAO) CountByProtocolType(protocolType string) (int64, error) {
	var count int64
	err := dao.db.Model(&model.Application{}).Where("protocol_type = ?", protocolType).Count(&count).Error
	return count, err
}

// CountActive 统计活跃应用数量
func (dao *ApplicationDAO) CountActive() (int64, error) {
	var count int64
	err := dao.db.Model(&model.Application{}).Where("is_active = ?", true).Count(&count).Error
	return count, err
}

// CountInactive 统计非活跃应用数量
func (dao *ApplicationDAO) CountInactive() (int64, error) {
	var count int64
	err := dao.db.Model(&model.Application{}).Where("is_active = ?", false).Count(&count).Error
	return count, err
}

// UpdateClientSecret 更新 OIDC 应用的客户端密钥
func (dao *ApplicationDAO) UpdateClientSecret(appID uint, hashedSecret string) error {
	// 获取现有应用
	app, err := dao.GetByID(appID)
	if err != nil {
		return err
	}

	// 更新协议配置中的 clientSecret
	if app.ProtocolConfig == nil {
		app.ProtocolConfig = make(map[string]interface{})
	}
	app.ProtocolConfig["clientSecret"] = hashedSecret

	// 保存更新
	return dao.Update(app)
}
