package migrations

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"gorm.io/gorm"
)

var allowedLegacyProviderTypes = map[string]struct{}{
	"generic": {}, "wechat": {}, "wechat_miniprogram": {}, "qq": {},
	"alipay": {}, "github": {}, "google": {}, "oidc": {},
	"feishu": {}, "wecom": {}, "dingtalk": {},
}

type legacyConnectionSource struct {
	Category     model.ConnectionCategory
	ID           uint
	Name         string
	Identifier   string
	ProviderType string
	Description  string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	RedirectURL  string
	Scopes       string
	Icon         string
	SortOrder    int
	Enabled      bool
	IsDefault    bool
	Attributes   model.JSONObject
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Remark       string
}

type migratedConnectionKey struct {
	Category model.ConnectionCategory
	ID       uint
}

// Legacy row types live only in the versioned migration. Runtime packages do
// not retain models or DAOs capable of reading the retired tables.
type legacySocialProvider struct {
	ID                                                                          uint
	Name, Identifier, Type, Description                                         string
	ClientID, ClientSecret, AuthURL, TokenURL, UserInfoURL, RedirectURL, Scopes string
	Icon                                                                        string
	SortOrder                                                                   int
	Status                                                                      int
	IsDefault                                                                   bool
	Attributes                                                                  model.JSONObject
	CreatedAt, UpdatedAt                                                        time.Time
	Remark                                                                      string
}

func (legacySocialProvider) TableName() string { return "social_providers" }

type legacyIdentityProvider struct {
	ID                                                                          uint
	Name, Identifier, Type, Description                                         string
	ClientID, ClientSecret, AuthURL, TokenURL, UserInfoURL, RedirectURL, Scopes string
	Icon                                                                        string
	SortOrder                                                                   int
	Status                                                                      int
	IsDefault                                                                   bool
	Attributes                                                                  model.JSONObject
	CreatedAt, UpdatedAt                                                        time.Time
	Remark                                                                      string
}

func (legacyIdentityProvider) TableName() string { return "identity_providers" }

type legacySocialAccount struct {
	ID, UserID, ProviderID uint
	OpenID, UnionID        string
	UserInfo               model.JSONObject
	LastUsedAt             *time.Time
	LoginCount             int64
	CreatedAt, UpdatedAt   time.Time
}

func (legacySocialAccount) TableName() string { return "user_social_accounts" }

type legacyIdentityAccount struct {
	ID, UserID, ProviderID  uint
	ExternalUserID, UnionID string
	UserInfo                model.JSONObject
	LastUsedAt              *time.Time
	LoginCount              int64
	CreatedAt, UpdatedAt    time.Time
}

func (legacyIdentityAccount) TableName() string { return "user_identity_accounts" }

func migrateLegacyIdentityData(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	sources, err := loadAndValidateLegacyConnections(db)
	if err != nil {
		return err
	}
	cipher, err := credentialcrypto.FromEnvironment()
	if err != nil {
		return fmt.Errorf("凭据主密钥检查失败: %w", err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		connectionIDs := make(map[migratedConnectionKey]uint, len(sources))
		for _, source := range sources {
			connectionID, migrateErr := migrateLegacyConnection(tx, cipher, source)
			if migrateErr != nil {
				return migrateErr
			}
			connectionIDs[migratedConnectionKey{Category: source.Category, ID: source.ID}] = connectionID
		}

		socialCount, err := migrateSocialAccounts(tx, connectionIDs)
		if err != nil {
			return err
		}
		enterpriseCount, err := migrateEnterpriseAccounts(tx, connectionIDs)
		if err != nil {
			return err
		}
		return verifyIdentityMigration(tx, len(sources), socialCount+enterpriseCount)
	})
}

func loadAndValidateLegacyConnections(db *gorm.DB) ([]legacyConnectionSource, error) {
	var socialProviders []legacySocialProvider
	if err := db.Where("deleted_at IS NULL").Find(&socialProviders).Error; err != nil {
		return nil, fmt.Errorf("读取旧社交登录提供商失败: %w", err)
	}
	var identityProviders []legacyIdentityProvider
	if err := db.Where("deleted_at IS NULL").Find(&identityProviders).Error; err != nil {
		return nil, fmt.Errorf("读取旧企业身份源失败: %w", err)
	}

	seenIdentifiers := make(map[string]string, len(socialProviders)+len(identityProviders))
	sources := make([]legacyConnectionSource, 0, len(socialProviders)+len(identityProviders))
	validate := func(source legacyConnectionSource) error {
		identifier := strings.TrimSpace(source.Identifier)
		if identifier == "" {
			return fmt.Errorf("旧 %s provider id=%d 的 identifier 为空", source.Category, source.ID)
		}
		if previous, exists := seenIdentifiers[identifier]; exists {
			return fmt.Errorf("跨旧表重复 identifier=%q（%s 与 %s:%d）", identifier, previous, source.Category, source.ID)
		}
		if _, ok := allowedLegacyProviderTypes[source.ProviderType]; !ok {
			return fmt.Errorf("旧 provider %q 使用非法类型 %q", identifier, source.ProviderType)
		}
		seenIdentifiers[identifier] = fmt.Sprintf("%s:%d", source.Category, source.ID)
		return nil
	}

	for _, provider := range socialProviders {
		source := legacyConnectionSource{
			Category: model.ConnectionCategorySocial, ID: provider.ID, Name: provider.Name,
			Identifier: provider.Identifier, ProviderType: provider.Type, Description: provider.Description,
			ClientID: provider.ClientID, ClientSecret: provider.ClientSecret, AuthURL: provider.AuthURL,
			TokenURL: provider.TokenURL, UserInfoURL: provider.UserInfoURL, RedirectURL: provider.RedirectURL,
			Scopes: provider.Scopes, Icon: provider.Icon, SortOrder: provider.SortOrder,
			Enabled: provider.Status == 1, IsDefault: provider.IsDefault,
			Attributes: provider.Attributes, CreatedAt: provider.CreatedAt, UpdatedAt: provider.UpdatedAt,
			Remark: provider.Remark,
		}
		if err := validate(source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	for _, provider := range identityProviders {
		source := legacyConnectionSource{
			Category: model.ConnectionCategoryEnterprise, ID: provider.ID, Name: provider.Name,
			Identifier: provider.Identifier, ProviderType: provider.Type, Description: provider.Description,
			ClientID: provider.ClientID, ClientSecret: provider.ClientSecret, AuthURL: provider.AuthURL,
			TokenURL: provider.TokenURL, UserInfoURL: provider.UserInfoURL, RedirectURL: provider.RedirectURL,
			Scopes: provider.Scopes, Icon: provider.Icon, SortOrder: provider.SortOrder,
			Enabled: provider.Status == 1, IsDefault: provider.IsDefault,
			Attributes: provider.Attributes, CreatedAt: provider.CreatedAt, UpdatedAt: provider.UpdatedAt,
			Remark: provider.Remark,
		}
		if err := validate(source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}

	if err := validateLegacyBindings(db, socialProviders, identityProviders); err != nil {
		return nil, err
	}
	return sources, nil
}

func validateLegacyBindings(db *gorm.DB, social []legacySocialProvider, enterprise []legacyIdentityProvider) error {
	for _, provider := range social {
		var duplicates int64
		query := db.Table("user_social_accounts").
			Select("user_id").
			Where("provider_id = ? AND deleted_at IS NULL", provider.ID).
			Group("user_id").Having("COUNT(*) > 1")
		if err := db.Table("(?) AS duplicate_bindings", query).Count(&duplicates).Error; err != nil {
			return fmt.Errorf("检查旧社交绑定唯一性失败: %w", err)
		}
		if duplicates > 0 {
			return fmt.Errorf("旧社交连接 %q 存在同一用户的重复绑定", provider.Identifier)
		}
		for _, column := range []string{"open_id", "union_id"} {
			query := db.Table("user_social_accounts").Select(column).
				Where("provider_id = ? AND deleted_at IS NULL AND "+column+" IS NOT NULL AND "+column+" <> ''", provider.ID).
				Group(column).Having("COUNT(*) > 1")
			if err := db.Table("(?) AS duplicate_identifiers", query).Count(&duplicates).Error; err != nil {
				return fmt.Errorf("检查旧社交标识 %s 唯一性失败: %w", column, err)
			}
			if duplicates > 0 {
				return fmt.Errorf("旧社交连接 %q 存在重复 %s，无法完整迁移别名", provider.Identifier, column)
			}
		}
	}
	for _, provider := range enterprise {
		var duplicates int64
		query := db.Table("user_identity_accounts").
			Select("user_id").
			Where("provider_id = ? AND deleted_at IS NULL", provider.ID).
			Group("user_id").Having("COUNT(*) > 1")
		if err := db.Table("(?) AS duplicate_bindings", query).Count(&duplicates).Error; err != nil {
			return fmt.Errorf("检查旧企业绑定唯一性失败: %w", err)
		}
		if duplicates > 0 {
			return fmt.Errorf("旧企业连接 %q 存在同一用户的重复绑定", provider.Identifier)
		}
		for _, column := range []string{"external_user_id", "union_id"} {
			query := db.Table("user_identity_accounts").Select(column).
				Where("provider_id = ? AND deleted_at IS NULL AND "+column+" IS NOT NULL AND "+column+" <> ''", provider.ID).
				Group(column).Having("COUNT(*) > 1")
			if err := db.Table("(?) AS duplicate_identifiers", query).Count(&duplicates).Error; err != nil {
				return fmt.Errorf("检查旧企业标识 %s 唯一性失败: %w", column, err)
			}
			if duplicates > 0 {
				return fmt.Errorf("旧企业连接 %q 存在重复 %s，无法完整迁移别名", provider.Identifier, column)
			}
		}
	}
	return nil
}

func migrateLegacyConnection(db *gorm.DB, cipher *credentialcrypto.Cipher, source legacyConnectionSource) (uint, error) {
	config := cloneJSONObject(source.Attributes)
	config["clientId"] = source.ClientID
	config["authUrl"] = source.AuthURL
	config["tokenUrl"] = source.TokenURL
	config["userInfoUrl"] = source.UserInfoURL
	config["redirectUrl"] = source.RedirectURL
	config["scopes"] = source.Scopes

	status := model.ConnectionStatusDisabled
	if source.Enabled {
		status = model.ConnectionStatusEnabled
	}
	connection := model.IdentityConnection{}
	result := db.Where("identifier = ?", source.Identifier).First(&connection)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return 0, fmt.Errorf("检查统一连接 %q 失败: %w", source.Identifier, result.Error)
	}
	if result.Error == gorm.ErrRecordNotFound {
		connection = model.IdentityConnection{
			Name: source.Name, Identifier: source.Identifier, Category: source.Category,
			ProviderType: source.ProviderType, Description: source.Description,
			Capabilities: model.StringList{string(model.ConnectionCapabilityAuthentication)},
			Config:       config, Icon: source.Icon, SortOrder: source.SortOrder, Status: status,
			IsDefault: source.IsDefault, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
			Remark: source.Remark,
		}
		if err := db.Create(&connection).Error; err != nil {
			return 0, fmt.Errorf("迁移连接 %q 失败: %w", source.Identifier, err)
		}
	} else if connection.Category != source.Category || connection.ProviderType != source.ProviderType {
		return 0, fmt.Errorf("统一连接 %q 已存在但类别或类型与旧数据不一致", source.Identifier)
	}

	var credentialCount int64
	if err := db.Model(&model.IdentityConnectionCredential{}).
		Where("connection_id = ? AND purpose = ?", connection.ID, model.CredentialPurposeAuthentication).
		Count(&credentialCount).Error; err != nil {
		return 0, fmt.Errorf("检查连接 %q 凭据失败: %w", source.Identifier, err)
	}
	if credentialCount == 0 {
		plaintext, err := json.Marshal(map[string]string{
			"clientId": source.ClientID, "clientSecret": source.ClientSecret,
		})
		if err != nil {
			return 0, fmt.Errorf("序列化连接 %q 凭据失败: %w", source.Identifier, err)
		}
		envelope, err := cipher.Encrypt(plaintext, credentialcrypto.AAD(connection.ID, string(model.CredentialPurposeAuthentication), cipher.ActiveKeyVersion()))
		if err != nil {
			return 0, fmt.Errorf("加密连接 %q 凭据失败: %w", source.Identifier, err)
		}
		rotatedAt := source.UpdatedAt
		if rotatedAt.IsZero() {
			rotatedAt = time.Now()
		}
		credential := model.IdentityConnectionCredential{
			ConnectionID: connection.ID, Purpose: model.CredentialPurposeAuthentication,
			Ciphertext: envelope.Ciphertext, Nonce: envelope.Nonce, Algorithm: envelope.Algorithm,
			KeyVersion: envelope.KeyVersion, RotatedAt: rotatedAt,
		}
		if err := db.Create(&credential).Error; err != nil {
			return 0, fmt.Errorf("保存连接 %q 加密凭据失败: %w", source.Identifier, err)
		}
	}
	return connection.ID, nil
}

func migrateSocialAccounts(db *gorm.DB, connectionIDs map[migratedConnectionKey]uint) (int, error) {
	var accounts []legacySocialAccount
	if err := db.Where("deleted_at IS NULL").Find(&accounts).Error; err != nil {
		return 0, fmt.Errorf("读取旧社交账号绑定失败: %w", err)
	}
	for _, account := range accounts {
		connectionID, ok := connectionIDs[migratedConnectionKey{Category: model.ConnectionCategorySocial, ID: account.ProviderID}]
		if !ok {
			return 0, fmt.Errorf("旧社交账号 id=%d 找不到 provider id=%d", account.ID, account.ProviderID)
		}
		if err := migrateLegacyAccount(db, connectionID, account.UserID, account.OpenID, account.UnionID, account.UserInfo, account.LastUsedAt, account.LoginCount, account.CreatedAt, account.UpdatedAt); err != nil {
			return 0, fmt.Errorf("迁移旧社交账号 id=%d 失败: %w", account.ID, err)
		}
	}
	return len(accounts), nil
}

func migrateEnterpriseAccounts(db *gorm.DB, connectionIDs map[migratedConnectionKey]uint) (int, error) {
	var accounts []legacyIdentityAccount
	if err := db.Where("deleted_at IS NULL").Find(&accounts).Error; err != nil {
		return 0, fmt.Errorf("读取旧企业账号绑定失败: %w", err)
	}
	for _, account := range accounts {
		connectionID, ok := connectionIDs[migratedConnectionKey{Category: model.ConnectionCategoryEnterprise, ID: account.ProviderID}]
		if !ok {
			return 0, fmt.Errorf("旧企业账号 id=%d 找不到 provider id=%d", account.ID, account.ProviderID)
		}
		if err := migrateLegacyAccount(db, connectionID, account.UserID, account.ExternalUserID, account.UnionID, account.UserInfo, account.LastUsedAt, account.LoginCount, account.CreatedAt, account.UpdatedAt); err != nil {
			return 0, fmt.Errorf("迁移旧企业账号 id=%d 失败: %w", account.ID, err)
		}
	}
	return len(accounts), nil
}

func migrateLegacyAccount(db *gorm.DB, connectionID, userID uint, subject, unionID string, profile model.JSONObject, lastUsedAt *time.Time, loginCount int64, createdAt, updatedAt time.Time) error {
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("登录主标识为空")
	}
	var existing model.ExternalIdentifier
	result := db.Where("connection_id = ? AND namespace = ? AND value = ?", connectionID, model.IdentifierNamespaceLoginSubject, subject).First(&existing)
	if result.Error == nil {
		return nil
	}
	if result.Error != gorm.ErrRecordNotFound {
		return result.Error
	}

	principal := model.ExternalPrincipal{
		ConnectionID: connectionID, UserID: &userID, Status: model.PrincipalStatusActive,
		DisplayName: profileString(profile, "displayName", "display_name", "nickname", "name"),
		Avatar:      profileString(profile, "avatar", "avatarUrl", "avatar_url"),
		Email:       profileString(profile, "email"), Phone: profileString(profile, "phone", "mobile"),
		RawProfile: cloneJSONObject(profile), LastLoginAt: lastUsedAt, LoginCount: loginCount,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	if err := db.Create(&principal).Error; err != nil {
		return err
	}
	primary := model.ExternalIdentifier{
		ConnectionID: connectionID, PrincipalID: principal.ID,
		Namespace: model.IdentifierNamespaceLoginSubject, Value: subject, IsPrimary: true,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	if err := db.Create(&primary).Error; err != nil {
		return err
	}
	if strings.TrimSpace(unionID) != "" {
		var unionCount int64
		if err := db.Model(&model.ExternalIdentifier{}).
			Where("connection_id = ? AND namespace = ? AND value = ?", connectionID, model.IdentifierNamespaceUnionID, unionID).
			Count(&unionCount).Error; err != nil {
			return err
		}
		if unionCount == 0 {
			alias := model.ExternalIdentifier{
				ConnectionID: connectionID, PrincipalID: principal.ID,
				Namespace: model.IdentifierNamespaceUnionID, Value: unionID,
				CreatedAt: createdAt, UpdatedAt: updatedAt,
			}
			if err := db.Create(&alias).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyIdentityMigration(db *gorm.DB, expectedConnections, expectedPrincipals int) error {
	var connections, credentials, principals int64
	if err := db.Model(&model.IdentityConnection{}).Count(&connections).Error; err != nil {
		return err
	}
	if err := db.Model(&model.IdentityConnectionCredential{}).Count(&credentials).Error; err != nil {
		return err
	}
	if err := db.Model(&model.ExternalPrincipal{}).Count(&principals).Error; err != nil {
		return err
	}
	if connections != int64(expectedConnections) || credentials != int64(expectedConnections) || principals != int64(expectedPrincipals) {
		return fmt.Errorf("迁移数量核对失败: connections=%d/%d credentials=%d/%d principals=%d/%d", connections, expectedConnections, credentials, expectedConnections, principals, expectedPrincipals)
	}
	return nil
}

func cloneJSONObject(source model.JSONObject) model.JSONObject {
	result := make(model.JSONObject, len(source)+6)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func profileString(profile model.JSONObject, keys ...string) string {
	for _, key := range keys {
		if value, ok := profile[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
