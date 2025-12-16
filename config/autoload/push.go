package autoload

import envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"

// Push 推送服务配置
type Push struct {
}

func (receiver Push) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"push.baseUrl":        helper.GetEnv().GetString("push.baseUrl", "https://push.mliev.com"),
		"push.appId":          helper.GetEnv().GetString("push.appId", ""),
		"push.appSecret":      helper.GetEnv().GetString("push.appSecret", ""),
		"push.smsChannelId":   helper.GetEnv().GetInt("push.smsChannelId", 1),
		"push.emailChannelId": helper.GetEnv().GetInt("push.emailChannelId", 2),
		"push.smsSignature":   helper.GetEnv().GetString("push.smsSignature", ""),
		"push.emailSignature": helper.GetEnv().GetString("push.emailSignature", ""),
	}
}
