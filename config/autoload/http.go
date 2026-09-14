package autoload

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"slices"
)

type Http struct {
	DefaultAddr     string
	ExtraStaticDirs []string
}

func (receiver Http) InitConfig() map[string]any {
	env := helper.GetEnv()
	mode := env.GetString("app.mode", "release")
	defaultStaticMode := "embed"
	if mode == "debug" {
		defaultStaticMode = "disk"
	}
	defaultAddr := receiver.DefaultAddr
	if defaultAddr == "" {
		defaultAddr = ":8080"
	}
	staticDirs := env.GetStringSlice("http.static_dir", []string{"admin", "auth", "install", "static"})
	for _, dir := range receiver.ExtraStaticDirs {
		if !slices.Contains(staticDirs, dir) {
			staticDirs = append(staticDirs, dir)
		}
	}
	return map[string]any{
		"http.addr":        env.GetString("http.addr", defaultAddr),
		"http.mode":        mode,
		"http.load_static": env.GetBool("http.load_static", true),
		"http.static_mode": env.GetString("http.static_mode", defaultStaticMode),
		"http.static_dir":  staticDirs,
	}
}
