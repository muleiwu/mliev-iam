package config

import (
	"cnb.cool/mliev/iam/mliev-iam/migrations"
	"cnb.cool/mliev/open/go-web/pkg/interfaces"
	cacheAssembly "cnb.cool/mliev/open/go-web/pkg/server/cache/assembly"
	configAssembly "cnb.cool/mliev/open/go-web/pkg/server/config/assembly"
	databaseAssembly "cnb.cool/mliev/open/go-web/pkg/server/database/assembly"
	envAssembly "cnb.cool/mliev/open/go-web/pkg/server/env/assembly"
	"cnb.cool/mliev/open/go-web/pkg/server/http_server/service"
	loggerAssembly "cnb.cool/mliev/open/go-web/pkg/server/logger/assembly"
	redisAssembly "cnb.cool/mliev/open/go-web/pkg/server/redis/assembly"
)

// App 是 mliev-iam 的 AppProvider 实现。
type App struct{}

// Assemblies 声明装配顺序 env → config → logger → database → redis → cache。
func (a App) Assemblies() []interfaces.AssemblyInterface {
	return []interfaces.AssemblyInterface{
		&envAssembly.Env{},
		&configAssembly.Config{DefaultConfigs: Config{}.Get()},
		&loggerAssembly.Logger{},
		&databaseAssembly.Database{},
		&redisAssembly.Redis{},
		&cacheAssembly.Cache{},
	}
}

// Servers 声明运行服务：先跑 migration，再起 http_server。
// 注意使用我们自己的 migrations.Server 而非框架自带的 migration.Migration：
// 后者用 goose.Up(db, "migrations") 读文件系统根目录，而我们按方言把 SQL
// 放在 migrations/{mysql,postgresql,sqlite}/ 并通过 //go:embed 打包，那个
// 版本扫不到新 SQL，老库升级时会错过未应用的迁移。
func (a App) Servers() []interfaces.ServerInterface {
	return []interfaces.ServerInterface{
		&migrations.Server{},
		&service.HttpServer{},
	}
}
