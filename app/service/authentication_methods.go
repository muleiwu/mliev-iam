package service

import (
	"errors"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

func stringPointerValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (s *TwoFactorService) passkeySettings() PasskeyConfig {
	if s.passkeyConfig != nil {
		return *s.passkeyConfig
	}
	return ReadPasskeyConfig()
}

// A usable login path needs a primary method and, when enabled, a distinct second method.
// External identities count only as primary methods. Recovery codes never enable 2FA.
func (s *TwoFactorService) loginPath(userID uint, methods []dto.TwoFactorMethodInfo) (bool, bool, error) {
	primary, types := false, 0
	for _, m := range methods {
		if !m.Enabled || !m.Available {
			continue
		}
		types++
		if m.Method != "totp" {
			primary = true
		}
	}
	if primary && types >= 2 {
		return true, true, nil
	}
	var principals []model.ExternalPrincipal
	if err := s.db.Where("user_id = ? AND status = ?", userID, model.PrincipalStatusActive).Find(&principals).Error; err != nil {
		return false, false, err
	}
	external := false
	for _, p := range principals {
		var connection model.IdentityConnection
		if err := s.db.First(&connection, p.ConnectionID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		} else if err != nil {
			return false, false, err
		}
		if connection.IsEnabled() && connection.HasCapability(model.ConnectionCapabilityAuthentication) {
			external = true
			break
		}
	}
	return primary || external, (primary && types >= 2) || (external && types >= 1), nil
}
func (s *TwoFactorService) validateLoginPath(userID uint) error {
	status, err := s.Status(userID)
	if err != nil {
		return err
	}
	first, two, err := s.loginPath(userID, status.Methods)
	if err != nil {
		return err
	}
	if !first {
		return errors.New("不能移除唯一的登录方式，请先添加其他登录方式")
	}
	if status.Enabled && !two {
		return errors.New("两步验证需要两种不同的认证方式，请先添加其他方式或明确关闭两步验证")
	}
	return nil
}
func (s *TwoFactorService) withDB(db *gorm.DB) *TwoFactorService {
	copy := *s
	copy.db = db
	return &copy
}

// InvalidateAuthenticationConfiguration is for administrator/directory changes:
// invalidate pending challenges and all grants without minting a replacement session.
func InvalidateAuthenticationConfiguration(tx *gorm.DB, userID uint, action string) error {
	if err := dao.LockLoginUser(tx, userID); err != nil {
		return err
	}
	if err := tx.Model(&model.User{}).Where("id = ?", userID).Update("two_factor_version", gorm.Expr("two_factor_version + 1")).Error; err != nil {
		return err
	}
	return RevokeLoginSessions(tx, userID, "", "", action, nil)
}
