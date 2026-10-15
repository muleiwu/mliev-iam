package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrTwoFactorRequired = errors.New("需要完成二次验证")
	ErrTwoFactorExpired  = errors.New("验证已失效，请重新登录或重新验证身份")
	ErrTwoFactorCode     = errors.New("验证码错误、已使用或已过期")
	ErrTwoFactorLimited  = errors.New("验证尝试过多，请稍后重试")
)

const twoFactorTTL = 5 * time.Minute

// TwoFactorProof can only be constructed by this package after verification.
// It is never serialized or accepted from an HTTP request.
type TwoFactorProof struct {
	userID           uint
	version          uint64
	method           string
	at               time.Time
	primaryPasskeyID uint
	stamp            string
}

func (p *TwoFactorProof) validFor(u *model.User) bool {
	return p != nil && p.userID == u.ID && p.version == u.TwoFactorVersion && p.method != "" && time.Since(p.at) < twoFactorTTL
}

// This private storage shape includes encrypted pending credentials in Redis.
// API responses always use the explicit DTO and never serialize this structure.
type twoFactorStoredFactor struct {
	UserID       uint
	Method       string
	Receiver     string
	Ciphertext   string
	Nonce        string
	Algorithm    string
	KeyVersion   int
	LastUsedStep int64
	CreatedAt    time.Time
}

func (f twoFactorStoredFactor) factor() model.TwoFactorMethod { return model.TwoFactorMethod(f) }

type TwoFactorTransaction struct {
	ContinuationID   string
	ContinuationCSRF string
	ID               string
	UserID           uint
	Version          uint64
	Purpose          string
	Kind             string
	PrimaryMethod    string
	PrimaryPasskeyID uint
	OIDCSessionID    string
	ParentSessionID  string
	RememberMe       bool
	BindingHash      string
	CredentialStamp  string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	Pending          *twoFactorStoredFactor
}

type TwoFactorService struct {
	db            *gorm.DB
	redis         *redis.Client
	config        *VerificationCodeConfig
	passkeyConfig *PasskeyConfig
	// send is injectable for integration tests; production uses the configured PushService.
	send func(context.Context, string, string, string, time.Duration) error
	now  func() time.Time
}

func NewTwoFactorService(db *gorm.DB, client *redis.Client, config *VerificationCodeConfig) *TwoFactorService {
	return &TwoFactorService{db: db, redis: client, config: config, now: time.Now}
}

func twoFactorRandom() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func twoFactorHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func twoFactorCredentialStamp(u *model.User) string {
	return twoFactorHash(fmt.Sprintf("%s:%s:%v:%s:%v:%s:%v", u.PasswordHash, u.Salt, u.SessionRevokedAt, stringPointerValue(u.Email), u.EmailVerifiedAt, stringPointerValue(u.Phone), u.PhoneVerifiedAt))
}

func (s *TwoFactorService) user(id uint) (*model.User, error) {
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		return nil, err
	}
	if !u.CanLogin() {
		return nil, ErrTwoFactorExpired
	}
	return &u, nil
}

func (s *TwoFactorService) channelAvailable(method string) bool {
	if s.send != nil {
		return true
	}
	return s.config != nil && s.config.BaseURL != "" && s.config.AppID != "" && s.config.AppSecret != "" && ((method == "phone" && s.config.SMSChannelID > 0) || (method == "email" && s.config.EmailChannelID > 0))
}

func verifiedFactorReceiver(u *model.User, method string) string {
	if method == "email" && u.EmailVerifiedAt != nil && u.Email != nil {
		return *u.Email
	}
	if method == "phone" && u.PhoneVerifiedAt != nil && u.Phone != nil {
		return *u.Phone
	}
	return ""
}

func (s *TwoFactorService) methodAvailable(u *model.User, f model.TwoFactorMethod) bool {
	if f.Method == "totp" {
		c, e := credentialcrypto.FromEnvironment()
		if e != nil {
			return false
		}
		_, e = c.Decrypt(factorEnvelope(f), credentialcrypto.AAD(u.ID, "two_factor_totp", f.KeyVersion))
		return e == nil
	}
	return f.Receiver != "" && f.Receiver == verifiedFactorReceiver(u, f.Method) && s.channelAvailable(f.Method)
}

func (s *TwoFactorService) Status(userID uint) (*dto.TwoFactorStatus, error) {
	u, err := s.user(userID)
	if err != nil {
		return nil, err
	}
	factors, err := dao.NewTwoFactorDAO(s.db).Methods(userID)
	if err != nil {
		return nil, err
	}
	status := &dto.TwoFactorStatus{Enabled: u.TwoFactorEnabled, HasPassword: u.PasswordHash != "", Methods: []dto.TwoFactorMethodInfo{}}
	for _, method := range []string{"password", "passkey", "totp", "phone", "email"} {
		info := dto.TwoFactorMethodInfo{Method: method}
		switch method {
		case "password":
			info.Enabled = u.PasswordHash != ""
			info.Available = info.Enabled
		case "passkey":
			cfg := s.passkeySettings()
			if cfg.Enabled {
				if _, e := cfg.WebAuthn(); e == nil {
					var count int64
					if err := s.db.Model(&model.Passkey{}).Where("user_id = ? AND rp_id = ? AND user_handle = ?", userID, cfg.RPID, u.UUID).Count(&count).Error; err != nil {
						return nil, err
					}
					info.Enabled = count > 0
					info.Available = info.Enabled
				}
			}
		case "totp":
			_, e := credentialcrypto.FromEnvironment()
			info.Available = e == nil
			for _, f := range factors {
				if f.Method == method {
					info.Enabled = true
					info.Available = s.methodAvailable(u, f)
				}
			}
		default:
			receiver := verifiedFactorReceiver(u, method)
			info.Enabled = receiver != ""
			info.Available = info.Enabled && s.channelAvailable(method)
			info.MaskedReceiver = MaskVerificationReceiver(receiver)
		}
		status.Methods = append(status.Methods, info)
	}

	// Computing eligibility also accounts for external-only accounts.
	_, status.CanEnable, err = s.loginPath(userID, status.Methods)
	if err != nil {
		return nil, err
	}
	err = s.db.Model(&model.TwoFactorRecoveryCode{}).Where("user_id = ? AND used_at IS NULL", userID).Count(&status.RecoveryCodesRemaining).Error
	return status, err
}

func (s *TwoFactorService) BeginLogin(ctx context.Context, userID uint, kind, primary, oidcID string, remember bool, binding string, continuation ...string) (*TwoFactorTransaction, error) {
	u, err := s.user(userID)
	if err != nil {
		return nil, err
	}
	if !u.TwoFactorEnabled {
		return nil, nil
	}
	tr, err := s.newTransaction(ctx, u, "login", kind, primary, oidcID, "", remember, binding, nil)
	if err != nil {
		return nil, err
	}
	if len(continuation) == 2 && continuation[0] != "" {
		tr.ContinuationID, tr.ContinuationCSRF = continuation[0], continuation[1]
		data, err := json.Marshal(tr)
		if err != nil {
			return nil, err
		}
		if err = s.redis.Set(ctx, "2fa:challenge:"+tr.ID, data, twoFactorTTL).Err(); err != nil {
			return nil, err
		}
	}
	return tr, nil
}

func (s *TwoFactorService) newTransaction(ctx context.Context, u *model.User, purpose, kind, primary, oidcID, parent string, remember bool, binding string, pending *model.TwoFactorMethod) (*TwoFactorTransaction, error) {
	if s.redis == nil || binding == "" {
		return nil, errors.New("二次验证服务不可用")
	}
	id, err := twoFactorRandom()
	if err != nil {
		return nil, err
	}
	tr := &TwoFactorTransaction{ID: id, UserID: u.ID, Version: u.TwoFactorVersion, Purpose: purpose, Kind: kind, PrimaryMethod: primary, OIDCSessionID: oidcID, ParentSessionID: parent, RememberMe: remember, BindingHash: twoFactorHash(binding), CredentialStamp: twoFactorCredentialStamp(u), CreatedAt: s.now(), ExpiresAt: s.now().Add(twoFactorTTL), Pending: nil}
	if pending != nil {
		stored := twoFactorStoredFactor(*pending)
		tr.Pending = &stored
	}
	data, err := json.Marshal(tr)
	if err != nil {
		return nil, err
	}
	if err = s.redis.Set(ctx, "2fa:challenge:"+id, data, twoFactorTTL).Err(); err != nil {
		return nil, err
	}
	return tr, nil
}

func (s *TwoFactorService) Read(ctx context.Context, id, binding string) (*TwoFactorTransaction, error) {
	if s.redis == nil || len(id) != 43 || binding == "" {
		return nil, ErrTwoFactorExpired
	}
	raw, err := s.redis.Get(ctx, "2fa:challenge:"+id).Bytes()
	if err != nil {
		return nil, ErrTwoFactorExpired
	}
	var tr TwoFactorTransaction
	if json.Unmarshal(raw, &tr) != nil || subtle.ConstantTimeCompare([]byte(tr.BindingHash), []byte(twoFactorHash(binding))) != 1 || !tr.ExpiresAt.After(s.now()) {
		return nil, ErrTwoFactorExpired
	}
	u, err := s.user(tr.UserID)
	if err != nil {
		return nil, err
	}
	if u.TwoFactorVersion != tr.Version || twoFactorCredentialStamp(u) != tr.CredentialStamp {
		return nil, ErrTwoFactorExpired
	}
	if tr.ParentSessionID != "" {
		if _, err = dao.NewLoginSessionDAO(s.db).Valid(tr.ParentSessionID, u.ID); err != nil {
			return nil, ErrTwoFactorExpired
		}
	}
	if tr.Purpose == "login" && !u.TwoFactorEnabled {
		return nil, ErrTwoFactorExpired
	}
	return &tr, nil
}

func (s *TwoFactorService) View(ctx context.Context, tr *TwoFactorTransaction) (*dto.TwoFactorChallenge, error) {
	u, err := s.user(tr.UserID)
	if err != nil {
		return nil, err
	}
	out := &dto.TwoFactorChallenge{ID: tr.ID, Purpose: tr.Purpose, ExpiresAt: tr.ExpiresAt.Unix(), Methods: []dto.TwoFactorMethodInfo{}}
	if tr.Pending != nil {
		out.Methods = append(out.Methods, dto.TwoFactorMethodInfo{Method: tr.Pending.Method, Available: s.methodAvailable(u, tr.Pending.factor()), MaskedReceiver: MaskVerificationReceiver(tr.Pending.Receiver)})
	} else {
		status, err := s.Status(tr.UserID)
		if err != nil {
			return nil, err
		}
		for _, m := range status.Methods {
			if m.Enabled && m.Method != tr.PrimaryMethod {
				out.Methods = append(out.Methods, m)
			}
		}
		if status.Enabled && status.RecoveryCodesRemaining > 0 {
			out.Methods = append(out.Methods, dto.TwoFactorMethodInfo{Method: "recovery", Enabled: true, Available: true})
		}
	}
	return out, nil
}

func (s *TwoFactorService) Cancel(ctx context.Context, id, binding string) error {
	if _, err := s.Read(ctx, id, binding); err != nil {
		return err
	}
	return s.redis.Del(ctx, "2fa:challenge:"+id).Err()
}

func (s *TwoFactorService) allowed(ctx context.Context, tr *TwoFactorTransaction, method string) error {
	view, err := s.View(ctx, tr)
	if err != nil {
		return err
	}
	for _, m := range view.Methods {
		if m.Method == method && m.Available {
			return nil
		}
	}
	return errors.New("该验证方式当前不可用，请选择其他方式或返回登录")
}

var twoFactorConsume = redis.NewScript(`if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`)

func (s *TwoFactorService) SendCode(ctx context.Context, tr *TwoFactorTransaction, method string) (int, error) {
	if method != "phone" && method != "email" {
		return 0, errors.New("该方式不支持发送验证码")
	}
	if err := s.allowed(ctx, tr, method); err != nil {
		return 0, err
	}
	u, err := s.user(tr.UserID)
	if err != nil {
		return 0, err
	}
	rateKey := fmt.Sprintf("2fa:send:%d:%s", u.ID, method)
	marker, err := twoFactorRandom()
	if err != nil {
		return 0, err
	}
	ok, err := s.redis.SetNX(ctx, rateKey, marker, time.Minute).Result()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 60, errors.New("发送过于频繁，请 60 秒后重试")
	}
	code, err := generateVerificationCode()
	if err != nil {
		return 0, err
	}
	key := "2fa:code:" + tr.ID + ":" + method
	ttl := tr.ExpiresAt.Sub(s.now())
	if ttl <= 0 {
		return 0, ErrTwoFactorExpired
	}
	hash := twoFactorHash(tr.ID + ":" + method + ":" + code)
	if err = s.redis.Set(ctx, key, hash, ttl).Err(); err == nil {
		receiver := verifiedFactorReceiver(u, method)
		if s.send != nil {
			err = s.send(ctx, method, receiver, code, ttl)
		} else {
			client, e := NewPushService(dto.PushCredentials{BaseURL: s.config.BaseURL, AppID: s.config.AppID, AppSecret: s.config.AppSecret})
			err = e
			if err == nil {
				id, signature, bindings := s.config.EmailChannelID, s.config.EmailSignature, s.config.EmailTemplateBindings
				if method == "phone" {
					id, signature, bindings = s.config.SMSChannelID, s.config.SMSSignature, s.config.SMSTemplateBindings
				}
				_, err = client.SendCode(ctx, id, signature, receiver, bindings, code, ttl)
			}
		}
	}
	if err != nil {
		_, _ = twoFactorConsume.Run(ctx, s.redis, []string{key}, hash).Result()
		_, _ = twoFactorConsume.Run(ctx, s.redis, []string{rateKey}, marker).Result()
		return 0, errors.New("验证码发送失败，请稍后重试或使用其他方式")
	}
	return 60, nil
}

// Reserve attempts atomically across challenges and nodes. Successful attempts
// release the account/IP reservations; failed attempts retain their 15m budget.
var twoFactorAttempts = redis.NewScript(`
for i=1,3 do if tonumber(redis.call('GET',KEYS[i]) or '0') >= tonumber(ARGV[i]) then return 0 end end
for i=1,3 do local n=redis.call('INCR',KEYS[i]); if n==1 then redis.call('EXPIRE',KEYS[i],ARGV[i+3]) end end
return 1`)

func (s *TwoFactorService) Verify(ctx context.Context, tr *TwoFactorTransaction, method, code, ip string) (*TwoFactorProof, error) {
	if method == "passkey" {
		return nil, errors.New("请使用 Passkey 验证接口")
	}
	return s.verifyWith(ctx, tr, method, code, ip, nil)
}

func (s *TwoFactorService) verifyWith(ctx context.Context, tr *TwoFactorTransaction, method, code, ip string, assertion func(*gorm.DB, *model.User) error) (*TwoFactorProof, error) {
	if s.redis == nil {
		return nil, ErrTwoFactorExpired
	}

	if err := s.allowed(ctx, tr, method); err != nil {
		return nil, err
	}
	lockKey := "2fa:verify-lock:" + tr.ID
	lockToken, err := twoFactorRandom()
	if err != nil {
		return nil, err
	}
	locked, err := s.redis.SetNX(ctx, lockKey, lockToken, twoFactorTTL).Result()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, errors.New("验证正在处理中，请稍后重试")
	}
	defer func() { _, _ = twoFactorConsume.Run(ctx, s.redis, []string{lockKey}, lockToken).Result() }()
	stored, err := s.redis.Get(ctx, "2fa:challenge:"+tr.ID).Bytes()
	expected, marshalErr := json.Marshal(tr)
	if err != nil || marshalErr != nil || string(stored) != string(expected) || !tr.ExpiresAt.After(s.now()) {
		return nil, ErrTwoFactorExpired
	}

	keys := []string{"2fa:attempt:challenge:" + tr.ID, fmt.Sprintf("2fa:attempt:user:%d", tr.UserID), "2fa:attempt:ip:" + twoFactorHash(ip)}
	ok, err := twoFactorAttempts.Run(ctx, s.redis, keys, 5, 10, 50, 300, 900, 900).Int()
	if err != nil {
		return nil, err
	}
	if ok != 1 {
		return nil, ErrTwoFactorLimited
	}
	if method == "email" || method == "phone" {
		n, e := twoFactorConsume.Run(ctx, s.redis, []string{"2fa:code:" + tr.ID + ":" + method}, twoFactorHash(tr.ID+":"+method+":"+code)).Int()
		if e != nil {
			return nil, e
		}
		if n != 1 {
			return nil, ErrTwoFactorCode
		}
	}
	// Lock the user before consuming a factor, matching session/config lock order.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, tr.UserID); err != nil {
			return err
		}
		var u model.User
		if err := tx.First(&u, tr.UserID).Error; err != nil {
			return err
		}
		if !u.CanLogin() || u.TwoFactorVersion != tr.Version || twoFactorCredentialStamp(&u) != tr.CredentialStamp {
			return ErrTwoFactorExpired
		}
		if tr.ParentSessionID != "" {
			if _, err := dao.NewLoginSessionDAO(tx).Valid(tr.ParentSessionID, u.ID); err != nil {
				return ErrTwoFactorExpired
			}
		}
		if tr.PrimaryPasskeyID != 0 {
			p := &PasskeyProof{userID: u.ID, credentialID: tr.PrimaryPasskeyID, version: tr.Version, stamp: tr.CredentialStamp, at: tr.CreatedAt}
			if err := p.validate(tx, &u); err != nil {
				return err
			}
		}
		if assertion != nil {
			return assertion(tx, &u)
		}
		if method == "password" {
			if u.PasswordHash == "" || !NewUserService(tx).verifyPassword(code, u.Salt, u.PasswordHash) {
				return errors.New("密码错误")
			}
			return nil
		}
		if method == "phone" || method == "email" {
			if verifiedFactorReceiver(&u, method) == "" || !s.channelAvailable(method) {
				return ErrTwoFactorExpired
			}
			return nil
		}
		if method == "recovery" {
			now := s.now()
			r := tx.Model(&model.TwoFactorRecoveryCode{}).Where("user_id = ? AND code_hash = ? AND used_at IS NULL", u.ID, twoFactorHash(normalizeRecoveryCode(code))).Update("used_at", now)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrTwoFactorCode
			}
			return nil
		}
		var f *model.TwoFactorMethod
		if tr.Pending != nil {
			copy := tr.Pending.factor()
			f = &copy
		} else {
			var e error
			f, e = dao.NewTwoFactorDAO(tx).Method(u.ID, method)
			if e != nil {
				return ErrTwoFactorCode
			}
		}
		if !s.methodAvailable(&u, *f) {
			return ErrTwoFactorExpired
		}
		if method == "totp" {
			step, err := s.verifyTOTP(*f, code)
			if err != nil {
				return err
			}
			if tr.Pending != nil {
				tr.Pending.LastUsedStep = step
				return nil
			}
			r := tx.Model(&model.TwoFactorMethod{}).Where("user_id = ? AND method = ? AND last_used_step < ?", u.ID, "totp", step).Update("last_used_step", step)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrTwoFactorCode
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Redis GETDEL prevents two correct submissions from completing one login twice.
	if _, err = s.redis.GetDel(ctx, "2fa:challenge:"+tr.ID).Result(); err != nil {
		return nil, ErrTwoFactorExpired
	}
	for _, key := range keys[1:] {
		_ = s.redis.Decr(ctx, key).Err()
	}
	return &TwoFactorProof{userID: tr.UserID, version: tr.Version, method: method, at: s.now(), primaryPasskeyID: tr.PrimaryPasskeyID, stamp: tr.CredentialStamp}, nil
}

func factorEnvelope(f model.TwoFactorMethod) credentialcrypto.Envelope {
	return credentialcrypto.Envelope{Ciphertext: f.Ciphertext, Nonce: f.Nonce, Algorithm: f.Algorithm, KeyVersion: f.KeyVersion}
}

func (s *TwoFactorService) verifyTOTP(f model.TwoFactorMethod, code string) (int64, error) {
	if len(code) != 6 {
		return 0, ErrTwoFactorCode
	}
	cipher, err := credentialcrypto.FromEnvironment()
	if err != nil {
		return 0, err
	}
	secret, err := cipher.Decrypt(factorEnvelope(f), credentialcrypto.AAD(f.UserID, "two_factor_totp", f.KeyVersion))
	if err != nil {
		return 0, err
	}
	step := s.now().Unix() / 30
	for _, offset := range []int64{0, -1, 1} {
		candidate := step + offset
		valid, err := totp.ValidateCustom(code, string(secret), time.Unix(candidate*30, 0), totp.ValidateOpts{Period: 30, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if err == nil && valid && candidate > f.LastUsedStep {
			return candidate, nil
		}
	}
	return 0, ErrTwoFactorCode
}

func normalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}
