package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// AgreementDAO 协议数据访问对象。
type AgreementDAO struct {
	db *gorm.DB
}

// NewAgreementDAO 创建协议 DAO。
func NewAgreementDAO(db *gorm.DB) *AgreementDAO {
	return &AgreementDAO{db: db}
}

func (dao *AgreementDAO) Create(agreement *model.Agreement) error {
	return dao.db.Create(agreement).Error
}

func (dao *AgreementDAO) Update(agreement *model.Agreement) error {
	return dao.db.Save(agreement).Error
}

func (dao *AgreementDAO) Delete(agreement *model.Agreement) error {
	return dao.db.Delete(agreement).Error
}

func (dao *AgreementDAO) GetByID(id uint) (*model.Agreement, error) {
	var agreement model.Agreement
	err := dao.db.First(&agreement, id).Error
	if err != nil {
		return nil, err
	}
	return &agreement, nil
}

func (dao *AgreementDAO) GetByScopeCode(scope string, applicationID uint, code string) (*model.Agreement, error) {
	var agreement model.Agreement
	err := dao.db.
		Where("scope = ? AND application_id = ? AND code = ?", scope, applicationID, code).
		First(&agreement).Error
	if err != nil {
		return nil, err
	}
	return &agreement, nil
}

func (dao *AgreementDAO) ListByScope(scope string, applicationID uint, includeDisabled bool) ([]*model.Agreement, error) {
	var agreements []*model.Agreement
	query := dao.db.Where("scope = ? AND application_id = ?", scope, applicationID)
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("sort ASC, id ASC").Find(&agreements).Error
	return agreements, err
}

func (dao *AgreementDAO) ExistsByScopeCodeExceptID(scope string, applicationID uint, code string, excludeID uint) (bool, error) {
	var count int64
	query := dao.db.Model(&model.Agreement{}).
		Where("scope = ? AND application_id = ? AND code = ?", scope, applicationID, code)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// AgreementAcceptanceDAO 协议同意记录数据访问对象。
type AgreementAcceptanceDAO struct {
	db *gorm.DB
}

// NewAgreementAcceptanceDAO 创建同意记录 DAO。
func NewAgreementAcceptanceDAO(db *gorm.DB) *AgreementAcceptanceDAO {
	return &AgreementAcceptanceDAO{db: db}
}

func (dao *AgreementAcceptanceDAO) CreateBatch(records []model.AgreementAcceptance) error {
	if len(records) == 0 {
		return nil
	}
	return dao.db.Create(&records).Error
}
