package service

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/container"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/publicsuffix"
	"gorm.io/gorm"
)

var ErrPasskeyUnavailable = errors.New("Passkey 未启用或配置不可用")
var ErrPasskeyInvalid = errors.New("Passkey 验证失败，请重新尝试或选择其他认证方式")

type PasskeyConfig struct {
	Enabled     bool
	RPID        string
	Origins     []string
	DisplayName string
}

func ReadPasskeyConfig() PasskeyConfig {
	cfg, err := container.Get[gsr.Provider]()
	if err != nil || cfg == nil {
		return PasskeyConfig{}
	}
	return PasskeyConfig{Enabled: cfg.GetBool("passkey.enabled", false), RPID: cfg.GetString("passkey.rp_id", ""), Origins: cfg.GetStringSlice("passkey.rp_origins", nil), DisplayName: cfg.GetString("passkey.rp_display_name", "MLiev IAM")}
}

func (c PasskeyConfig) WebAuthn() (*webauthn.WebAuthn, error) {
	if !c.Enabled {
		return nil, ErrPasskeyUnavailable
	}
	if c.RPID == "" || c.RPID != strings.ToLower(c.RPID) || strings.ContainsAny(c.RPID, "/: *") || net.ParseIP(c.RPID) != nil || len(c.Origins) == 0 {
		return nil, fmt.Errorf("Passkey 必须配置固定 RP ID 和可信 Origin")
	}
	if c.RPID != "localhost" {
		if _, err := publicsuffix.EffectiveTLDPlusOne(c.RPID); err != nil {
			return nil, fmt.Errorf("Passkey RP ID 必须是有效域名")
		}
	}
	for _, origin := range c.Origins {
		u, err := url.Parse(origin)
		if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Host == "" || u.Opaque != "" {
			return nil, fmt.Errorf("Passkey Origin 必须是完整来源，不可包含路径")
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "localhost") {
			return nil, fmt.Errorf("Passkey Origin 必须使用 HTTPS（localhost 除外）")
		}
		if u.Hostname() != c.RPID && !strings.HasSuffix(u.Hostname(), "."+c.RPID) {
			return nil, fmt.Errorf("Passkey Origin 必须属于配置的 RP ID")
		}
	}
	return webauthn.New(&webauthn.Config{RPID: c.RPID, RPDisplayName: c.DisplayName, RPOrigins: c.Origins, RPAllowCrossOrigin: false, AttestationPreference: protocol.PreferNoAttestation, AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}})
}
func (c PasskeyConfig) AllowsOrigin(origin string) bool {
	for _, allowed := range c.Origins {
		if origin == allowed {
			return true
		}
	}
	return false
}
func (c PasskeyConfig) stamp() string { b, _ := json.Marshal(c); return twoFactorHash(string(b)) }

type passkeyUser struct {
	model.User
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte   { return []byte(u.UUID) }
func (u passkeyUser) WebAuthnName() string { return u.Username }
func (u passkeyUser) WebAuthnDisplayName() string {
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Username
}
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// PasskeyProof is internal evidence, never accepted from a request or serialized.
type PasskeyProof struct {
	userID       uint
	credentialID uint
	version      uint64
	stamp        string
	at           time.Time
}

func (p *PasskeyProof) UserID() uint { return p.userID }
func (p *PasskeyProof) validate(db *gorm.DB, u *model.User) error {
	if p == nil || p.userID != u.ID || p.version != u.TwoFactorVersion || p.stamp != twoFactorCredentialStamp(u) || time.Since(p.at) > twoFactorTTL {
		return ErrPasskeyInvalid
	}
	var count int64
	if err := db.Model(&model.Passkey{}).Where("id = ? AND user_id = ?", p.credentialID, u.ID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrPasskeyInvalid
	}
	return nil
}

type PasskeyTransaction struct {
	ID                   string
	Purpose              string
	BindingHash          string
	Origin               string
	ConfigStamp          string
	UserID               uint
	Version              uint64
	CredentialStamp      string
	ParentSessionID      string
	TwoFactorChallengeID string
	OIDCSessionID        string
	RememberMe           bool
	Name                 string
	CreatedAt            time.Time
	ExpiresAt            time.Time
	Session              webauthn.SessionData
}

type PasskeyOptions struct {
	TransactionID string `json:"transactionId"`
	PublicKey     any    `json:"publicKey"`
	ExpiresAt     int64  `json:"expiresAt"`
}
type PasskeyService struct {
	db      *gorm.DB
	redis   *redis.Client
	config  PasskeyConfig
	wa      *webauthn.WebAuthn
	factors *TwoFactorService
}

func NewPasskeyService(db *gorm.DB, client *redis.Client, cfg PasskeyConfig, factors *TwoFactorService) (*PasskeyService, error) {
	wa, err := cfg.WebAuthn()
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrPasskeyUnavailable
	}
	return &PasskeyService{db: db, redis: client, config: cfg, wa: wa, factors: factors}, nil
}
func (s *PasskeyService) user(db *gorm.DB, id uint) (*passkeyUser, error) {
	var user model.User
	if err := db.First(&user, id).Error; err != nil {
		return nil, err
	}
	if !user.CanLogin() || user.UUID == "" {
		return nil, ErrPasskeyInvalid
	}
	records, err := dao.NewPasskeyDAO(db).List(id)
	if err != nil {
		return nil, err
	}
	u := &passkeyUser{User: user}
	for _, r := range records {
		if r.RPID != s.config.RPID || r.UserHandle != user.UUID {
			continue
		}
		var cred webauthn.Credential
		if json.Unmarshal([]byte(r.CredentialData), &cred) != nil {
			return nil, ErrPasskeyInvalid
		}
		u.credentials = append(u.credentials, cred)
	}
	return u, nil
}
func (s *PasskeyService) save(ctx context.Context, tr *PasskeyTransaction, binding, origin string, options any) (*PasskeyOptions, error) {
	if binding == "" || !s.config.AllowsOrigin(origin) {
		return nil, ErrPasskeyInvalid
	}
	id, err := twoFactorRandom()
	if err != nil {
		return nil, err
	}
	tr.ID = id
	tr.BindingHash = twoFactorHash(binding)
	tr.Origin = origin
	tr.ConfigStamp = s.config.stamp()
	tr.CreatedAt = time.Now()
	tr.ExpiresAt = tr.CreatedAt.Add(twoFactorTTL)
	tr.Session.Expires = tr.ExpiresAt
	raw, err := json.Marshal(tr)
	if err != nil {
		return nil, err
	}
	if err = s.redis.Set(ctx, "passkey:transaction:"+id, raw, twoFactorTTL).Err(); err != nil {
		return nil, err
	}
	return &PasskeyOptions{TransactionID: id, PublicKey: options, ExpiresAt: tr.ExpiresAt.Unix()}, nil
}
func (s *PasskeyService) Read(ctx context.Context, id, binding, origin, purpose string) (*PasskeyTransaction, error) {
	if len(id) != 43 || binding == "" || !s.config.AllowsOrigin(origin) {
		return nil, ErrPasskeyInvalid
	}
	raw, err := s.redis.Get(ctx, "passkey:transaction:"+id).Bytes()
	if err != nil {
		return nil, ErrPasskeyInvalid
	}
	var tr PasskeyTransaction
	if json.Unmarshal(raw, &tr) != nil || tr.Purpose != purpose || tr.Origin != origin || tr.ConfigStamp != s.config.stamp() || !tr.ExpiresAt.After(time.Now()) || subtle.ConstantTimeCompare([]byte(tr.BindingHash), []byte(twoFactorHash(binding))) != 1 {
		return nil, ErrPasskeyInvalid
	}
	return &tr, nil
}
func (s *PasskeyService) consume(ctx context.Context, tr *PasskeyTransaction) error {
	raw, err := json.Marshal(tr)
	if err != nil {
		return err
	}
	n, err := twoFactorConsume.Run(ctx, s.redis, []string{"passkey:transaction:" + tr.ID}, string(raw)).Int()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrPasskeyInvalid
	}
	return nil
}
func PasskeyRequestLimit(ctx context.Context, client *redis.Client, binding, ip string) error {
	if client == nil || binding == "" {
		return ErrPasskeyUnavailable
	}
	ok, err := twoFactorAttempts.Run(ctx, client, []string{"passkey:rate:browser:" + twoFactorHash(binding), "passkey:rate:ip:" + twoFactorHash(ip), "passkey:rate:pair:" + twoFactorHash(binding+":"+ip)}, 30, 60, 30, 60, 60, 60).Int()
	if err != nil {
		return err
	}
	if ok != 1 {
		return ErrTwoFactorLimited
	}
	return nil
}
func validPasskeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", errors.New("Passkey 名称须为 1 至 100 字")
	}
	return name, nil
}
func (s *PasskeyService) BeginRegistration(ctx context.Context, userID uint, sid, operation, name, binding, origin string) (*PasskeyOptions, error) {
	name, err := validPasskeyName(name)
	if err != nil {
		return nil, err
	}
	op, err := s.factors.consumeOperation(ctx, operation, userID, sid, "user")
	if err != nil {
		return nil, err
	}
	u, err := s.user(s.db, userID)
	if err != nil {
		return nil, err
	}
	if u.TwoFactorVersion != op.Version || twoFactorCredentialStamp(&u.User) != op.CredentialStamp {
		return nil, ErrTwoFactorExpired
	}
	exclusions := []protocol.CredentialDescriptor{}
	for _, cred := range u.credentials {
		exclusions = append(exclusions, cred.Descriptor())
	}
	options, session, err := s.wa.BeginRegistration(u, webauthn.WithExclusions(exclusions), webauthn.WithExtensions(webauthn.WithExtensionCredProps()), webauthn.WithRegistrationOrigin(origin))
	if err != nil {
		return nil, err
	}
	tr := &PasskeyTransaction{Purpose: "registration", UserID: userID, Version: op.Version, CredentialStamp: op.CredentialStamp, ParentSessionID: sid, Name: name, Session: *session}
	return s.save(ctx, tr, binding, origin, options.Response)
}
func (s *PasskeyService) FinishRegistration(ctx context.Context, tr *PasskeyTransaction, userID uint, sid string, raw []byte, ip string) (*TwoFactorProof, error) {
	if tr.Purpose != "registration" || tr.UserID != userID || tr.ParentSessionID != sid {
		return nil, ErrPasskeyInvalid
	}
	if err := s.consume(ctx, tr); err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	if err != nil {
		return nil, ErrPasskeyInvalid
	}
	var proof *TwoFactorProof
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, userID); err != nil {
			return err
		}
		u, err := s.user(tx, userID)
		if err != nil {
			return err
		}
		if u.TwoFactorVersion != tr.Version || twoFactorCredentialStamp(&u.User) != tr.CredentialStamp {
			return ErrTwoFactorExpired
		}
		if _, err = dao.NewLoginSessionDAO(tx).Valid(sid, userID); err != nil {
			return err
		}
		cred, err := s.wa.CreateCredential(u, tr.Session, parsed)
		if err != nil {
			return ErrPasskeyInvalid
		}
		data, err := json.Marshal(cred)
		if err != nil {
			return err
		}
		item := model.Passkey{UserID: userID, RPID: s.config.RPID, UserHandle: u.UUID, CredentialIDHash: dao.PasskeyCredentialHash(cred.ID), CredentialID: base64.RawURLEncoding.EncodeToString(cred.ID), CredentialData: string(data), Name: tr.Name, CreatedAt: time.Now()}
		if err = tx.Create(&item).Error; err != nil {
			return errors.New("无法保存 Passkey，该凭据可能已绑定")
		}
		proof, err = s.factors.changeConfiguration(tx, &u.User, userID, "passkey_added", "passkey", "", ip, "passkey")
		return err
	})
	return proof, err
}
func (s *PasskeyService) BeginLogin(ctx context.Context, binding, origin, oidcID string, remember bool) (*PasskeyOptions, error) {
	options, session, err := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired), webauthn.WithLoginOrigin(origin))
	if err != nil {
		return nil, err
	}
	return s.save(ctx, &PasskeyTransaction{Purpose: "login", OIDCSessionID: oidcID, RememberMe: remember, Session: *session}, binding, origin, options.Response)
}
func (s *PasskeyService) assertion(tx *gorm.DB, tr *PasskeyTransaction, parsed *protocol.ParsedCredentialAssertionData, expected uint) (*PasskeyProof, error) {
	record, err := dao.NewPasskeyDAO(tx).Credential(s.config.RPID, parsed.RawID)
	if err != nil {
		return nil, ErrPasskeyInvalid
	}
	if expected != 0 && record.UserID != expected {
		return nil, ErrPasskeyInvalid
	}
	if err = dao.LockLoginUser(tx, record.UserID); err != nil {
		return nil, err
	}
	// Re-read after the shared user lock, including concurrent deletion and counter updates.
	record, err = dao.NewPasskeyDAO(tx).Credential(s.config.RPID, parsed.RawID)
	if err != nil {
		return nil, ErrPasskeyInvalid
	}
	u, err := s.user(tx, record.UserID)
	if err != nil {
		return nil, ErrPasskeyInvalid
	}
	if record.UserHandle != u.UUID {
		return nil, ErrPasskeyInvalid
	}
	if expected != 0 && (u.TwoFactorVersion != tr.Version || twoFactorCredentialStamp(&u.User) != tr.CredentialStamp) {
		return nil, ErrTwoFactorExpired
	}
	var credential *webauthn.Credential
	if tr.Purpose == "login" {
		credential, err = s.wa.ValidateDiscoverableLogin(func(rawID, handle []byte) (webauthn.User, error) {
			if string(handle) != u.UUID || base64.RawURLEncoding.EncodeToString(rawID) != record.CredentialID {
				return nil, ErrPasskeyInvalid
			}
			return u, nil
		}, tr.Session, parsed)
	} else {
		credential, err = s.wa.ValidateLogin(u, tr.Session, parsed)
	}
	if err != nil || credential.Authenticator.CloneWarning {
		return nil, ErrPasskeyInvalid
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return nil, err
	}
	if err = tx.Model(record).Updates(map[string]any{"credential_data": string(data), "last_used_at": time.Now()}).Error; err != nil {
		return nil, err
	}
	return &PasskeyProof{userID: u.ID, credentialID: record.ID, version: u.TwoFactorVersion, stamp: twoFactorCredentialStamp(&u.User), at: time.Now()}, nil
}
func (s *PasskeyService) FinishLogin(ctx context.Context, tr *PasskeyTransaction, raw []byte) (*PasskeyProof, error) {
	if tr.Purpose != "login" {
		return nil, ErrPasskeyInvalid
	}
	if err := s.consume(ctx, tr); err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(raw)
	if err != nil {
		return nil, ErrPasskeyInvalid
	}
	// Resolve the owner before the transaction so its first snapshot read follows
	// the user lock, including on MySQL with REPEATABLE READ isolation.
	owner, err := dao.NewPasskeyDAO(s.db).Credential(s.config.RPID, parsed.RawID)
	if err != nil {
		return nil, ErrPasskeyInvalid
	}
	var proof *PasskeyProof
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, owner.UserID); err != nil {
			return err
		}
		var e error
		proof, e = s.assertion(tx, tr, parsed, 0)
		return e
	})
	return proof, err
}
func (s *PasskeyService) BeginSecondFactor(ctx context.Context, tr *TwoFactorTransaction, binding, origin string) (*PasskeyOptions, error) {
	if err := s.factors.allowed(ctx, tr, "passkey"); err != nil {
		return nil, err
	}
	u, err := s.user(s.db, tr.UserID)
	if err != nil {
		return nil, err
	}
	options, session, err := s.wa.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired), webauthn.WithLoginOrigin(origin))
	if err != nil {
		return nil, err
	}
	return s.save(ctx, &PasskeyTransaction{Purpose: "second", UserID: u.ID, Version: tr.Version, CredentialStamp: tr.CredentialStamp, TwoFactorChallengeID: tr.ID, Session: *session}, binding, origin, options.Response)
}
func (s *PasskeyService) FinishSecondFactor(ctx context.Context, tr *TwoFactorTransaction, ceremony *PasskeyTransaction, raw []byte, ip string) (*TwoFactorProof, error) {
	if ceremony.Purpose != "second" || ceremony.TwoFactorChallengeID != tr.ID || ceremony.UserID != tr.UserID || ceremony.Version != tr.Version {
		return nil, ErrPasskeyInvalid
	}
	return s.factors.verifyWith(ctx, tr, "passkey", "", ip, func(tx *gorm.DB, u *model.User) error {
		if err := s.consume(ctx, ceremony); err != nil {
			return err
		}
		parsed, err := protocol.ParseCredentialRequestResponseBytes(raw)
		if err != nil {
			return ErrPasskeyInvalid
		}
		_, err = s.assertion(tx, ceremony, parsed, u.ID)
		return err
	})
}
func (s *TwoFactorService) BindPrimaryPasskey(ctx context.Context, tr *TwoFactorTransaction, proof *PasskeyProof) error {
	u, err := s.user(tr.UserID)
	if err != nil {
		return err
	}
	if err = proof.validate(s.db, u); err != nil {
		return err
	}
	if tr.Version != proof.version || tr.CredentialStamp != proof.stamp {
		return ErrPasskeyInvalid
	}
	tr.PrimaryPasskeyID = proof.credentialID
	raw, err := json.Marshal(tr)
	if err != nil {
		return err
	}
	return s.redis.Set(ctx, "2fa:challenge:"+tr.ID, raw, time.Until(tr.ExpiresAt)).Err()
}

func (s *TwoFactorService) RemovePasskey(ctx context.Context, userID uint, sid, token string, id uint, ip string) (*TwoFactorProof, error) {
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
		r := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Passkey{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errors.New("Passkey 不存在")
		}
		if err := s.withDB(tx).validateLoginPath(userID); err != nil {
			return err
		}
		var err error
		proof, err = s.changeConfiguration(tx, &u, userID, "passkey_removed", "passkey", "", ip, op.Method)
		return err
	})
	return proof, err
}
func RenamePasskey(db *gorm.DB, userID, id uint, name string) error {
	name, err := validPasskeyName(name)
	if err != nil {
		return err
	}
	var record model.Passkey
	if err = db.Where("id = ? AND user_id = ?", id, userID).First(&record).Error; err != nil {
		return errors.New("Passkey 不存在")
	}
	return db.Model(&record).Update("name", name).Error
}
