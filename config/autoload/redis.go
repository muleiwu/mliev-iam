package autoload

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type Redis struct {
}

func (receiver Redis) InitConfig() map[string]any {
	env := helper.GetEnv()
	return map[string]any{
		"redis.host":     envOr("REDIS_HOST", env.GetString("redis.host", "localhost")),
		"redis.port":     env.GetInt("redis.port", 6379),
		"redis.password": envOr("REDIS_PASSWORD", env.GetString("redis.password", "")),
		"redis.db":       env.GetInt("redis.db", 0),
	}
}
