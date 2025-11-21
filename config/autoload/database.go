package autoload

import envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"

type Database struct {
}

func (receiver Database) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"database.driver":   helper.GetEnv().GetString("database.driver", "mysql"),
		"database.host":     helper.GetEnv().GetString("database.host", "127.0.0.1"),
		"database.filepath": helper.GetEnv().GetString("database.filepath", "./config/mliev_iam.db"),
		"database.port":     helper.GetEnv().GetInt("database.port", 3306),
		"database.database": helper.GetEnv().GetString("database.database", "test"),
		"database.username": helper.GetEnv().GetString("database.username", "test"),
		"database.password": helper.GetEnv().GetString("database.password", "123456"),
	}
}
