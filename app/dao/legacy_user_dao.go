package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// LegacyUserDao 旧用户（单独旧表）数据访问。
type LegacyUserDao struct {
	db *gorm.DB
}

func NewLegacyUserDao(db *gorm.DB) *LegacyUserDao {
	return &LegacyUserDao{db: db}
}

func (dao *LegacyUserDao) Create(u *model.LegacyUser) error {
	return dao.db.Create(u).Error
}

// Upsert 按 (source_id, username) 写入或更新（导入幂等）。
func (dao *LegacyUserDao) Upsert(u *model.LegacyUser) error {
	var existing model.LegacyUser
	err := dao.db.Where("source_id = ? AND username = ?", u.SourceID, u.Username).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return dao.db.Create(u).Error
	}
	if err != nil {
		return err
	}
	u.ID = existing.ID
	u.CreatedAt = existing.CreatedAt
	return dao.db.Save(u).Error
}

func (dao *LegacyUserDao) GetByID(id uint) (*model.LegacyUser, error) {
	var u model.LegacyUser
	if err := dao.db.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetPendingByUsername 取某源下未迁移的旧用户（登录钩子用）。
func (dao *LegacyUserDao) GetPendingByUsername(sourceID uint, username string) (*model.LegacyUser, error) {
	var u model.LegacyUser
	err := dao.db.Where("source_id = ? AND username = ? AND status <> ?",
		sourceID, username, model.LegacyUserStatusMigrated).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetPendingByIdentifier 取某源下未迁移、且 username/email/phone 任一匹配的旧用户。
func (dao *LegacyUserDao) GetPendingByIdentifier(sourceID uint, identifier string) (*model.LegacyUser, error) {
	var u model.LegacyUser
	err := dao.db.Where("source_id = ? AND status <> ?", sourceID, model.LegacyUserStatusMigrated).
		Where("username = ? OR email = ? OR phone = ?", identifier, identifier, identifier).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (dao *LegacyUserDao) UpdateFields(id uint, fields map[string]interface{}) error {
	return dao.db.Model(&model.LegacyUser{}).Where("id = ?", id).Updates(fields).Error
}

// LegacyUserQuery 旧用户列表查询条件。
type LegacyUserQuery struct {
	SourceID uint
	Username string
	Status   string
	Page     int
	PageSize int
}

func (dao *LegacyUserDao) List(q *LegacyUserQuery) ([]model.LegacyUser, int64, error) {
	var users []model.LegacyUser
	var total int64

	query := dao.db.Model(&model.LegacyUser{})
	if q.SourceID > 0 {
		query = query.Where("source_id = ?", q.SourceID)
	}
	if q.Username != "" {
		query = query.Where("username LIKE ?", "%"+q.Username+"%")
	}
	if q.Status != "" {
		query = query.Where("status = ?", q.Status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (dao *LegacyUserDao) CountBySource(sourceID uint) (int64, error) {
	var count int64
	err := dao.db.Model(&model.LegacyUser{}).Where("source_id = ?", sourceID).Count(&count).Error
	return count, err
}
