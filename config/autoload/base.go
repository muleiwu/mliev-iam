package autoload

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type Base struct {
}

func (receiver Base) InitConfig() map[string]any {
	env := helper.GetEnv()
	return map[string]any{
		"app.name":      env.GetString("app.app_name", "mliev-iam"),
		"app.installed": env.GetBool("app.installed", false),
		"app.mode":      env.GetString("app.mode", "release"),
	}
}
