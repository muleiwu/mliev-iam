package autoload

import envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"

// DatabaseSafety 数据库安全配置
//
//	database.auto_migrate —— 是否允许运行时 AutoMigrate（默认 release=false, debug=true）
//	  为了避免生产环境因模型变更被 GORM 静默改表，release 模式默认禁用；
//	  新增表/字段请用 goose 迁移文件（migrations/YYYYMMDDHHMMSS_xxx.go）。
type DatabaseSafety struct{}

func (receiver DatabaseSafety) InitConfig(helper envInterface.HelperInterface) map[string]any {
	env := helper.GetEnv()
	mode := env.GetString("mode", "release")
	defaultAllow := mode == "debug"
	return map[string]any{
		"database.auto_migrate": env.GetBool("database.auto_migrate", defaultAllow),
	}
}
