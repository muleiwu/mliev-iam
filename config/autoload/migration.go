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

		// 用户相关模型
		&model.User{},

		// 关联表模型（依赖User和Role/Permission/Department）
		&model.UserDepartment{}, // 用户部门关联表（保持独立性，支持To C和To B）
		&model.UserRole{},
		&model.RolePermission{},

		// 应用管理模型（支持多协议）
		&model.Application{},
		&model.ApplicationOIDCKey{}, // OIDC应用密钥表

		// OIDC协议相关模型
		&model.AuthorizationCode{},
		&model.OIDCToken{},

		// 社会化登录相关模型
		&model.SocialProvider{},    // 社会化登录提供商配置表
		&model.UserSocialAccount{}, // 用户社交账号绑定表

		// 测试模型（可选）
		//&model.TestDemo{},
	}
}

func (receiver Migration) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"database.migration": receiver.Get(),
	}
}
