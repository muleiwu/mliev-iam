package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// AttachmentDao 附件数据访问对象
type AttachmentDao struct {
	db *gorm.DB
}

// NewAttachmentDao 创建附件DAO实例
func NewAttachmentDao(db *gorm.DB) *AttachmentDao {
	return &AttachmentDao{db: db}
}

// Create 创建附件记录
func (d *AttachmentDao) Create(attachment *model.Attachment) error {
	return d.db.Create(attachment).Error
}

// GetByUUID 根据UUID获取附件
func (d *AttachmentDao) GetByUUID(uuid string) (*model.Attachment, error) {
	var attachment model.Attachment
	if err := d.db.Where("uuid = ?", uuid).First(&attachment).Error; err != nil {
		return nil, err
	}
	return &attachment, nil
}
