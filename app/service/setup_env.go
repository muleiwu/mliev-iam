package service

import setupkit "cnb.cool/mliev/open/setup-kit"

// bindSetupEnvironment declares installation-only presets. setup-kit reads and
// validates them once, preserving user overrides and handling secret defaults.
func bindSetupEnvironment(database, redis, admin *setupkit.Step) {
	database.Connections[0].DefaultDriverEnv = "IAM_SETUP_DB_DRIVER"
	database.Connections[0].Env = map[string]string{
		"host":        "IAM_SETUP_DB_HOST",
		"port":        "IAM_SETUP_DB_PORT",
		"username":    "IAM_SETUP_DB_USERNAME",
		"password":    "IAM_SETUP_DB_PASSWORD",
		"database":    "IAM_SETUP_DB_DATABASE",
		"tls":         "IAM_SETUP_DB_TLS",
		"charset":     "IAM_SETUP_DB_CHARSET",
		"sqlite.path": "IAM_SETUP_DB_FILEPATH",
	}
	redis.Connections[0].Env = map[string]string{
		"host":     "IAM_SETUP_REDIS_HOST",
		"port":     "IAM_SETUP_REDIS_PORT",
		"username": "IAM_SETUP_REDIS_USERNAME",
		"password": "IAM_SETUP_REDIS_PASSWORD",
		"db":       "IAM_SETUP_REDIS_DB",
		"tls":      "IAM_SETUP_REDIS_TLS",
	}
	adminEnv := map[string]string{
		"admin.username": "IAM_SETUP_ADMIN_USERNAME",
		"admin.email":    "IAM_SETUP_ADMIN_EMAIL",
		"admin.realName": "IAM_SETUP_ADMIN_REAL_NAME",
		"admin.phone":    "IAM_SETUP_ADMIN_PHONE",
		"admin.password": "IAM_SETUP_ADMIN_PASSWORD",
		"admin.confirm":  "IAM_SETUP_ADMIN_PASSWORD",
	}
	for i := range admin.Fields {
		admin.Fields[i].Env = adminEnv[admin.Fields[i].Key]
	}
}
