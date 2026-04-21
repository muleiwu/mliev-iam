package autoload

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type Cache struct {
}

func (receiver Cache) InitConfig() map[string]any {
	env := helper.GetEnv()
	return map[string]any{
		"cache.driver": env.GetString("cache.driver", "redis"), // memory,redis,none
	}
}
