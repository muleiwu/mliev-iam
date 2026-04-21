package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// CaptchaConfig 验证码配置模型
// 用于存储验证码服务提供商的配置信息
type CaptchaConfig struct {
	ID        uint              `gorm:"primaryKey" json:"id"`
	Name      string            `gorm:"size:100;not null" json:"name" validate:"required,max=100"`         // 配置名称
	Provider  string            `gorm:"size:50;not null;index" json:"provider" validate:"required,max=50"` // 提供商类型: tencent, geetest 等
	Enabled   bool              `gorm:"default:true" json:"enabled"`                                       // 是否启用
	Params    datatypes.JSONMap `gorm:"type:json" json:"params"`                                           // 参数配置(JSON格式)
	Remark    string            `gorm:"size:255" json:"remark" validate:"max=255"`                         // 备注
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	DeletedAt gorm.DeletedAt    `gorm:"index" json:"-"`
}

// TableName returns the table name for CaptchaConfig model.
func (*CaptchaConfig) TableName() string {
	return "captcha_configs"
}
