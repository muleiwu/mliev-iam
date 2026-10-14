package autoload

import "cnb.cool/mliev/open/go-web/pkg/helper"

type Passkey struct{}

func (Passkey) InitConfig() map[string]any {
	env := helper.GetEnv()
	return map[string]any{
		"passkey.enabled":         env.GetBool("passkey.enabled", false),
		"passkey.rp_id":           env.GetString("passkey.rp_id", ""),
		"passkey.rp_origins":      env.GetStringSlice("passkey.rp_origins", []string{}),
		"passkey.rp_display_name": env.GetString("passkey.rp_display_name", "MLiev IAM"),
	}
}
