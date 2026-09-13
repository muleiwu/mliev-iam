package autoload

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type Cache struct {
}

func (receiver Cache) InitConfig() map[string]any {
	env := helper.GetEnv()
	driver := env.GetString("cache.driver", "redis")
	if !env.GetBool("app.installed", false) {
		driver = "memory"
	}
	return map[string]any{
		"cache.driver": driver, // memory,redis,none
	}
}
