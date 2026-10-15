package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/alicebob/miniredis/v2"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func twoFactorFixture(t *testing.T) (*TwoFactorService, *model.User, *model.LoginSession) {
	t.Helper()
	t.Setenv(credentialcrypto.EnvironmentKeys, "")
	t.Setenv(credentialcrypto.EnvironmentKey, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.LoginSession{}, &model.TwoFactorMethod{}, &model.TwoFactorRecoveryCode{}, &model.TwoFactorEvent{}, &model.OIDCToken{}, &model.AuthorizationCode{}, &model.Role{}, &model.UserRole{}, &model.ExternalPrincipal{}, &model.ExternalIdentifier{}, &model.Passkey{}, &model.IdentityConnection{}))
	now := time.Now()
	email, phone := "factor@example.test", "+8613800138000"
	u := &model.User{Username: "factor-user", IsActivated: true, Status: model.UserStatusActive, Email: &email, Phone: &phone, EmailVerifiedAt: &now, PhoneVerifiedAt: &now}
	salt, hash, err := NewUserService(db).GeneratePasswordHash("current-password")
	require.NoError(t, err)
	u.Salt, u.PasswordHash = salt, hash
	require.NoError(t, db.Create(u).Error)
	session, err := NewLoginSessionService(db).Create(u.ID, "password", "192.0.2.1", "test", now.Add(time.Hour))
	require.NoError(t, err)
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { client.Close() })
	return NewTwoFactorService(db, client, nil), u, session
}
func enableTestFactor(t *testing.T, s *TwoFactorService, u *model.User, method string) []string {
	t.Helper()
	u.TwoFactorEnabled = true
	u.TwoFactorVersion++
	require.NoError(t, s.db.Model(u).Updates(map[string]any{"two_factor_enabled": true, "two_factor_version": u.TwoFactorVersion}).Error)
	require.NoError(t, s.db.Create(&model.TwoFactorMethod{UserID: u.ID, Method: method, Receiver: verifiedFactorReceiver(u, method), CreatedAt: time.Now()}).Error)
	codes, err := replaceRecoveryCodes(s.db, u.ID)
	require.NoError(t, err)
	return codes
}
func TestTwoFactorEnrollmentAndSessionAssurance(t *testing.T) {
	s, u, old := twoFactorFixture(t)
	ctx := context.Background()
	token, tr, err := s.Reauthenticate(ctx, u.ID, old.ID, "incorrect", "browser", "user")
	require.Error(t, err)
	require.Empty(t, token)
	require.Nil(t, tr)
	token, tr, err = s.Reauthenticate(ctx, u.ID, old.ID, "current-password", "browser", "user")
	require.NoError(t, err)
	require.Nil(t, tr)
	enrollment, err := s.Enroll(ctx, u.ID, old.ID, token, "totp", "browser")
	require.NoError(t, err)
	require.NotEmpty(t, enrollment.Secret)
	require.Contains(t, enrollment.QRCode, "data:image/png;base64,")
	var current model.User
	require.NoError(t, s.db.First(&current, u.ID).Error)
	require.False(t, current.TwoFactorEnabled, "pending enrollment must not enable 2FA")
	_, err = s.Enroll(ctx, u.ID, old.ID, token, "totp", "browser")
	require.Error(t, err, "management proof is single use")
	tr, err = s.Read(ctx, enrollment.TwoFactorChallenge.ID, "browser")
	require.NoError(t, err)
	_, err = s.Read(ctx, tr.ID, "other-browser")
	require.ErrorIs(t, err, ErrTwoFactorExpired)
	code, err := totp.GenerateCode(enrollment.Secret, s.now())
	require.NoError(t, err)
	proof, err := s.Verify(ctx, tr, "totp", code, "192.0.2.1")
	require.NoError(t, err)
	codes, renewed, err := s.ConfirmEnrollment(ctx, tr, proof, "192.0.2.1")
	require.NoError(t, err)
	require.Empty(t, codes, "binding alone does not enable two-step verification")
	require.NoError(t, s.db.First(&current, u.ID).Error)
	require.False(t, current.TwoFactorEnabled)
	boundSession, err := NewLoginSessionService(s.db).CreateWithProof(u.ID, "security_change", "", "", time.Now().Add(time.Hour), renewed)
	require.NoError(t, err)
	op, _, err := s.Reauthenticate(ctx, u.ID, boundSession.ID, "current-password", "browser", "user")
	require.NoError(t, err)
	codes, renewed, err = s.Mutate(ctx, u.ID, boundSession.ID, op, "enable", "", "192.0.2.1", false)
	require.NoError(t, err)
	require.Len(t, codes, 10)
	_, err = dao.NewLoginSessionDAO(s.db).Valid(old.ID, u.ID)
	require.Error(t, err)
	_, err = NewLoginSessionService(s.db).Create(u.ID, "password", "", "", time.Now().Add(time.Hour))
	require.ErrorIs(t, err, ErrTwoFactorRequired)
	fresh, err := NewLoginSessionService(s.db).CreateWithProof(u.ID, "security_change", "", "", time.Now().Add(time.Hour), renewed)
	require.NoError(t, err)
	require.Equal(t, "password", fresh.TwoFactorMethod)
	_, err = dao.NewLoginSessionDAO(s.db).Valid(fresh.ID, u.ID)
	require.NoError(t, err)
	stored, err := dao.NewTwoFactorDAO(s.db).Method(u.ID, "totp")
	require.NoError(t, err)
	raw, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NotContains(t, string(raw), enrollment.Secret)
	require.NotEqual(t, enrollment.Secret, stored.Ciphertext)
	login, err := s.BeginLogin(ctx, u.ID, "oidc", "password", "oidc-123", true, "browser")
	require.NoError(t, err)
	_, err = s.Verify(ctx, login, "totp", code, "192.0.2.1")
	require.ErrorIs(t, err, ErrTwoFactorCode, "enrollment code cannot be replayed for login")
	require.NoError(t, s.db.Model(u).Update("two_factor_version", gorm.Expr("two_factor_version + 1")).Error)
	_, err = dao.NewLoginSessionDAO(s.db).Valid(fresh.ID, u.ID)
	require.Error(t, err)
	require.Error(t, dao.NewLoginSessionDAO(s.db).Lock(fresh.ID, u.ID), "locked grant creation must check assurance too")
	_, err = s.Read(ctx, login.ID, "browser")
	require.ErrorIs(t, err, ErrTwoFactorExpired)
}
func TestTwoFactorDeliveryIsolationAndContactChanges(t *testing.T) {
	for _, method := range []string{"phone", "email"} {
		t.Run(method, func(t *testing.T) {
			s, u, _ := twoFactorFixture(t)
			ctx := context.Background()
			enableTestFactor(t, s, u, method)
			var delivered, receiver string
			s.send = func(_ context.Context, gotMethod, to, code string, _ time.Duration) error {
				require.Equal(t, method, gotMethod)
				delivered, receiver = code, to
				return nil
			}
			tr, err := s.BeginLogin(ctx, u.ID, "oidc", method, "session", false, "browser")
			require.NoError(t, err)
			view, err := s.View(ctx, tr)
			require.NoError(t, err)
			for _, item := range view.Methods {
				require.NotEqual(t, method, item.Method)
			}
			_, err = s.SendCode(ctx, tr, method)
			require.Error(t, err)
			tr, err = s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
			require.NoError(t, err)
			_, err = s.SendCode(ctx, tr, method)
			require.NoError(t, err)
			require.Equal(t, verifiedFactorReceiver(u, method), receiver)
			_, err = s.SendCode(ctx, tr, method)
			require.Error(t, err)
			require.NoError(t, s.redis.Set(ctx, "verification:code:"+method+":"+receiver+":login", "123456", time.Minute).Err())
			_, err = s.Verify(ctx, tr, method, "123456", "192.0.2.1")
			if delivered != "123456" {
				require.ErrorIs(t, err, ErrTwoFactorCode)
			}
			proof, err := s.Verify(ctx, tr, method, delivered, "192.0.2.1")
			require.NoError(t, err)
			require.NotNil(t, proof)
			_, err = s.Verify(ctx, tr, method, delivered, "192.0.2.1")
			require.Error(t, err)
			tr, err = s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
			require.NoError(t, err)
			require.NoError(t, s.db.Model(u).Update(method, "changed-contact").Error)
			status, err := s.Status(u.ID)
			require.NoError(t, err)
			require.True(t, status.Enabled)
			_, err = s.Verify(ctx, tr, method, delivered, "192.0.2.1")
			require.Error(t, err, "contact replacement must not enroll the new address automatically")
		})
	}
}
func TestTwoFactorRecoveryConcurrentConsumptionAndLimits(t *testing.T) {
	s, u, _ := twoFactorFixture(t)
	ctx := context.Background()
	codes := enableTestFactor(t, s, u, "email")
	a, err := s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
	require.NoError(t, err)
	b, err := s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
	require.NoError(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, tr := range []*TwoFactorTransaction{a, b} {
		wg.Add(1)
		go func(tr *TwoFactorTransaction) {
			defer wg.Done()
			_, err := s.Verify(ctx, tr, "recovery", codes[0], "192.0.2.1")
			results <- err
		}(tr)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	tr, err := s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		_, err = s.Verify(ctx, tr, "recovery", "invalid", "192.0.2.1")
		require.ErrorIs(t, err, ErrTwoFactorCode)
	}
	_, err = s.Verify(ctx, tr, "recovery", codes[1], "192.0.2.1")
	require.ErrorIs(t, err, ErrTwoFactorLimited)
	for i := 0; i < 4; i++ {
		tr, err = s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
		require.NoError(t, err)
		_, err = s.Verify(ctx, tr, "recovery", "invalid", "192.0.2.1")
		require.ErrorIs(t, err, ErrTwoFactorCode)
	}
	tr, err = s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
	require.NoError(t, err)
	_, err = s.Verify(ctx, tr, "recovery", codes[1], "192.0.2.2")
	require.ErrorIs(t, err, ErrTwoFactorLimited, "changing challenge or IP cannot bypass account budget")
}
func TestTwoFactorChallengeExpiryAndCredentialChange(t *testing.T) {
	s, u, _ := twoFactorFixture(t)
	ctx := context.Background()
	enableTestFactor(t, s, u, "email")
	tr, err := s.BeginLogin(ctx, u.ID, "external", "external", "", false, "browser")
	require.NoError(t, err)
	s.now = func() time.Time { return tr.ExpiresAt.Add(time.Second) }
	_, err = s.Read(ctx, tr.ID, "browser")
	require.ErrorIs(t, err, ErrTwoFactorExpired)
	s.now = time.Now
	require.NoError(t, s.db.Model(u).Update("password_hash", "replaced").Error)
	_, err = s.Read(ctx, tr.ID, "browser")
	require.ErrorIs(t, err, ErrTwoFactorExpired)
	tr, err = s.BeginLogin(ctx, u.ID, "external", "external", "", false, "browser")
	require.NoError(t, err)
	require.NoError(t, s.db.Model(u).Update("status", model.UserStatusDisabled).Error)
	_, err = s.Read(ctx, tr.ID, "browser")
	require.Error(t, err)
}
func TestTwoFactorDisableRegenerateAndAudit(t *testing.T) {
	s, u, _ := twoFactorFixture(t)
	ctx := context.Background()
	codes := enableTestFactor(t, s, u, "email")
	login, err := s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
	require.NoError(t, err)
	proof, err := s.Verify(ctx, login, "recovery", codes[0], "192.0.2.1")
	require.NoError(t, err)
	session, err := NewLoginSessionService(s.db).CreateWithProof(u.ID, "password", "", "", time.Now().Add(time.Hour), proof)
	require.NoError(t, err)
	_, manage, err := s.Reauthenticate(ctx, u.ID, session.ID, "", "browser", "user")
	require.NoError(t, err)
	require.NotNil(t, manage)
	proof, err = s.Verify(ctx, manage, "recovery", codes[1], "192.0.2.1")
	require.NoError(t, err)
	op, err := s.Operation(ctx, u.ID, session.ID, "user", proof)
	require.NoError(t, err)
	newCodes, renewed, err := s.Mutate(ctx, u.ID, session.ID, op, "regenerate", "", "192.0.2.1", false)
	require.NoError(t, err)
	require.Len(t, newCodes, 10)
	_, err = dao.NewLoginSessionDAO(s.db).Valid(session.ID, u.ID)
	require.Error(t, err)
	session, err = NewLoginSessionService(s.db).CreateWithProof(u.ID, "password", "", "", time.Now().Add(time.Hour), renewed)
	require.NoError(t, err)
	login, err = s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
	require.NoError(t, err)
	_, err = s.Verify(ctx, login, "recovery", codes[2], "192.0.2.1")
	require.ErrorIs(t, err, ErrTwoFactorCode)
	_, manage, err = s.Reauthenticate(ctx, u.ID, session.ID, "", "browser", "user")
	require.NoError(t, err)
	proof, err = s.Verify(ctx, manage, "recovery", newCodes[0], "192.0.2.1")
	require.NoError(t, err)
	op, err = s.Operation(ctx, u.ID, session.ID, "user", proof)
	require.NoError(t, err)
	_, _, err = s.Mutate(ctx, u.ID, session.ID, op, "remove", "email", "192.0.2.1", false)
	require.Error(t, err)
	_, manage, err = s.Reauthenticate(ctx, u.ID, session.ID, "", "browser", "user")
	require.NoError(t, err)
	proof, err = s.Verify(ctx, manage, "recovery", newCodes[1], "192.0.2.1")
	require.NoError(t, err)
	op, err = s.Operation(ctx, u.ID, session.ID, "user", proof)
	require.NoError(t, err)
	_, renewed, err = s.Mutate(ctx, u.ID, session.ID, op, "disable", "", "192.0.2.1", true)
	require.NoError(t, err)
	status, err := s.Status(u.ID)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.Zero(t, status.RecoveryCodesRemaining)
	_, err = s.Read(ctx, login.ID, "browser")
	require.ErrorIs(t, err, ErrTwoFactorExpired)
	_, err = NewLoginSessionService(s.db).CreateWithProof(u.ID, "password", "", "", time.Now().Add(time.Hour), renewed)
	require.NoError(t, err)
	var events []model.TwoFactorEvent
	require.NoError(t, s.db.Order("id").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, "regenerate", events[0].Action)
	require.Equal(t, "disable", events[1].Action)
}
func TestTwoFactorTOTPTimeWindow(t *testing.T) {
	s, u, _ := twoFactorFixture(t)
	// RFC 6238 SHA-1 test secret. Its six-digit truncation at t=59 is 287082.
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cipher, err := credentialcrypto.FromEnvironment()
	require.NoError(t, err)
	envelope, err := cipher.Encrypt([]byte(secret), credentialcrypto.AAD(u.ID, "two_factor_totp", cipher.ActiveKeyVersion()))
	require.NoError(t, err)
	factor := model.TwoFactorMethod{UserID: u.ID, Method: "totp", Ciphertext: envelope.Ciphertext, Nonce: envelope.Nonce, Algorithm: envelope.Algorithm, KeyVersion: envelope.KeyVersion}
	s.now = func() time.Time { return time.Unix(59, 0) }
	step, err := s.verifyTOTP(factor, "287082")
	require.NoError(t, err)
	require.Equal(t, int64(1), step)
	s.now = func() time.Time { return time.Unix(89, 0) }
	_, err = s.verifyTOTP(factor, "287082")
	require.NoError(t, err)
	s.now = func() time.Time { return time.Unix(90, 0) }
	_, err = s.verifyTOTP(factor, "287082")
	require.ErrorIs(t, err, ErrTwoFactorCode)
	s.now = func() time.Time { return time.Unix(59, 0) }
	factor.LastUsedStep = 1
	_, err = s.verifyTOTP(factor, "287082")
	require.ErrorIs(t, err, ErrTwoFactorCode)
}

func TestTwoFactorAdminResetRequiresFreshAdminProof(t *testing.T) {
	s, target, _ := twoFactorFixture(t)
	require.NoError(t, s.db.Create(&model.Passkey{UserID: target.ID, RPID: "localhost", UserHandle: target.UUID, CredentialIDHash: "preserved", CredentialID: "preserved", CredentialData: "{}", Name: "Keep this login method"}).Error)
	ctx := context.Background()
	codes := enableTestFactor(t, s, target, "email")
	pending, err := s.BeginLogin(ctx, target.ID, "account", "password", "", false, "target-browser")
	require.NoError(t, err)
	admin := model.User{Username: "factor-admin", IsActivated: true, Status: model.UserStatusActive}
	admin.Salt, admin.PasswordHash, err = NewUserService(s.db).GeneratePasswordHash("admin-password")
	require.NoError(t, err)
	require.NoError(t, s.db.Create(&admin).Error)
	session, err := NewLoginSessionService(s.db).Create(admin.ID, "password", "", "", time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.NoError(t, s.db.Model(session).Update("has_admin_access", true).Error)
	kind := model.RoleTypeSystem
	role := model.Role{Name: "super-admin", Code: "super_admin", Type: &kind}
	require.NoError(t, s.db.Create(&role).Error)
	require.NoError(t, s.db.Create(&model.UserRole{UserID: admin.ID, RoleID: role.ID, AssignedBy: admin.ID, IsActive: true, ApprovalStatus: 1}).Error)
	require.Error(t, s.Reset(ctx, admin.ID, target.ID, session.ID, "", "verified identity", "192.0.2.1"))
	userOp, _, err := s.Reauthenticate(ctx, admin.ID, session.ID, "admin-password", "admin-browser", "user")
	require.NoError(t, err)
	require.Error(t, s.Reset(ctx, admin.ID, target.ID, session.ID, userOp, "verified identity", "192.0.2.1"), "a user-management proof cannot authorize an administrative reset")
	op, _, err := s.Reauthenticate(ctx, admin.ID, session.ID, "admin-password", "admin-browser", "admin")
	require.NoError(t, err)
	require.Error(t, s.Reset(ctx, admin.ID, target.ID, session.ID, op, "", "192.0.2.1"))
	require.NoError(t, s.Reset(ctx, admin.ID, target.ID, session.ID, op, "identity verified via support", "192.0.2.1"))
	var passkeys int64
	require.NoError(t, s.db.Model(&model.Passkey{}).Where("user_id = ?", target.ID).Count(&passkeys).Error)
	require.Equal(t, int64(1), passkeys, "administrator reset preserves primary Passkeys")
	status, err := s.Status(target.ID)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.Zero(t, status.RecoveryCodesRemaining)
	_, err = s.Read(ctx, pending.ID, "target-browser")
	require.ErrorIs(t, err, ErrTwoFactorExpired)
	require.Error(t, s.Reset(ctx, admin.ID, target.ID, session.ID, op, "identity verified again", "192.0.2.1"))
	var events []model.TwoFactorEvent
	require.NoError(t, s.db.Find(&events).Error)
	require.Len(t, events, 1)
	require.Equal(t, admin.ID, events[0].ActorID)
	require.Equal(t, "admin_reset", events[0].Action)
	var remaining int64
	require.NoError(t, s.db.Model(&model.TwoFactorRecoveryCode{}).Where("code_hash = ?", twoFactorHash(normalizeRecoveryCode(codes[0]))).Count(&remaining).Error)
	require.Zero(t, remaining)
}

func TestTwoFactorPasswordlessEnrollmentAndUnavailableChannels(t *testing.T) {
	s, u, session := twoFactorFixture(t)
	ctx := context.Background()
	require.NoError(t, s.db.Model(u).Updates(map[string]any{"password_hash": "", "salt": ""}).Error)
	require.NoError(t, s.db.Model(session).Update("login_method", "external").Error)
	op, _, err := s.Reauthenticate(ctx, u.ID, session.ID, "", "browser", "user")
	require.NoError(t, err, "recent upstream login can reauthenticate a passwordless user")
	_, err = s.Enroll(ctx, u.ID, session.ID, op, "phone", "browser")
	require.Error(t, err, "unconfigured messaging cannot be enrolled")
	require.NoError(t, s.db.Model(session).Update("created_at", time.Now().Add(-6*time.Minute)).Error)
	_, _, err = s.Reauthenticate(ctx, u.ID, session.ID, "", "browser", "user")
	require.Error(t, err)
	enableTestFactor(t, s, u, "email")
	pending, err := s.BeginLogin(ctx, u.ID, "external", "external", "", false, "browser")
	require.NoError(t, err)
	view, err := s.View(ctx, pending)
	require.NoError(t, err)
	require.Len(t, view.Methods, 3)
	require.False(t, view.Methods[0].Available)
	require.False(t, view.Methods[1].Available)
	require.Equal(t, "recovery", view.Methods[2].Method)
}

func TestTwoFactorContactReplacementRequiresExistingFactor(t *testing.T) {
	s, u, _ := twoFactorFixture(t)
	ctx := context.Background()
	codes := enableTestFactor(t, s, u, "email")
	tr, err := s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
	require.NoError(t, err)
	proof, err := s.Verify(ctx, tr, "recovery", codes[0], "192.0.2.1")
	require.NoError(t, err)
	session, err := NewLoginSessionService(s.db).CreateWithProof(u.ID, "password", "", "", time.Now().Add(time.Hour), proof)
	require.NoError(t, err)
	_, err = s.ChangeContact(ctx, u.ID, session.ID, "", VerificationTypeEmail, "new@example.test", "192.0.2.1")
	require.Error(t, err)
	_, tr, err = s.Reauthenticate(ctx, u.ID, session.ID, "", "browser", "user")
	require.NoError(t, err)
	proof, err = s.Verify(ctx, tr, "recovery", codes[1], "192.0.2.1")
	require.NoError(t, err)
	op, err := s.Operation(ctx, u.ID, session.ID, "user", proof)
	require.NoError(t, err)
	renewed, err := s.ChangeContact(ctx, u.ID, session.ID, op, VerificationTypeEmail, "new@example.test", "192.0.2.1")
	require.NoError(t, err)
	factor, err := dao.NewTwoFactorDAO(s.db).Method(u.ID, "email")
	require.NoError(t, err)
	require.Equal(t, "new@example.test", factor.Receiver)
	_, err = dao.NewLoginSessionDAO(s.db).Valid(session.ID, u.ID)
	require.Error(t, err)
	_, err = NewLoginSessionService(s.db).CreateWithProof(u.ID, "password", "", "", time.Now().Add(time.Hour), renewed)
	require.NoError(t, err)
}

func TestTwoFactorDefersExternalBindingUntilVerification(t *testing.T) {
	s, u, _ := twoFactorFixture(t)
	ctx := context.Background()
	codes := enableTestFactor(t, s, u, "email")
	auth := NewExternalAuthService(s.db, s.redis)
	profile := &protocol.ExternalProfile{DisplayName: "External user", Identifiers: []protocol.ExternalProfileIdentifier{{Namespace: model.IdentifierNamespaceLoginSubject, Value: "external-subject", Primary: true}}}
	continuation, csrf, err := auth.storeContinuation(&model.IdentityConnection{ID: 42, Category: model.ConnectionCategorySocial}, profile, "")
	require.NoError(t, err)
	result, err := auth.BindWithExistingUser(ctx, continuation, csrf, u.Username, "current-password")
	require.NoError(t, err)
	require.Equal(t, continuation, result.PendingContinuationID)
	var count int64
	require.NoError(t, s.db.Model(&model.ExternalPrincipal{}).Count(&count).Error)
	require.Zero(t, count, "password alone must not attach an external identity")
	require.ErrorIs(t, auth.CompleteTwoFactorBinding(ctx, continuation, csrf, nil), ErrTwoFactorRequired)
	tr, err := s.BeginLogin(ctx, u.ID, "external", "password", "", false, "browser", continuation, csrf)
	require.NoError(t, err)
	restored, err := s.Read(ctx, tr.ID, "browser")
	require.NoError(t, err)
	require.Equal(t, continuation, restored.ContinuationID)
	proof, err := s.Verify(ctx, restored, "recovery", codes[0], "192.0.2.1")
	require.NoError(t, err)
	require.NoError(t, auth.CompleteTwoFactorBinding(ctx, continuation, csrf, proof))
	require.NoError(t, s.db.Model(&model.ExternalPrincipal{}).Where("user_id = ?", u.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.Error(t, auth.CompleteTwoFactorBinding(ctx, continuation, csrf, proof))
}

func TestTwoFactorDeliveryMethodsAreAutomaticallyReady(t *testing.T) {
	for _, method := range []string{"email", "phone"} {
		t.Run(method, func(t *testing.T) {
			s, u, session := twoFactorFixture(t)
			ctx := context.Background()
			var code string
			s.send = func(_ context.Context, gotMethod, to, value string, _ time.Duration) error {
				require.Equal(t, method, gotMethod)
				require.Equal(t, verifiedFactorReceiver(u, method), to)
				code = value
				return nil
			}
			op, _, err := s.Reauthenticate(ctx, u.ID, session.ID, "current-password", "browser", "user")
			require.NoError(t, err)
			_, err = s.Enroll(ctx, u.ID, session.ID, op, method, "browser")
			require.ErrorContains(t, err, "联系方式")
			status, err := s.Status(u.ID)
			require.NoError(t, err)
			require.False(t, status.Enabled)
			require.True(t, status.CanEnable)
			codes, renewed, err := s.Mutate(ctx, u.ID, session.ID, op, "enable", "", "192.0.2.1", false)
			require.NoError(t, err)
			require.Len(t, codes, 10)
			_, err = NewLoginSessionService(s.db).CreateWithProof(u.ID, "security_change", "", "", time.Now().Add(time.Hour), renewed)
			require.NoError(t, err)
			tr, err := s.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
			require.NoError(t, err)
			_, err = s.SendCode(ctx, tr, method)
			require.NoError(t, err)
			_, err = s.Verify(ctx, tr, method, code, "192.0.2.1")
			require.NoError(t, err)
			_, err = dao.NewTwoFactorDAO(s.db).Method(u.ID, method)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound, "no duplicate channel enrollment is needed")
		})
	}
}

func TestTwoFactorOIDCChallengeCannotCompleteAfterReplacement(t *testing.T) {
	s, _, _ := twoFactorFixture(t)
	ctx := context.Background()
	sessions := NewOIDCSessionService(s.redis)
	tr, err := sessions.CreateSession(ctx, "client", "https://rp.example/callback", "openid", "state", "nonce", "code", "", "", "")
	require.NoError(t, err)
	tr.PendingTwoFactorID = "first-challenge"
	require.NoError(t, sessions.UpdateSession(ctx, tr))
	tr.PendingTwoFactorID = "replacement-challenge"
	require.NoError(t, sessions.UpdateSession(ctx, tr))
	err = sessions.MarkAsAuthenticated(ctx, tr.SessionID, 1, "login-session", true, "first-challenge")
	require.ErrorIs(t, err, ErrTwoFactorExpired)
	current, err := sessions.GetSession(ctx, tr.SessionID)
	require.NoError(t, err)
	require.False(t, current.IsAuthenticated)
	require.Equal(t, "replacement-challenge", current.PendingTwoFactorID)
	require.False(t, current.HasLoginInteraction)
	require.NoError(t, sessions.MarkAsAuthenticated(ctx, tr.SessionID, 1, "login-session", true, "replacement-challenge"))
	current, err = sessions.GetSession(ctx, tr.SessionID)
	require.NoError(t, err)
	require.True(t, current.HasLoginInteraction)
	require.ErrorIs(t, sessions.MarkAsAuthenticated(ctx, tr.SessionID, 1, "another-session", true, "replacement-challenge"), ErrTwoFactorExpired)
}
