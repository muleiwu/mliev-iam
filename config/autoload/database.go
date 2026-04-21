package autoload

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type Database struct {
}

func (receiver Database) InitConfig() map[string]any {
	env := helper.GetEnv()
	driver := env.GetString("database.driver", "mysql")

	// sqlite 特例：go-web 的 sqlite 驱动把 DatabaseConfig.Host 当作 DSN（文件路径），
	// 所以这里把 database.filepath 映射到 database.host，避免误用 IP/localhost 作为文件名。
	host := env.GetString("database.host", "127.0.0.1")
	if driver == "sqlite" {
		host = env.GetString("database.filepath", "./config/mliev_iam.db")
	}

	return map[string]any{
		"database.driver":   driver,
		"database.host":     host,
		"database.filepath": env.GetString("database.filepath", "./config/mliev_iam.db"),
		"database.port":     env.GetInt("database.port", 3306),
		"database.dbname":   env.GetString("database.database", "test"),
		"database.username": env.GetString("database.username", "test"),
		"database.password": env.GetString("database.password", "123456"),
		"database.charset":  env.GetString("database.charset", "utf8mb4"),
	}
}
