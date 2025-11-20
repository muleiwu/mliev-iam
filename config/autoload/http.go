package autoload

import envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"

type Http struct {
}

func (receiver Http) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"http.addr":        ":8080",
		"http.load_static": true,
		"http.static_dir":  []string{"admin", "auth", "install", "static"},
	}
}
