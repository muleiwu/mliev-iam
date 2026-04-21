package autoload

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

// Push 推送服务配置
type Push struct {
}

func (receiver Push) InitConfig() map[string]any {
	env := helper.GetEnv()
	return map[string]any{
		"push.baseUrl":        env.GetString("push.baseUrl", "https://push.mliev.com"),
		"push.appId":          env.GetString("push.appId", ""),
		"push.appSecret":      env.GetString("push.appSecret", ""),
		"push.smsChannelId":   env.GetInt("push.smsChannelId", 1),
		"push.emailChannelId": env.GetInt("push.emailChannelId", 2),
		"push.smsSignature":   env.GetString("push.smsSignature", ""),
		"push.emailSignature": env.GetString("push.emailSignature", ""),
	}
}
