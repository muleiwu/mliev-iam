package config

import (
	appService "cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/pkg/extension"
	"cnb.cool/mliev/open/go-web/pkg/container"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"cnb.cool/mliev/open/go-web/pkg/interfaces"
	cacheAssembly "cnb.cool/mliev/open/go-web/pkg/server/cache/assembly"
	configAssembly "cnb.cool/mliev/open/go-web/pkg/server/config/assembly"
	envAssembly "cnb.cool/mliev/open/go-web/pkg/server/env/assembly"
	"cnb.cool/mliev/open/go-web/pkg/server/http_server/service"
	loggerAssembly "cnb.cool/mliev/open/go-web/pkg/server/logger/assembly"
)

// App 是 mliev-iam 的 AppProvider 实现。
type App struct {
	Modules         []extension.Module
	Edition         string
	Version         string
	DefaultHTTPAddr string
	// Optional installation branding and final destination. Empty values retain
	// the edition's standard name and the /admin/ destination.
	SetupName      string
	SetupEntryPath string
}

// Assemblies 声明装配顺序 env → config → logger → database → redis → cache。
func (a App) Assemblies() []interfaces.AssemblyInterface {
	return []interfaces.AssemblyInterface{
		&envAssembly.Env{},
		&configAssembly.Config{DefaultConfigs: Config{Modules: a.Modules, Edition: a.Edition, Version: a.Version, DefaultHTTPAddr: a.DefaultHTTPAddr}.Get()},
		&loggerAssembly.Logger{},
		&bootstrapDatabase{},
		&bootstrapRedis{},
		&cacheAssembly.Cache{},
		&startupChecks{modules: a.Modules},
		&setupAssembly{edition: a.Edition, options: appService.SetupOptions{Name: a.SetupName, EntryPath: a.SetupEntryPath}},
	}
}

// Servers starts HTTP alone during installation. On an installed deployment,
// the final assembly has already completed migrations and extension checks.
func (a App) Servers() []interfaces.ServerInterface {
	if !helper.GetConfig().GetBool("app.installed", false) {
		return []interfaces.ServerInterface{container.MustGet[*appService.SetupInstaller](), &service.HttpServer{}}
	}
	return []interfaces.ServerInterface{
		container.MustGet[*appService.SetupInstaller](),
		&appService.SyncWorkerServer{},
		&appService.LoginSessionCleanupServer{},
		&service.HttpServer{},
	}
}
