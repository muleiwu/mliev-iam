package service

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type weChatConflictError struct{ hash string }

func (e *weChatConflictError) Error() string {
	return "微信身份指向不同账号，请联系管理员处理"
}

func miniHash(parts ...string) string {
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func miniCredential(db *gorm.DB, c *model.IdentityConnection) (string, string, error) {
	values, err := NewIdentityConnectionService(db).CredentialValues(c.ID, model.CredentialPurposeAuthentication)
	if err != nil {
		return "", "", err
	}
	value := func(key string) string {
		if v, ok := values[key].(string); ok && v != "" {
			return v
		}
		v, _ := c.Config[key].(string)
		return v
	}
	return value("clientId"), value("clientSecret"), nil
}

func miniPlatform(c *model.IdentityConnection) string {
	v, _ := c.Config["openPlatformId"].(string)
	return strings.TrimSpace(v)
}

func miniIdentityKeys(appID, platform, openID, unionID string) []model.WeChatIdentity {
	result := []model.WeChatIdentity{{Kind: "openid", Scope: appID, Subject: openID}}
	if platform != "" && unionID != "" {
		result = append(result, model.WeChatIdentity{Kind: "unionid", Scope: platform, Subject: unionID})
	}
	for i := range result {
		result[i].Hash = miniHash(result[i].Kind, result[i].Scope, result[i].Subject)
	}
	return result
}

// Resolve checks every historical binding, including disabled connections, before
// accepting an index hit. A migration must never choose the first of two users.
func resolveMiniIdentity(db *gorm.DB, connection *model.IdentityConnection, appID, openID, unionID string) (uint, error) {
	keys := miniIdentityKeys(appID, miniPlatform(connection), openID, unionID)
	users := map[uint]bool{}
	for _, key := range keys {
		var index model.WeChatIdentity
		err := db.First(&index, "hash = ?", key.Hash).Error
		if err == nil {
			users[index.UserID] = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
	}
	var connections []model.IdentityConnection
	if err := db.Where("provider_type = ?", "wechat_miniprogram").Find(&connections).Error; err != nil {
		return 0, err
	}
	for _, candidate := range connections {
		candidateAppID, _, err := miniCredential(db, &candidate)
		if err != nil {
			return 0, err
		}
		query := db.Model(&model.ExternalIdentifier{}).Select("external_principals.*, external_identifiers.value AS matched_value, external_identifiers.namespace AS matched_namespace").Joins("JOIN external_principals ON external_principals.id = external_identifiers.principal_id").Where("external_principals.deleted_at IS NULL AND external_identifiers.connection_id = ?", candidate.ID)
		conditions := db.Where("1 = 0")
		if candidateAppID == appID {
			conditions = conditions.Or("external_identifiers.namespace IN ? AND external_identifiers.value = ?", []string{"open_id", "login_subject"}, openID)
		}
		if unionID != "" && miniPlatform(connection) != "" && miniPlatform(&candidate) == miniPlatform(connection) {
			conditions = conditions.Or("external_identifiers.namespace = ? AND external_identifiers.value = ?", "union_id", unionID)
		}
		var principals []struct {
			model.ExternalPrincipal
			MatchedValue     string
			MatchedNamespace string
		}
		if err := query.Where(conditions).Scan(&principals).Error; err != nil {
			return 0, err
		}
		for _, principal := range principals {
			// WeChat identifiers are case sensitive even on legacy MySQL schemas
			// whose identifier column uses a case-insensitive collation.
			exactOpen := candidateAppID == appID && principal.MatchedValue == openID && (principal.MatchedNamespace == "open_id" || principal.MatchedNamespace == "login_subject")
			exactUnion := unionID != "" && miniPlatform(connection) != "" && miniPlatform(&candidate) == miniPlatform(connection) && principal.MatchedNamespace == "union_id" && principal.MatchedValue == unionID
			if !exactOpen && !exactUnion {
				continue
			}
			if principal.Status == model.PrincipalStatusBlocked {
				return 0, errors.New("微信身份已停用")
			}
			if principal.UserID != nil {
				users[*principal.UserID] = true
			}
		}
	}
	if len(users) > 1 {
		return 0, &weChatConflictError{keys[0].Hash}
	}
	for id := range users {
		return id, nil
	}
	return 0, nil
}

func ensureMiniIdentityIndex(db *gorm.DB, c *model.IdentityConnection, appID, openID, unionID string, userID uint) error {
	if !c.IsEnabled() || !c.HasCapability(model.ConnectionCapabilityAuthentication) {
		return errors.New("微信身份连接已停用")
	}
	existing, err := resolveMiniIdentity(db, c, appID, openID, unionID)
	if err != nil {
		return err
	}
	if existing != 0 && existing != userID {
		return &weChatConflictError{miniHash("openid", appID, openID)}
	}
	for _, key := range miniIdentityKeys(appID, miniPlatform(c), openID, unionID) {
		key.UserID = userID
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&key).Error; err != nil {
			return err
		}
		var stored model.WeChatIdentity
		if err := db.First(&stored, "hash = ?", key.Hash).Error; err != nil {
			return err
		}
		if stored.UserID != userID {
			return &weChatConflictError{key.Hash}
		}
	}
	return nil
}

func attachMiniIdentity(db *gorm.DB, c *model.IdentityConnection, appID, openID, unionID string, userID uint) error {
	profile := &protocol.ExternalProfile{Identifiers: []protocol.ExternalProfileIdentifier{
		{Namespace: model.IdentifierNamespaceLoginSubject, Value: openID, Primary: true},
		{Namespace: model.IdentifierNamespaceOpenID, Value: openID},
	}, Raw: map[string]any{"openid": openID}}
	if unionID != "" {
		profile.Identifiers = append(profile.Identifiers, protocol.ExternalProfileIdentifier{Namespace: model.IdentifierNamespaceUnionID, Value: unionID})
		profile.Raw["unionid"] = unionID
	}
	return NewExternalAuthService(db, nil).attachProfile(db, c.ID, userID, profile)
}

// An identity namespace cannot be reassigned once it has ever held bindings.
// Both configuration updates and direct credential rotation use this guard.
func guardMiniConnectionIdentity(db *gorm.DB, next *model.IdentityConnection, credentials map[string]any) error {
	var old model.IdentityConnection
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, next.ID).Error; err != nil {
		return err
	}
	if old.ProviderType != "wechat_miniprogram" && next.ProviderType != "wechat_miniprogram" {
		return nil
	}
	if len(miniPlatform(next)) > 128 {
		return errors.New("开放平台归属标识不能超过128字节")
	}
	oldID, _, err := miniCredential(db, &old)
	if err != nil {
		return err
	}
	nextID := oldID
	if v, ok := next.Config["clientId"].(string); ok && v != "" {
		nextID = v
	}
	// Existing credentials take precedence over config, as in the adapter.
	current, err := NewIdentityConnectionService(db).CredentialValues(old.ID, model.CredentialPurposeAuthentication)
	if err != nil {
		return err
	}
	if v, ok := current["clientId"].(string); ok && v != "" {
		nextID = v
	}
	if v, ok := credentials["clientId"].(string); ok && v != "" {
		nextID = v
	}
	if oldID == nextID && miniPlatform(&old) == miniPlatform(next) && old.ProviderType == next.ProviderType && old.Category == next.Category {
		return nil
	}
	var count int64
	if err := db.Unscoped().Model(&model.ExternalPrincipal{}).Where("connection_id = ?", old.ID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("连接已有身份数据，不能修改 AppID、开放平台归属或身份源类型")
	}
	return nil
}

// Read-only resolution shared by legacy code login and QR login. In particular,
// do not call the generic resolver here: it appends aliases before authentication
// and only considers one connection, so a newly supplied UnionID could conflict
// with an account already bound through another mini program.
func findMiniProgramPrincipal(db *gorm.DB, c *model.IdentityConnection, profile *protocol.ExternalProfile) (*model.ExternalPrincipal, error) {
	if !c.IsEnabled() || !c.HasCapability(model.ConnectionCapabilityAuthentication) {
		return nil, errors.New("微信身份连接已停用")
	}
	appID, _, err := miniCredential(db, c)
	if err != nil {
		return nil, err
	}
	var openID, unionID string
	for _, id := range profile.Identifiers {
		if id.Namespace == model.IdentifierNamespaceOpenID {
			openID = id.Value
		}
		if id.Namespace == model.IdentifierNamespaceUnionID {
			unionID = id.Value
		}
	}
	if appID == "" || openID == "" {
		return nil, errors.New("微信身份标识不完整")
	}
	userID, err := resolveMiniIdentity(db, c, appID, openID, unionID)
	if err != nil {
		return nil, err
	}
	condition := db.Where("namespace IN ? AND value = ?", []string{"open_id", "login_subject"}, openID)
	if miniPlatform(c) != "" && unionID != "" {
		condition = condition.Or("namespace = ? AND value = ?", "union_id", unionID)
	}
	var matches []model.ExternalIdentifier
	if err := db.Where("connection_id = ?", c.ID).Where(condition).Find(&matches).Error; err != nil {
		return nil, err
	}
	ids := map[uint]bool{}
	for _, match := range matches {
		if ((match.Namespace == model.IdentifierNamespaceOpenID || match.Namespace == model.IdentifierNamespaceLoginSubject) && match.Value == openID) || (miniPlatform(c) != "" && unionID != "" && match.Namespace == model.IdentifierNamespaceUnionID && match.Value == unionID) {
			ids[match.PrincipalID] = true
		}
	}
	if len(ids) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if len(ids) > 1 {
		return nil, &weChatConflictError{miniHash("openid", appID, openID)}
	}
	var principal model.ExternalPrincipal
	for id := range ids {
		if err := db.First(&principal, id).Error; err != nil {
			return nil, err
		}
	}
	if principal.Status == model.PrincipalStatusBlocked {
		return nil, errors.New("微信身份已停用")
	}
	if principal.UserID != nil && *principal.UserID != userID {
		return nil, &weChatConflictError{miniHash("openid", appID, openID)}
	}
	return &principal, nil
}

func recordMiniIdentityConflict(db *gorm.DB, connectionID uint, err error) {
	var conflict *weChatConflictError
	if errors.As(err, &conflict) {
		_ = db.Create(&model.WeChatIdentityConflict{ConnectionID: connectionID, IdentityHash: conflict.hash}).Error
	}
}

// Existing WeChat bindings may learn a newly available UnionID, but only after
// primary authentication (and MFA, if enabled). Keep the continuation private.
func (s *ExternalAuthService) prepareMiniProgramLogin(connection *model.IdentityConnection, profile *protocol.ExternalProfile, userID uint, sessionID string) (*dto.SocialAuthCallbackResponse, error) {
	var continuationID, csrf string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var current model.IdentityConnection
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, connection.ID).Error; err != nil {
			return err
		}
		if err := dao.LockLoginUser(tx, userID); err != nil {
			return err
		}
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		if !user.CanLogin() {
			return errors.New("账号不可登录")
		}
		principal, err := findMiniProgramPrincipal(tx, &current, profile)
		if err != nil {
			return err
		}
		if principal.UserID == nil || *principal.UserID != userID {
			return ErrMiniTransaction
		}
		if user.TwoFactorEnabled {
			continuationID, csrf, err = s.storeContinuation(&current, profile, sessionID, &user)
			return err
		}
		auth := NewExternalAuthService(tx, s.redis)
		if err := auth.attachProfile(tx, current.ID, userID, profile); err != nil {
			return err
		}
		return auth.refreshPrincipal(principal, profile, true)
	})
	if err != nil {
		recordMiniIdentityConflict(s.db, connection.ID, err)
		return nil, err
	}
	response, err := s.loginUser(userID, false, sessionID == "")
	if err != nil {
		return nil, err
	}
	response.SessionID = sessionID
	response.PendingContinuationID = continuationID
	response.PendingContinuationCSRF = csrf
	return response, nil
}
