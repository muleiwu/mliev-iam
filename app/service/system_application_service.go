package service

import (
	"cnb.cool/mliev/iam/mliev-iam/pkg/firstparty"
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
	AdminConsoleApplicationUUID  = model.BuiltinAdminConsoleApplicationUUID
	AdminConsoleApplicationName  = "木雷坞 IAM 管理后台"
	AdminConsoleCallbackPath     = model.BuiltinAdminConsoleCallbackPath
	AccountCenterApplicationUUID = firstparty.AccountID
	AccountCenterCallbackPath    = firstparty.AccountCallbackPath
)

// IsAdminConsoleApplication limits first-party behavior to the single built-in,
// protected client. Merely marking another application protected is insufficient.
func IsAdminConsoleApplication(app *model.Application) bool {
	return app != nil && app.UUID == AdminConsoleApplicationUUID && app.IsProtected
}

// ApplicationAutoApprovesConsent is limited to these two built-in surfaces.
// Registered extension clients retain their own consent policy.
func ApplicationAutoApprovesConsent(app *model.Application) bool {
	return app != nil && app.IsProtected &&
		(app.UUID == AdminConsoleApplicationUUID || app.UUID == AccountCenterApplicationUUID)
}

// EnsureAccountCenterApplication allows all existing users, without admin roles.
func EnsureAccountCenterApplication(db *gorm.DB) (*model.Application, error) {
	return EnsureFirstPartyApplication(db, AccountCenterApplicationUUID, nil)
}

// ApplicationAllowsRegistration is the single policy used by the public
// client configuration and every server-side registration entry point.
// Built-in clients accept existing accounts; admin access is checked separately.
func ApplicationAllowsRegistration(app *model.Application) bool {
	if app == nil {
		return false
	}
	_, builtin := firstparty.Lookup(app.UUID)
	return !(builtin && app.IsProtected)
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

	return EnsureFirstPartyApplication(db, AdminConsoleApplicationUUID, &model.ApplicationAccessScope{UserIDs: []uint{}, DepartmentIDs: []uint{}, RoleIDs: []uint{superAdminRoleID}})
}

// EnsureFirstPartyApplication reconciles only clients registered by the binary.
func EnsureFirstPartyApplication(db *gorm.DB, clientID string, scope *model.ApplicationAccessScope) (*model.Application, error) {
	client, ok := firstparty.Lookup(clientID)
	if !ok || db == nil {
		return nil, errors.New("内置认证客户端未注册或数据库未初始化")
	}
	protocolConfig := adminConsoleProtocolConfig()
	protocolConfig["clientId"] = client.ID
	protocolConfig["redirectUris"] = []interface{}{client.CallbackPath}
	var result *model.Application
	err := db.Transaction(func(tx *gorm.DB) error {
		var app model.Application
		err := tx.Unscoped().Where("uuid = ?", client.ID).First(&app).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			app = model.Application{
				UUID:           client.ID,
				Name:           client.Name,
				Description:    client.Description,
				ProtocolType:   string(model.ProtocolTypeOIDC),
				ProtocolConfig: protocolConfig,
				IsActive:       true,
				IsVerified:     true,
				IsProtected:    true,
				AccessScope:    scope,
			}
			if err := tx.Create(&app).Error; err != nil {
				return fmt.Errorf("创建内置认证应用失败: %w", err)
			}
		case err != nil:
			return fmt.Errorf("检查内置认证应用失败: %w", err)
		default:
			app.ProtocolType = string(model.ProtocolTypeOIDC)
			app.ProtocolConfig = protocolConfig
			app.IsActive = true
			app.IsVerified = true
			app.IsProtected = true
			app.AccessScope = scope
			app.DeletedAt = gorm.DeletedAt{}
			if err := tx.Unscoped().Save(&app).Error; err != nil {
				return fmt.Errorf("修复内置认证应用失败: %w", err)
			}
		}

		if _, err := dao.NewApplicationOIDCKeyDAO(tx).GetPrimaryByApplicationID(app.ID); err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("检查内置认证签名密钥失败: %w", err)
			}
			if err := protocol.NewOIDCProtocolHandler(tx).CreateApplication(&app); err != nil {
				return fmt.Errorf("初始化内置认证签名密钥失败: %w", err)
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
	if _, err := EnsureAdminConsoleApplication(helper.GetDatabase(), 0); err != nil {
		return err
	}
	_, err := EnsureAccountCenterApplication(helper.GetDatabase())
	return err
}

func (*SystemApplicationServer) Stop() error { return nil }
