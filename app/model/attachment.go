package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	AttachmentStorageTypeS3          = "s3"
	AttachmentBizTypeAvatar          = "avatar"
	AttachmentBizTypeApplicationIcon = "application_icon"
)

// Attachment 上传到对象存储的附件记录。
// 对外仅暴露 UUID（/attachments/{uuid} 302 跳转到真实存储地址），不暴露自增 ID。
type Attachment struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	UUID           string         `gorm:"size:36;not null;uniqueIndex:uk_attachments_uuid" json:"uuid"`
	FileName       string         `gorm:"size:255;not null" json:"fileName"`
	FileSize       int64          `gorm:"not null;default:0" json:"fileSize"`
	MimeType       string         `gorm:"size:100;not null" json:"mimeType"`
	StorageType    string         `gorm:"size:20;not null;default:s3" json:"storageType"`
	StorageKey     string         `gorm:"size:500;not null" json:"-"`
	BizType        string         `gorm:"size:50;not null;default:avatar" json:"bizType"`
	UploaderUserID uint           `gorm:"not null;default:0;index" json:"uploaderUserId"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the table name for Attachment.
func (*Attachment) TableName() string {
	return "attachments"
}
