package autoload

import envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"

type Base struct {
}

func (receiver Base) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"app.name":      helper.GetEnv().GetString("app.app_name", "mliev-iam"),
		"app.installed": helper.GetEnv().GetBool("app.installed", false),
		"mode":          helper.GetEnv().GetString("mode", "release"),
	}
}
