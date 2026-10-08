package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/png"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type twoFactorOperation struct {
	UserID          uint
	Version         uint64
	SessionID       string
	Kind            string
	CredentialStamp string
	Method          string
	At              time.Time
}
type TwoFactorEnrollment struct {
	TwoFactorChallenge *dto.TwoFactorChallenge `json:"twoFactorChallenge"`
	Secret             string                  `json:"secret,omitempty"`
	OTPAuthURI         string                  `json:"otpAuthUri,omitempty"`
	QRCode             string                  `json:"qrCode,omitempty"`
}

func (s *TwoFactorService) Reauthenticate(ctx context.Context, userID uint, sid, password, binding, kind string) (string, *TwoFactorTransaction, error) {
	u, err := s.user(userID)
	if err != nil {
		return "", nil, err
	}
	session, err := dao.NewLoginSessionDAO(s.db).Valid(sid, userID)
	if err != nil {
		return "", nil, err
	}
	if kind == "admin" && !session.HasAdminAccess {
		return "", nil, ErrTwoFactorExpired
	}
	if u.TwoFactorEnabled {
		tr, e := s.newTransaction(ctx, u, "manage", kind, "", "", sid, false, binding, nil)
		return "", tr, e
	}
	if u.PasswordHash != "" {
		profile, e := NewUserService(s.db).ValidateUserForLogin(u.Username, password)
		if e != nil || profile.ID != u.ID {
			return "", nil, errors.New("当前密码错误")
		}
	} else if s.now().Sub(session.CreatedAt) > twoFactorTTL {
		return "", nil, errors.New("请通过原应用重新登录，并在 5 分钟内返回本页确认身份")
	}
	op, err := s.Operation(ctx, u.ID, sid, kind, &TwoFactorProof{userID: u.ID, version: u.TwoFactorVersion, method: session.LoginMethod, at: s.now()})
	return op, nil, err
}

func (s *TwoFactorService) Operation(ctx context.Context, userID uint, sid, kind string, proof *TwoFactorProof) (string, error) {
	u, err := s.user(userID)
	if err != nil {
		return "", err
	}
	if !proof.validFor(u) {
		return "", ErrTwoFactorExpired
	}
	if _, err = dao.NewLoginSessionDAO(s.db).Valid(sid, userID); err != nil {
		return "", err
	}
	token, err := twoFactorRandom()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(twoFactorOperation{UserID: userID, Version: u.TwoFactorVersion, SessionID: sid, Kind: kind, CredentialStamp: twoFactorCredentialStamp(u), Method: proof.method, At: proof.at})
	if err != nil {
		return "", err
	}
	if s.redis == nil {
		return "", errors.New("二次验证服务不可用")
	}
	return token, s.redis.Set(ctx, "2fa:operation:"+twoFactorHash(token), data, twoFactorTTL).Err()
}

func (s *TwoFactorService) consumeOperation(ctx context.Context, token string, userID uint, sid, kind string) (*twoFactorOperation, error) {
	if len(token) != 43 || s.redis == nil {
		return nil, ErrTwoFactorExpired
	}
	key := "2fa:operation:" + twoFactorHash(token)
	raw, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, ErrTwoFactorExpired
	}
	var op twoFactorOperation
	if json.Unmarshal(raw, &op) != nil || op.UserID != userID || op.SessionID != sid || op.Kind != kind || s.now().Sub(op.At) > twoFactorTTL {
		return nil, ErrTwoFactorExpired
	}
	if _, err = dao.NewLoginSessionDAO(s.db).Valid(sid, userID); err != nil {
		return nil, err
	}
	u, err := s.user(userID)
	if err != nil {
		return nil, err
	}
	if u.TwoFactorVersion != op.Version || twoFactorCredentialStamp(u) != op.CredentialStamp {
		return nil, ErrTwoFactorExpired
	}
	n, err := twoFactorConsume.Run(ctx, s.redis, []string{key}, string(raw)).Int()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, ErrTwoFactorExpired
	}
	return &op, nil
}

func (s *TwoFactorService) Enroll(ctx context.Context, userID uint, sid, token, method, binding string) (*TwoFactorEnrollment, error) {
	if method != "totp" && method != "phone" && method != "email" {
		return nil, errors.New("不支持的验证方式")
	}
	op, err := s.consumeOperation(ctx, token, userID, sid, "user")
	if err != nil {
		return nil, err
	}
	u, err := s.user(userID)
	if err != nil {
		return nil, err
	}
	if u.TwoFactorVersion != op.Version {
		return nil, ErrTwoFactorExpired
	}
	factor := &model.TwoFactorMethod{UserID: userID, Method: method, CreatedAt: s.now()}
	result := &TwoFactorEnrollment{}
	if method == "totp" {
		cipher, err := credentialcrypto.FromEnvironment()
		if err != nil {
			return nil, err
		}
		key, err := totp.Generate(totp.GenerateOpts{Issuer: "MLiev IAM", AccountName: u.Username, Period: 30, SecretSize: 20, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if err != nil {
			return nil, err
		}
		envelope, err := cipher.Encrypt([]byte(key.Secret()), credentialcrypto.AAD(userID, "two_factor_totp", cipher.ActiveKeyVersion()))
		if err != nil {
			return nil, err
		}
		factor.Ciphertext, factor.Nonce, factor.Algorithm, factor.KeyVersion = envelope.Ciphertext, envelope.Nonce, envelope.Algorithm, envelope.KeyVersion
		image, err := key.Image(240, 240)
		if err != nil {
			return nil, err
		}
		var buffer bytes.Buffer
		if err = png.Encode(&buffer, image); err != nil {
			return nil, err
		}
		result.Secret, result.OTPAuthURI, result.QRCode = key.Secret(), key.URL(), "data:image/png;base64,"+base64.StdEncoding.EncodeToString(buffer.Bytes())
	} else {
		factor.Receiver = verifiedFactorReceiver(u, method)
		if factor.Receiver == "" || !s.channelAvailable(method) {
			return nil, errors.New("请先验证联系方式，并配置对应的消息推送渠道")
		}
	}
	tr, err := s.newTransaction(ctx, u, "enroll", "user", "", "", sid, false, binding, factor)
	if err != nil {
		return nil, err
	}
	result.TwoFactorChallenge, err = s.View(ctx, tr)
	return result, err
}

func (s *TwoFactorService) ConfirmEnrollment(ctx context.Context, tr *TwoFactorTransaction, proof *TwoFactorProof, ip string) ([]string, *TwoFactorProof, error) {
	if tr.Pending == nil || tr.Purpose != "enroll" {
		return nil, nil, ErrTwoFactorExpired
	}
	var codes []string
	var renewed *TwoFactorProof
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, tr.UserID); err != nil {
			return err
		}
		var u model.User
		if err := tx.First(&u, tr.UserID).Error; err != nil {
			return err
		}
		if !u.CanLogin() || !proof.validFor(&u) || twoFactorCredentialStamp(&u) != tr.CredentialStamp {
			return ErrTwoFactorExpired
		}
		if _, err := dao.NewLoginSessionDAO(tx).Valid(tr.ParentSessionID, u.ID); err != nil {
			return err
		}
		if !s.methodAvailable(&u, tr.Pending.factor()) {
			return ErrTwoFactorExpired
		}
		factor := tr.Pending.factor()
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "method"}}, UpdateAll: true}).Create(&factor).Error; err != nil {
			return err
		}
		if !u.TwoFactorEnabled {
			var err error
			codes, err = replaceRecoveryCodes(tx, u.ID)
			if err != nil {
				return err
			}
		}
		u.TwoFactorEnabled = true
		var err error
		renewed, err = s.changeConfiguration(tx, &u, u.ID, "enroll", tr.Pending.Method, "", ip, proof.method)
		return err
	})
	return codes, renewed, err
}

func (s *TwoFactorService) changeConfiguration(tx *gorm.DB, u *model.User, actor uint, action, method, reason, ip, proofMethod string) (*TwoFactorProof, error) {
	u.TwoFactorVersion++
	if err := tx.Model(u).Updates(map[string]any{"two_factor_enabled": u.TwoFactorEnabled, "two_factor_version": u.TwoFactorVersion}).Error; err != nil {
		return nil, err
	}
	if err := RevokeLoginSessions(tx, u.ID, "", "", "two_factor_changed", &actor); err != nil {
		return nil, err
	}
	if err := tx.Create(&model.TwoFactorEvent{UserID: u.ID, ActorID: actor, Action: action, Method: method, Reason: reason, IP: ip, CreatedAt: s.now()}).Error; err != nil {
		return nil, err
	}
	return &TwoFactorProof{userID: u.ID, version: u.TwoFactorVersion, method: proofMethod, at: s.now()}, nil
}

func replaceRecoveryCodes(tx *gorm.DB, userID uint) ([]string, error) {
	if err := tx.Where("user_id = ?", userID).Delete(&model.TwoFactorRecoveryCode{}).Error; err != nil {
		return nil, err
	}
	codes := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		text := strings.ToUpper(hex.EncodeToString(raw))
		code := text[:8] + "-" + text[8:16] + "-" + text[16:24] + "-" + text[24:]
		codes = append(codes, code)
		if err := tx.Create(&model.TwoFactorRecoveryCode{UserID: userID, CodeHash: twoFactorHash(text)}).Error; err != nil {
			return nil, err
		}
	}
	return codes, nil
}

func (s *TwoFactorService) Mutate(ctx context.Context, userID uint, sid, token, action, method, ip string, confirmDisable bool) ([]string, *TwoFactorProof, error) {
	op, err := s.consumeOperation(ctx, token, userID, sid, "user")
	if err != nil {
		return nil, nil, err
	}
	var codes []string
	var proof *TwoFactorProof
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, userID); err != nil {
			return err
		}
		var u model.User
		if err := tx.First(&u, userID).Error; err != nil {
			return err
		}
		if !u.CanLogin() || u.TwoFactorVersion != op.Version || twoFactorCredentialStamp(&u) != op.CredentialStamp {
			return ErrTwoFactorExpired
		}
		if _, err := dao.NewLoginSessionDAO(tx).Valid(sid, userID); err != nil {
			return err
		}
		switch action {
		case "remove":
			if method != "totp" && method != "phone" && method != "email" {
				return errors.New("不支持的验证方式")
			}
			var count int64
			if err := tx.Model(&model.TwoFactorMethod{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
				return err
			}
			if count <= 1 && !confirmDisable {
				return errors.New("移除最后一种方式会关闭 2FA，请确认关闭")
			}
			if err := tx.Where("user_id = ? AND method = ?", userID, method).Delete(&model.TwoFactorMethod{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.TwoFactorMethod{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
				return err
			}
			u.TwoFactorEnabled = count > 0
		case "disable":
			if !confirmDisable {
				return errors.New("请确认关闭 2FA")
			}
			u.TwoFactorEnabled = false
			if err := tx.Where("user_id = ?", userID).Delete(&model.TwoFactorMethod{}).Error; err != nil {
				return err
			}
		case "regenerate":
			if !u.TwoFactorEnabled {
				return errors.New("请先开启 2FA")
			}
			var err error
			codes, err = replaceRecoveryCodes(tx, userID)
			if err != nil {
				return err
			}
		default:
			return errors.New("不支持的操作")
		}
		if !u.TwoFactorEnabled {
			if err := tx.Where("user_id = ?", userID).Delete(&model.TwoFactorRecoveryCode{}).Error; err != nil {
				return err
			}
		}
		var err error
		proof, err = s.changeConfiguration(tx, &u, userID, action, method, "", ip, op.Method)
		return err
	})
	return codes, proof, err
}

func (s *TwoFactorService) Reset(ctx context.Context, actor, userID uint, sid, token, reason, ip string) error {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 3 || len([]rune(reason)) > 500 {
		return errors.New("请填写 3 至 500 字的身份核验及重置原因")
	}
	if actor == userID {
		return errors.New("请在个人安全设置中关闭自己的 2FA")
	}
	if _, err := s.consumeOperation(ctx, token, actor, sid, "admin"); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, actor); err != nil {
			return err
		}
		session, err := dao.NewLoginSessionDAO(tx).Valid(sid, actor)
		if err != nil || !session.HasAdminAccess {
			return ErrTwoFactorExpired
		}
		admin, err := IsSuperAdmin(tx, actor)
		if err != nil {
			return err
		}
		if !admin {
			return errors.New("无权重置 2FA")
		}
		if err = dao.LockLoginUser(tx, userID); err != nil {
			return err
		}
		var u model.User
		if err = tx.First(&u, userID).Error; err != nil {
			return err
		}
		if err = tx.Where("user_id = ?", userID).Delete(&model.TwoFactorMethod{}).Error; err != nil {
			return err
		}
		if err = tx.Where("user_id = ?", userID).Delete(&model.TwoFactorRecoveryCode{}).Error; err != nil {
			return err
		}
		u.TwoFactorEnabled = false
		_, err = s.changeConfiguration(tx, &u, actor, "admin_reset", "", reason, ip, "")
		return err
	})
}

// ChangeContact runs only after the controller has validated a code sent to the
// new address. Existing factors must also be reauthenticated before replacement.
func (s *TwoFactorService) ChangeContact(ctx context.Context, userID uint, sid, token string, method VerificationType, value, ip string) (*TwoFactorProof, error) {
	op, err := s.consumeOperation(ctx, token, userID, sid, "user")
	if err != nil {
		return nil, err
	}
	var proof *TwoFactorProof
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, userID); err != nil {
			return err
		}
		var u model.User
		if err := tx.First(&u, userID).Error; err != nil {
			return err
		}
		if !u.CanLogin() || u.TwoFactorVersion != op.Version || twoFactorCredentialStamp(&u) != op.CredentialStamp {
			return ErrTwoFactorExpired
		}
		if _, err := dao.NewLoginSessionDAO(tx).Valid(sid, userID); err != nil {
			return err
		}
		if err := NewUserService(tx).UpdateVerifiedContact(userID, method, value); err != nil {
			return err
		}
		if err := tx.Model(&model.TwoFactorMethod{}).Where("user_id = ? AND method = ?", userID, string(method)).Update("receiver", value).Error; err != nil {
			return err
		}
		var err error
		proof, err = s.changeConfiguration(tx, &u, userID, "contact_changed", string(method), "", ip, op.Method)
		return err
	})
	return proof, err
}
