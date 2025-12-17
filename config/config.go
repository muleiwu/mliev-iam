package config

import (
	"cnb.cool/mliev/iam/mliev-iam/config/autoload"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
)

type Config struct {
}

func (receiver Config) Get() []interfaces.InitConfig {
	return []interfaces.InitConfig{
		autoload.Base{},
		autoload.Http{},
		autoload.StaticFs{},
		autoload.Database{},
		autoload.Redis{},
		autoload.Push{},
		autoload.Middleware{},
		autoload.Router{},
		autoload.Migration{},
	}
}
