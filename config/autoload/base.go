package autoload

import envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"

type Base struct {
}

func (receiver Base) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"app.base.app_name": helper.GetEnv().GetString("app.base.app_name", "go-web-app"),
		"mode":              helper.GetEnv().GetString("mode", "release"),
	}
}
