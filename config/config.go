package config

import (
	"cnb.cool/mliev/iam/mliev-iam/config/autoload"
	"cnb.cool/mliev/open/go-web/pkg/interfaces"
)

type Config struct {
}

func (receiver Config) Get() []interfaces.InitConfig {
	return []interfaces.InitConfig{
		autoload.Base{},
		autoload.Cache{},
		autoload.Http{},
		autoload.Cors{},
		autoload.StaticFs{},
		autoload.Database{},
		autoload.DatabaseSafety{},
		autoload.Redis{},
		autoload.Push{},
		autoload.Migration{},
		autoload.Middleware{},
		autoload.Router{},
	}
}
