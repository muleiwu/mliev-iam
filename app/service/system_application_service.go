package service

import (
	"errors"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"cnb.cool/mliev/open/go-web/pkg/interfaces"
	"gorm.io/gorm"
)

const (
	// AdminConsoleApplicationUUID is UUIDv5(URL, "https://cnb.cool/mliev/iam/mliev-iam/admin-console").
	// It is deliberately stable so every deployment can identify the built-in client.
	AdminConsoleApplicationUUID = model.BuiltinAdminConsoleApplicationUUID
	AdminConsoleApplicationName = "木雷坞 IAM 管理后台"
	AdminConsoleCallbackPath    = model.BuiltinAdminConsoleCallbackPath
)

// IsAdminConsoleApplication limits first-party behavior to the single built-in,
// protected client. Merely marking another application protected is insufficient.
func IsAdminConsoleApplication(app *model.Application) bool {
	return app != nil && app.UUID == AdminConsoleApplicationUUID && app.IsProtected
}

// ApplicationAllowsRegistration is the single policy used by the public
// client configuration and every server-side registration entry point.
// The built-in admin console only accepts existing super administrators.
func ApplicationAllowsRegistration(app *model.Application) bool {
	return app != nil && !IsAdminConsoleApplication(app)
}

func adminConsoleProtocolConfig() map[string]interface{} {
	return map[string]interface{}{
		"clientId":         AdminConsoleApplicationUUID,
		"redirectUris":     []interface{}{AdminConsoleCallbackPath},
		"grantTypes":       []interface{}{string(model.GrantTypeAuthorizationCode)},
		"responseTypes":    []interface{}{string(model.ResponseTypeCode)},
		"scopes":           []interface{}{"openid", "profile"},
		"clientType":       "public",
		"requirePkce":      true,
		"tokenLifetime":    3600,
		"signingAlgorithm": "RS256",
	}
}

// EnsureAdminConsoleApplication creates or repairs the built-in OIDC client.
// Display fields are intentionally preserved; security-critical fields converge
// to the compiled defaults on every installation/startup check.
func EnsureAdminConsoleApplication(db *gorm.DB, superAdminRoleID uint) (*model.Application, error) {
	if db == nil {
		return nil, errors.New("数据库未初始化")
	}
	if superAdminRoleID == 0 {
		role, err := dao.NewRoleDao(db).GetByCode("super_admin")
		if err != nil {
			return nil, fmt.Errorf("查找 super_admin 角色失败: %w", err)
		}
		superAdminRoleID = role.ID
	}

	var result *model.Application
	err := db.Transaction(func(tx *gorm.DB) error {
		var app model.Application
		err := tx.Unscoped().Where("uuid = ?", AdminConsoleApplicationUUID).First(&app).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			app = model.Application{
				UUID:           AdminConsoleApplicationUUID,
				Name:           AdminConsoleApplicationName,
				Description:    "IAM 系统内置管理后台",
				ProtocolType:   string(model.ProtocolTypeOIDC),
				ProtocolConfig: adminConsoleProtocolConfig(),
				IsActive:       true,
				IsVerified:     true,
				IsProtected:    true,
				AccessScope: &model.ApplicationAccessScope{
					UserIDs:       []uint{},
					DepartmentIDs: []uint{},
					RoleIDs:       []uint{superAdminRoleID},
				},
			}
			if err := tx.Create(&app).Error; err != nil {
				return fmt.Errorf("创建内置管理后台应用失败: %w", err)
			}
		case err != nil:
			return fmt.Errorf("检查内置管理后台应用失败: %w", err)
		default:
			app.ProtocolType = string(model.ProtocolTypeOIDC)
			app.ProtocolConfig = adminConsoleProtocolConfig()
			app.IsActive = true
			app.IsVerified = true
			app.IsProtected = true
			app.AccessScope = &model.ApplicationAccessScope{
				UserIDs:       []uint{},
				DepartmentIDs: []uint{},
				RoleIDs:       []uint{superAdminRoleID},
			}
			app.DeletedAt = gorm.DeletedAt{}
			if err := tx.Unscoped().Save(&app).Error; err != nil {
				return fmt.Errorf("修复内置管理后台应用失败: %w", err)
			}
		}

		if _, err := dao.NewApplicationOIDCKeyDAO(tx).GetPrimaryByApplicationID(app.ID); err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("检查内置管理后台签名密钥失败: %w", err)
			}
			if err := protocol.NewOIDCProtocolHandler(tx).CreateApplication(&app); err != nil {
				return fmt.Errorf("初始化内置管理后台签名密钥失败: %w", err)
			}
		}

		result = &app
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SystemApplicationServer reconciles built-in applications after schema migrations.
type SystemApplicationServer struct{}

var _ interfaces.ServerInterface = (*SystemApplicationServer)(nil)

func (*SystemApplicationServer) Run() error {
	if !helper.GetConfig().GetBool("app.installed", false) {
		return nil
	}
	_, err := EnsureAdminConsoleApplication(helper.GetDatabase(), 0)
	return err
}

func (*SystemApplicationServer) Stop() error { return nil }
