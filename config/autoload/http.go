package autoload

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type Http struct {
}

func (receiver Http) InitConfig() map[string]any {
	env := helper.GetEnv()
	mode := env.GetString("app.mode", "release")
	defaultStaticMode := "embed"
	if mode == "debug" {
		defaultStaticMode = "disk"
	}
	return map[string]any{
		"http.addr":        env.GetString("http.addr", ":8080"),
		"http.mode":        mode,
		"http.load_static": env.GetBool("http.load_static", true),
		"http.static_mode": env.GetString("http.static_mode", defaultStaticMode),
		"http.static_dir":  env.GetStringSlice("http.static_dir", []string{"admin", "auth", "install", "static"}),
	}
}
