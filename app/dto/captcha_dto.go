package dto

import "time"

// ============== 验证码配置 DTOs ==============

// CaptchaTicket 验证码凭证（业务接口中使用）
type CaptchaTicket struct {
	Provider string            `json:"provider"` // 提供商类型
	Data     map[string]string `json:"data"`     // 凭证数据（各提供商字段不同）
}

// CaptchaConfigResponse 验证码配置响应
type CaptchaConfigResponse struct {
	ID        uint              `json:"id"`
	Name      string            `json:"name"`
	Provider  string            `json:"provider"`
	Enabled   bool              `json:"enabled"`
	Params    map[string]string `json:"params"`
	Remark    string            `json:"remark"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// CreateCaptchaConfigRequest 创建验证码配置请求
type CreateCaptchaConfigRequest struct {
	Name     string            `json:"name" binding:"required" validate:"max=100"`
	Provider string            `json:"provider" binding:"required" validate:"max=50"`
	Enabled  bool              `json:"enabled"`
	Params   map[string]string `json:"params" binding:"required"`
	Remark   string            `json:"remark" validate:"max=255"`
}

// UpdateCaptchaConfigRequest 更新验证码配置请求
type UpdateCaptchaConfigRequest struct {
	Name     string            `json:"name" binding:"required" validate:"max=100"`
	Provider string            `json:"provider" binding:"required" validate:"max=50"`
	Enabled  bool              `json:"enabled"`
	Params   map[string]string `json:"params" binding:"required"`
	Remark   string            `json:"remark" validate:"max=255"`
}

// CaptchaProviderField 提供商参数字段定义
type CaptchaProviderField struct {
	Key         string `json:"key"`         // 参数键名
	Label       string `json:"label"`       // 显示名称
	Type        string `json:"type"`        // 字段类型: text, password
	Required    bool   `json:"required"`    // 是否必填
	Placeholder string `json:"placeholder"` // 占位提示
}

// CaptchaProviderInfo 验证码提供商信息
type CaptchaProviderInfo struct {
	Provider string                 `json:"provider"` // 提供商标识
	Label    string                 `json:"label"`    // 显示名称
	Fields   []CaptchaProviderField `json:"fields"`   // 参数字段定义
}

// CaptchaSceneBindings 场景绑定配置
type CaptchaSceneBindings struct {
	Login         uint `json:"login"`         // 登录场景配置ID (0表示不启用)
	Register      uint `json:"register"`      // 注册场景配置ID (0表示不启用)
	ResetPassword uint `json:"resetPassword"` // 找回密码场景配置ID (0表示不启用)
	SendSms       uint `json:"sendSms"`       // 发送短信场景配置ID (0表示不启用)
	SendEmail     uint `json:"sendEmail"`     // 发送邮件场景配置ID (0表示不启用)
}

// UpdateCaptchaSceneBindingsRequest 更新场景绑定请求
type UpdateCaptchaSceneBindingsRequest struct {
	Login         uint `json:"login"`
	Register      uint `json:"register"`
	ResetPassword uint `json:"resetPassword"`
	SendSms       uint `json:"sendSms"`
	SendEmail     uint `json:"sendEmail"`
}

// CaptchaClientParamsResponse 前端初始化参数响应
type CaptchaClientParamsResponse struct {
	Enabled  bool                   `json:"enabled"`  // 是否启用验证码
	Provider string                 `json:"provider"` // 提供商类型
	Params   map[string]interface{} `json:"params"`   // 前端初始化参数
}
