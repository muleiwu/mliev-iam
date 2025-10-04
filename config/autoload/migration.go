package autoload

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
)

type Migration struct {
}

func (receiver Migration) Get() []any {
	return []any{
		// 基础模型（被其他表引用的表需要先创建）
		&model.Department{},
		&model.Role{},
		&model.Permission{},

		// 用户相关模型（依赖Department）
		&model.User{},

		// 关联表模型（依赖User和Role/Permission）
		&model.UserRole{},
		&model.RolePermission{},

		// OIDC相关模型
		&model.OIDCClient{},
		&model.AuthorizationCode{},
		&model.OIDCToken{},

		// 测试模型（可选）
		//&model.TestDemo{},
	}
}

func (receiver Migration) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"database.migration": receiver.Get(),
	}
}
