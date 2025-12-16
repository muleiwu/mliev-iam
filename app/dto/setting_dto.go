package dto

import "time"

// RegisterFieldConfig 单个注册字段配置
type RegisterFieldConfig struct {
	Visible     bool   `json:"visible"`               // 是否显示
	Required    bool   `json:"required"`              // 是否必填
	Label       string `json:"label"`                 // 字段标签
	Placeholder string `json:"placeholder,omitempty"` // 占位符
}

// RegisterFieldsSettings 注册字段配置集合
type RegisterFieldsSettings struct {
	Email    RegisterFieldConfig `json:"email"`
	Phone    RegisterFieldConfig `json:"phone"`
	RealName RegisterFieldConfig `json:"realName"`
	Nickname RegisterFieldConfig `json:"nickname"`
}

// SettingResponse 设置响应信息
type SettingResponse struct {
	ID        uint      `json:"id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Category  string    `json:"category"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// UpdateSettingRequest 更新设置请求
type UpdateSettingRequest struct {
	Value    string `json:"value" binding:"required"`
	Category string `json:"category" binding:"omitempty"`
	Remark   string `json:"remark" binding:"omitempty"`
}

// CreateSettingRequest 创建设置请求
type CreateSettingRequest struct {
	Key      string `json:"key" binding:"required"`
	Value    string `json:"value" binding:"required"`
	Category string `json:"category" binding:"omitempty"`
	Remark   string `json:"remark" binding:"omitempty"`
}

// UpdateRegisterFieldsRequest 更新注册字段配置请求
type UpdateRegisterFieldsRequest struct {
	Email    RegisterFieldConfig `json:"email"`
	Phone    RegisterFieldConfig `json:"phone"`
	RealName RegisterFieldConfig `json:"realName"`
	Nickname RegisterFieldConfig `json:"nickname"`
}

// PushSettings 推送服务配置
type PushSettings struct {
	BaseURL        string `json:"baseUrl"`
	AppID          string `json:"appId"`
	AppSecret      string `json:"appSecret"`
	SMSChannelID   int    `json:"smsChannelId"`
	EmailChannelID int    `json:"emailChannelId"`
	SMSSignature   string `json:"smsSignature"`
	EmailSignature string `json:"emailSignature"`
}

// UpdatePushSettingsRequest 更新推送服务配置请求
type UpdatePushSettingsRequest struct {
	BaseURL        string `json:"baseUrl"`
	AppID          string `json:"appId"`
	AppSecret      string `json:"appSecret"`
	SMSChannelID   int    `json:"smsChannelId"`
	EmailChannelID int    `json:"emailChannelId"`
	SMSSignature   string `json:"smsSignature"`
	EmailSignature string `json:"emailSignature"`
}
