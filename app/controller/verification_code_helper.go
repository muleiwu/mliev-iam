package controller

import (
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

// getVerificationCodeConfig 从数据库获取验证码服务配置。
// 配置获取失败或必填字段缺失时返回 nil（调用方视为"验证码服务未配置"）。
func getVerificationCodeConfig() *service.VerificationCodeConfig {
	settingService := service.NewSettingService(helper.GetDatabase())
	pushSettings, err := settingService.GetPushSettings()
	if err != nil {
		return nil
	}

	if pushSettings.BaseURL == "" || pushSettings.AppID == "" || pushSettings.AppSecret == "" {
		return nil
	}

	return &service.VerificationCodeConfig{
		BaseURL:               pushSettings.BaseURL,
		AppID:                 pushSettings.AppID,
		AppSecret:             pushSettings.AppSecret,
		SMSChannelID:          pushSettings.SMSChannelID,
		EmailChannelID:        pushSettings.EmailChannelID,
		SMSSignature:          pushSettings.SMSSignature,
		EmailSignature:        pushSettings.EmailSignature,
		SMSTemplateBindings:   pushSettings.SMSTemplateBindings,
		EmailTemplateBindings: pushSettings.EmailTemplateBindings,
	}
}
