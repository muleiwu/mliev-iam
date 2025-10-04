package autoload

import (
	envInterface "cnb.cool/mliev/examples/go-web/internal/interfaces"
)

type Migration struct {
}

func (receiver Migration) Get() []any {
	return []any{
		// 用户相关模型
		//&model.User{},
		//&model.Department{},
		//&model.Role{},
		//&model.Permission{},
		//&model.UserRole{},
		//&model.RolePermission{},
		//
		//// OIDC相关模型
		//&model.OIDCClient{},
		//&model.AuthorizationCode{},
		//&model.OIDCToken{},

		// 测试模型（可选）
		//&model.TestDemo{},
	}
}

func (receiver Migration) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"database.migration": receiver.Get(),
	}
}
