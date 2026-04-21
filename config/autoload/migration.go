package autoload

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type Migration struct {
}

// Get 返回需要通过 GORM AutoMigrate 处理的模型列表（仅在 debug 模式或
// database.auto_migrate=true 时使用；生产环境请使用 migrations/ 下的 goose 脚本）。
func (receiver Migration) Get() []any {
	return []any{
		&model.Department{},
		&model.Role{},
		&model.Permission{},

		&model.User{},

		&model.UserDepartment{},
		&model.UserRole{},
		&model.RolePermission{},

		&model.Application{},
		&model.ApplicationOIDCKey{},

		&model.CaptchaConfig{},

		&model.AuthorizationCode{},
		&model.OIDCToken{},

		&model.SocialProvider{},
		&model.UserSocialAccount{},

		&model.Setting{},
	}
}

func (receiver Migration) InitConfig() map[string]any {
	env := helper.GetEnv()
	return map[string]any{
		"database.migration":     receiver.Get(),
		"database.migration.dir": env.GetString("database.migration.dir", "migrations"),
	}
}
