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
	Username RegisterFieldConfig `json:"username"`
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
	Username RegisterFieldConfig `json:"username"`
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

// StorageSettings 对象存储（S3 兼容）配置
type StorageSettings struct {
	Enabled     bool   `json:"enabled"`
	Endpoint    string `json:"endpoint"`    // S3 兼容 endpoint，留空表示 AWS 官方
	Region      string `json:"region"`
	Bucket      string `json:"bucket"`
	AccessKey   string `json:"accessKey"`
	SecretKey   string `json:"secretKey"`
	CDN         string `json:"cdn"`         // CDN 加速域名，留空则 302 跳转到预签名地址
	UploadPath  string `json:"uploadPath"`  // 存储 key 前缀，如 iam/avatar
	MaxFileSize int64  `json:"maxFileSize"` // 单文件大小上限（MB）
	PublicRead  bool   `json:"publicRead"`  // 上传时设置 public-read ACL
}

// UpdateStorageSettingsRequest 更新对象存储配置请求
type UpdateStorageSettingsRequest struct {
	Enabled     bool   `json:"enabled"`
	Endpoint    string `json:"endpoint"`
	Region      string `json:"region"`
	Bucket      string `json:"bucket"`
	AccessKey   string `json:"accessKey"`
	SecretKey   string `json:"secretKey"`
	CDN         string `json:"cdn"`
	UploadPath  string `json:"uploadPath"`
	MaxFileSize int64  `json:"maxFileSize"`
	PublicRead  bool   `json:"publicRead"`
}
