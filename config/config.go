package config

import (
	"cnb.cool/mliev/iam/mliev-iam/config/autoload"
	"cnb.cool/mliev/iam/mliev-iam/pkg/extension"
	"cnb.cool/mliev/iam/mliev-iam/pkg/firstparty"
	"cnb.cool/mliev/open/go-web/pkg/interfaces"
	http "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"context"
)

type Config struct {
	Modules         []extension.Module
	Edition         string
	Version         string
	DefaultHTTPAddr string
}

func (receiver Config) Get() []interfaces.InitConfig {
	var routes []func(http.RouterInterface)
	var middleware []http.HandlerFunc
	var staticDirs []string
	var identifiers []extension.LoginIdentifier
	for _, module := range receiver.Modules {
		identifiers = append(identifiers, module.LoginIdentifiers...)
		staticDirs = append(staticDirs, module.StaticDirs...)
		for _, client := range module.FirstPartyClients {
			firstparty.Register(client)
		}
		if module.Routes != nil {
			routes = append(routes, module.Routes)
		}
		middleware = append(middleware, module.Middleware...)
	}
	extension.ConfigureLoginIdentifiers(identifiers)
	capabilities := func(ctx context.Context) extension.Capabilities {
		edition, version := receiver.Edition, receiver.Version
		if edition == "" {
			edition = "community"
		}
		if version == "" {
			version = "1.0.0"
		}
		result := extension.Capabilities{Edition: edition, Version: version, Features: map[string]bool{}}
		for _, module := range receiver.Modules {
			if module.Capabilities != nil {
				for key, value := range module.Capabilities(ctx) {
					result.Features[key] = value
				}
			}
		}
		return result
	}
	return []interfaces.InitConfig{
		autoload.Base{},
		autoload.Cache{},
		autoload.Http{DefaultAddr: receiver.DefaultHTTPAddr, ExtraStaticDirs: staticDirs},
		autoload.Cors{},
		autoload.StaticFs{},
		autoload.Database{},
		autoload.DatabaseSafety{},
		autoload.Redis{},
		autoload.Push{},
		autoload.Middleware{Extra: middleware},
		autoload.Router{ExtraRoutes: routes, Capabilities: capabilities},
	}
}
