package autoload

import (
	"os"

	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type Database struct {
}

// envOr 返回环境变量值；若未设置或为空则回落到 fallback。
// 用于把机密（DB_PASSWORD 等）从 config.yaml 中移出，优先来自运行环境。
func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func (receiver Database) InitConfig() map[string]any {
	env := helper.GetEnv()
	driver := envOr("DB_DRIVER", env.GetString("database.driver", "mysql"))
	// Preserve MariaDB in the saved configuration; runtime consumers use MySQL.
	if driver == "mariadb" {
		driver = "mysql"
	}

	// sqlite 特例：go-web 的 sqlite 驱动把 DatabaseConfig.Host 当作 DSN（文件路径），
	// 所以这里把 database.filepath 映射到 database.host，避免误用 IP/localhost 作为文件名。
	host := envOr("DB_HOST", env.GetString("database.host", "127.0.0.1"))
	if driver == "sqlite" {
		host = envOr("DB_FILEPATH", env.GetString("database.filepath", "./config/mliev_iam.db"))
	}

	if !env.GetBool("app.installed", false) {
		driver = "memory"
	}
	return map[string]any{
		"database.driver":                 driver,
		"database.host":                   host,
		"database.filepath":               envOr("DB_FILEPATH", env.GetString("database.filepath", "./config/mliev_iam.db")),
		"database.port":                   env.GetInt("database.port", 3306),
		"database.dbname":                 envOr("DB_DATABASE", env.GetString("database.database", "test")),
		"database.username":               envOr("DB_USERNAME", env.GetString("database.username", "test")),
		"database.password":               envOr("DB_PASSWORD", env.GetString("database.password", "123456")),
		"database.charset":                envOr("DB_CHARSET", env.GetString("database.charset", "utf8mb4")),
		"database.tls":                    env.GetBool("database.tls", false),
		"database.prefer_simple_protocol": env.GetBool("database.prefer_simple_protocol", true),
	}
}
