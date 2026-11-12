package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrProfileCompletionConflict = errors.New("资料或登录状态已变化，请刷新页面后重试")
	ErrProfileCompletionInvalid  = errors.New("资料补全请求无效")
	ErrProfileCompletionRelogin  = errors.New("资料已保存，请重新登录后继续授权")
)

type ProfileCompletionInput struct {
	Email, EmailCode, Phone, PhoneCode string
	Issuer                             string
	// These deadlines are extracted from validated browser credentials by the
	// controller, not accepted as client-controlled JSON fields.
	UserTokenDeadline, RefreshDeadline, RefreshCookieDeadline time.Time
}

type ProfileCompletionResult struct {
	Session                                  *model.LoginSession
	Tokens                                   *dto.TokenResponse
	UserToken                                string
	UserTokenDeadline, RefreshCookieDeadline time.Time
	RememberMe                               bool
}

type OIDCProfileCompletionService struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewOIDCProfileCompletionService(db *gorm.DB, client *redis.Client) *OIDCProfileCompletionService {
	return &OIDCProfileCompletionService{db: db, redis: client}
}

func boundedTokenDeadline(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// Complete replaces only a browser-bound, already authenticated login. Profile
// persistence and revocation commit together. The replacement stays unusable
// until its OIDC binding and all credentials have been prepared successfully.
func (s *OIDCProfileCompletionService) Complete(ctx context.Context, expected *OIDCSession, input ProfileCompletionInput) (*ProfileCompletionResult, error) {
	if expected == nil || expected.UserID == nil || !expected.IsAuthenticated || expected.LoginSessionID == "" {
		return nil, ErrProfileCompletionConflict
	}
	sessions := NewOIDCSessionService(s.redis)
	if err := sessions.CheckProfileSession(ctx, expected); err != nil {
		return nil, ErrProfileCompletionConflict
	}
	jwtService, err := NewJWTAuthServiceWithError()
	if err != nil {
		return nil, err
	}
	var pending *model.LoginSession
	consumed := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, *expected.UserID); err != nil {
			return err
		}
		if err := dao.NewLoginSessionDAO(tx).Lock(expected.LoginSessionID, *expected.UserID); err != nil {
			return ErrProfileCompletionConflict
		}
		old, err := dao.NewLoginSessionDAO(tx).Valid(expected.LoginSessionID, *expected.UserID)
		if err != nil {
			return ErrProfileCompletionConflict
		}
		if err := sessions.CheckProfileSession(ctx, expected); err != nil {
			return ErrProfileCompletionConflict
		}
		var user model.User
		if err := tx.First(&user, old.UserID).Error; err != nil {
			return err
		}
		app, err := NewApplicationService(tx).GetApplicationByClientID(expected.ClientID)
		if err != nil {
			return err
		}
		if _, err := NewOIDCAuthorizationService(tx).evaluateOIDCGrant(app, user.ID, strings.Fields(expected.Scope), false); err != nil {
			return err
		}
		checks, updates, err := profileCompletionUpdates(&user, expected.Scope, input)
		if err != nil {
			return err
		}
		if user.TwoFactorEnabled {
			factors, err := dao.NewTwoFactorDAO(tx).Methods(user.ID)
			if err != nil {
				return err
			}
			for _, check := range checks {
				for _, factor := range factors {
					if factor.Method == string(check.Type) && factor.Receiver != updates[string(check.Type)] {
						return fmt.Errorf("%w：请在安全设置中验证身份后修改已绑定的联系方式", ErrProfileCompletionInvalid)
					}
				}
			}
		}
		users := NewUserService(tx)
		for _, check := range checks {
			value := updates[string(check.Type)].(string)
			used, err := users.ContactInUse(check.Type, value, user.ID)
			if err != nil {
				return err
			}
			if used {
				return fmt.Errorf("%w：邮箱或手机号已被其他用户使用", ErrProfileCompletionInvalid)
			}
		}
		if err := NewVerificationCodeService(s.redis, nil).VerifyCodes(ctx, expected.SessionID, checks); err != nil {
			return err
		}
		consumed = true
		version := user.TwoFactorVersion + 1
		updates["two_factor_version"] = version
		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return err
		}
		if err := RevokeLoginSessions(tx, user.ID, "", "", "contact_changed", &user.ID); err != nil {
			return err
		}
		copy := *old
		copy.ID = uuid.NewString()
		copy.NativeTransactionHash = nil
		copy.ActivationPending = true
		copy.HasAdminAccess = false
		copy.TwoFactorVersion = version
		copy.CreatedAt, copy.LastActiveAt = time.Now(), time.Now()
		copy.EndedAt, copy.EndedBy, copy.EndReason = nil, nil, ""
		remember := old.RememberMe != nil && *old.RememberMe
		copy.RememberMe = &remember
		if err := tx.Create(&copy).Error; err != nil {
			return err
		}
		pending = &copy
		return nil
	})
	if err != nil {
		if consumed {
			helper.GetLogger().Error(fmt.Sprintf("[oidc-profile] stage=save session=%s err=%v", expected.SessionID, err))
			return nil, errors.New("保存资料失败，请重新获取验证码后重试")
		}
		return nil, err
	}
	result := &ProfileCompletionResult{Session: pending, RememberMe: *pending.RememberMe}
	fail := func(stage string, cause error) (*ProfileCompletionResult, error) {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		cleanupErr := NewLoginSessionService(s.db.WithContext(cleanup)).Revoke(pending.UserID, pending.ID, "", "profile_completion_failed", nil)
		helper.GetLogger().Error(fmt.Sprintf("[oidc-profile] stage=%s session=%s replacement=%s err=%v cleanupErr=%v", stage, expected.SessionID, pending.ID, cause, cleanupErr))
		return nil, ErrProfileCompletionRelogin
	}
	if err := sessions.RebindProfileSession(ctx, expected, pending.ID); err != nil {
		return fail("bind", err)
	}
	rebound := *expected
	rebound.LoginSessionID = pending.ID
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, pending.UserID); err != nil {
			return err
		}
		var current model.LoginSession
		if err := tx.Where("id = ? AND user_id = ? AND activation_pending = ? AND ended_at IS NULL AND expires_at > ?", pending.ID, pending.UserID, true, time.Now()).First(&current).Error; err != nil {
			return err
		}
		if err := sessions.CheckProfileSession(ctx, &rebound); err != nil {
			return err
		}
		// Validate the inherited assurance against the current user configuration;
		// a concurrent administrator revocation/configuration change wins.
		current.ActivationPending = false
		if err := dao.ValidateSessionAssurance(tx, &current); err != nil {
			return err
		}
		if err := tx.Model(&current).Update("activation_pending", false).Error; err != nil {
			return err
		}
		app, err := NewApplicationService(tx).GetApplicationByClientID(expected.ClientID)
		if err != nil {
			return err
		}
		deadline := current.ExpiresAt
		refreshDeadline := boundedTokenDeadline(deadline, input.RefreshDeadline)
		limits := loginTokenLimits{Access: deadline, Refresh: refreshDeadline, ID: deadline, AuthTime: &expected.AuthTime}
		result.Tokens, err = NewOIDCAuthorizationService(tx).generateTokensLocked(app, current.UserID, strings.Fields(expected.Scope), expected.Nonce, input.Issuer, current.ID, nil, limits)
		if err != nil {
			return err
		}
		user, err := NewUserService(tx).GetUserModelByID(current.UserID)
		if err != nil {
			return err
		}
		result.UserTokenDeadline = boundedTokenDeadline(boundedTokenDeadline(time.Now().Add(jwtService.GetConfig().AccessTokenTTL), deadline), input.UserTokenDeadline)
		result.UserToken, err = jwtService.GetJWTService().GenerateTokenUntil(jwt.TokenSubject{UserID: user.ID, Username: user.Username, Role: "user", SessionID: current.ID}, result.UserTokenDeadline)
		if err != nil {
			return err
		}
		result.RefreshCookieDeadline = boundedTokenDeadline(boundedTokenDeadline(time.Now().Add(7*24*time.Hour), refreshDeadline), input.RefreshCookieDeadline)
		result.Session = &current
		return nil
	})
	if err != nil {
		return fail("activate", err)
	}
	helper.GetLogger().Info(fmt.Sprintf("[oidc-profile] stage=completed session=%s replacement=%s", expected.SessionID, pending.ID))
	return result, nil
}

func profileCompletionUpdates(user *model.User, scope string, input ProfileCompletionInput) ([]VerificationCodeCheck, map[string]interface{}, error) {
	allowed := map[string]bool{}
	for _, name := range strings.Fields(scope) {
		allowed[name] = true
	}
	checks := []VerificationCodeCheck{}
	updates := map[string]interface{}{}
	for _, contact := range []struct {
		kind      VerificationType
		raw, code string
		existing  *string
	}{
		{VerificationTypeEmail, input.Email, input.EmailCode, user.Email},
		{VerificationTypePhone, input.Phone, input.PhoneCode, user.Phone},
	} {
		if contact.raw == "" {
			if contact.code != "" {
				return nil, nil, ErrProfileCompletionInvalid
			}
			continue
		}
		if !allowed[string(contact.kind)] {
			return nil, nil, fmt.Errorf("%w：当前授权不需要该联系方式", ErrProfileCompletionInvalid)
		}
		if stringPointerValue(contact.existing) != "" {
			return nil, nil, ErrProfileCompletionConflict
		}
		value := strings.TrimSpace(contact.raw)
		if contact.kind == VerificationTypeEmail {
			address, err := mail.ParseAddress(value)
			if err != nil || address.Address != value || len(value) > 100 {
				return nil, nil, fmt.Errorf("%w：邮箱格式错误", ErrProfileCompletionInvalid)
			}
		} else {
			var err error
			value, err = utils.NormalizePhoneToE164(value, utils.DefaultPhoneRegion)
			if err != nil {
				return nil, nil, fmt.Errorf("%w：手机号格式错误", ErrProfileCompletionInvalid)
			}
		}
		// Match the existing send-code key exactly; only persisted values are normalized.
		checks = append(checks, VerificationCodeCheck{Type: contact.kind, Receiver: contact.raw, Code: contact.code})
		updates[string(contact.kind)] = value
		updates[string(contact.kind)+"_verified_at"] = time.Now()
	}
	if len(checks) == 0 {
		return nil, nil, ErrProfileCompletionInvalid
	}
	return checks, updates, nil
}
