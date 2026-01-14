package captcha

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// CaptchaProvider 验证码提供商接口
// 不同的验证码服务商需要实现此接口
type CaptchaProvider interface {
	// GetProviderType 返回提供商类型标识
	GetProviderType() string

	// GetProviderInfo 获取提供商信息及配置表单定义
	// 返回提供商的元信息，包括名称和配置字段定义
	GetProviderInfo() dto.CaptchaProviderInfo

	// GetClientParams 获取前端初始化需要的参数
	// 返回给前端用于初始化验证码组件
	// 例如：腾讯云返回 {"captchaAppId": "xxx"}
	GetClientParams(config *model.CaptchaConfig) map[string]interface{}

	// Verify 验证前端提交的ticket
	// config: 验证码配置
	// ticketData: 前端验证成功后返回的凭证数据（各提供商字段不同）
	// userIP: 用户客户端IP地址
	// 返回: 验证结果
	Verify(config *model.CaptchaConfig, ticketData map[string]string, userIP string) (*VerifyResult, error)
}

// VerifyResult 验证结果
type VerifyResult struct {
	Success bool   `json:"success"` // 是否验证成功
	Message string `json:"message"` // 错误消息（验证失败时）
}
