package model

import (
	"time"

	"gorm.io/gorm"
)

// Setting 系统设置模型
// 用于存储系统配置，采用 key-value 形式存储
type Setting struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Key       string         `gorm:"column:setting_key;uniqueIndex;size:100;not null" json:"key" validate:"required,max=100"`
	Value     string         `gorm:"type:text" json:"value"`
	Category  string         `gorm:"size:50;index" json:"category" validate:"max=50"` // 分类，如 "register", "security"
	Remark    string         `gorm:"size:255" json:"remark" validate:"max=255"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the table name for Setting model.
func (*Setting) TableName() string {
	return "settings"
}
