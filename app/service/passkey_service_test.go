package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

type testPasskeyDevice struct {
	key   *ecdsa.PrivateKey
	id    []byte
	count uint32
}

func newTestPasskeyDevice(t *testing.T) *testPasskeyDevice {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	id := make([]byte, 32)
	_, err = rand.Read(id)
	require.NoError(t, err)
	return &testPasskeyDevice{key: key, id: id}
}
func b64(v []byte) string { return base64.RawURLEncoding.EncodeToString(v) }
func jsonBytes(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}
func (d *testPasskeyDevice) registration(t *testing.T, tr *PasskeyTransaction, origin, rp string, flags byte) []byte {
	t.Helper()
	client := jsonBytes(t, map[string]any{"type": "webauthn.create", "challenge": tr.Session.Challenge, "origin": origin, "crossOrigin": false})
	rpHash := sha256.Sum256([]byte(rp))
	auth := append([]byte{}, rpHash[:]...)
	auth = append(auth, flags, 0, 0, 0, 0)
	auth = append(auth, make([]byte, 16)...)
	auth = binary.BigEndian.AppendUint16(auth, uint16(len(d.id)))
	auth = append(auth, d.id...)
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: d.key.X.FillBytes(make([]byte, 32)), -3: d.key.Y.FillBytes(make([]byte, 32))})
	require.NoError(t, err)
	auth = append(auth, cose...)
	obj, err := cbor.Marshal(map[string]any{"fmt": "none", "authData": auth, "attStmt": map[string]any{}})
	require.NoError(t, err)
	return jsonBytes(t, map[string]any{"id": b64(d.id), "rawId": b64(d.id), "type": "public-key", "response": map[string]any{"clientDataJSON": b64(client), "attestationObject": b64(obj), "transports": []string{"internal"}}, "clientExtensionResults": map[string]any{"credProps": map[string]any{"rk": true}}})
}
func (d *testPasskeyDevice) assertion(t *testing.T, tr *PasskeyTransaction, origin, rp, handle string, flags byte) []byte {
	t.Helper()
	d.count++
	client := jsonBytes(t, map[string]any{"type": "webauthn.get", "challenge": tr.Session.Challenge, "origin": origin, "crossOrigin": false})
	rpHash := sha256.Sum256([]byte(rp))
	auth := append([]byte{}, rpHash[:]...)
	auth = append(auth, flags)
	auth = binary.BigEndian.AppendUint32(auth, d.count)
	clientHash := sha256.Sum256(client)
	signed := append(append([]byte{}, auth...), clientHash[:]...)
	hash := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, d.key, hash[:])
	require.NoError(t, err)
	return jsonBytes(t, map[string]any{"id": b64(d.id), "rawId": b64(d.id), "type": "public-key", "response": map[string]any{"clientDataJSON": b64(client), "authenticatorData": b64(auth), "signature": b64(signature), "userHandle": b64([]byte(handle))}, "clientExtensionResults": map[string]any{}})
}
func passkeyFixture(t *testing.T) (*PasskeyService, *TwoFactorService, *model.User, *model.LoginSession) {
	t.Helper()
	f, u, sid := twoFactorFixture(t)
	cfg := PasskeyConfig{Enabled: true, RPID: "localhost", Origins: []string{"http://localhost:8080"}, DisplayName: "Test IAM"}
	f.passkeyConfig = &cfg
	s, err := NewPasskeyService(f.db, f.redis, cfg, f)
	require.NoError(t, err)
	return s, f, u, sid
}
func addTestPasskey(t *testing.T, s *PasskeyService, f *TwoFactorService, u *model.User, sid *model.LoginSession) (*testPasskeyDevice, *model.LoginSession) {
	t.Helper()
	ctx := context.Background()
	op, _, err := f.Reauthenticate(ctx, u.ID, sid.ID, "current-password", "browser", "user")
	require.NoError(t, err)
	options, err := s.BeginRegistration(ctx, u.ID, sid.ID, op, "Test device", "browser", "http://localhost:8080")
	require.NoError(t, err)
	tr, err := s.Read(ctx, options.TransactionID, "browser", "http://localhost:8080", "registration")
	require.NoError(t, err)
	device := newTestPasskeyDevice(t)
	raw := device.registration(t, tr, tr.Origin, "localhost", 0x45)
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	require.NoError(t, err)
	wu, err := s.user(s.db, u.ID)
	require.NoError(t, err)
	_, err = s.wa.CreateCredential(wu, tr.Session, parsed)
	require.NoError(t, err)
	proof, err := s.FinishRegistration(ctx, tr, u.ID, sid.ID, raw, "192.0.2.1")
	require.NoError(t, err)
	fresh, err := NewLoginSessionService(s.db).CreateWithProof(u.ID, "security_change", "", "", time.Now().Add(time.Hour), proof)
	require.NoError(t, err)
	require.NoError(t, s.db.First(u, u.ID).Error)
	return device, fresh
}
func beginTestPasskeyLogin(t *testing.T, s *PasskeyService) *PasskeyTransaction {
	t.Helper()
	options, err := s.BeginLogin(context.Background(), "browser", "http://localhost:8080", "", false)
	require.NoError(t, err)
	tr, err := s.Read(context.Background(), options.TransactionID, "browser", "http://localhost:8080", "login")
	require.NoError(t, err)
	return tr
}
func TestPasskeyConfiguration(t *testing.T) {
	base := PasskeyConfig{Enabled: true, RPID: "iam.example.com", Origins: []string{"https://iam.example.com"}, DisplayName: "IAM"}
	_, err := base.WebAuthn()
	require.NoError(t, err)
	for _, origin := range []string{"http://iam.example.com", "https://evil.example.com", "https://iam.example.com/path", "https://iam.example.com?x=1", "https://user@iam.example.com", "*"} {
		c := base
		c.Origins = []string{origin}
		_, err = c.WebAuthn()
		require.Error(t, err, origin)
	}
	for _, rp := range []string{"", "com", "https://iam.example.com", "127.0.0.1", "IAM.example.com"} {
		c := base
		c.RPID = rp
		_, err = c.WebAuthn()
		require.Error(t, err, rp)
	}
	base.Enabled = false
	_, err = base.WebAuthn()
	require.ErrorIs(t, err, ErrPasskeyUnavailable)
}
func TestPasskeyRegistrationLoginAndReplay(t *testing.T) {
	s, f, u, old := passkeyFixture(t)
	device, fresh := addTestPasskey(t, s, f, u, old)
	require.False(t, u.TwoFactorEnabled)
	_, err := NewLoginSessionService(s.db).Create(u.ID, "passkey", "", "", time.Now().Add(time.Hour))
	require.ErrorIs(t, err, ErrPasskeyInvalid, "Passkey sessions require server-verified primary evidence")
	_, err = dao.NewLoginSessionDAO(s.db).Valid(old.ID, u.ID)
	require.Error(t, err)
	records, err := dao.NewPasskeyDAO(s.db).List(u.ID)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, u.UUID, records[0].UserHandle)
	public := string(jsonBytes(t, records))
	require.NotContains(t, public, "credentialData")
	require.NotContains(t, public, "credentialId")
	require.NoError(t, RenamePasskey(s.db, u.ID, records[0].ID, " Laptop "))
	require.Error(t, RenamePasskey(s.db, u.ID+1, records[0].ID, "stolen"))
	require.Error(t, RenamePasskey(s.db, u.ID, records[0].ID, strings.Repeat("a", 101)))
	tr := beginTestPasskeyLogin(t, s)
	for _, bad := range []struct{ binding, origin, purpose string }{{"other", tr.Origin, "login"}, {"browser", "https://evil.example", "login"}, {"browser", tr.Origin, "second"}} {
		_, err = s.Read(context.Background(), tr.ID, bad.binding, bad.origin, bad.purpose)
		require.Error(t, err)
	}
	response := device.assertion(t, tr, tr.Origin, "localhost", u.UUID, 0x05)
	proof, err := s.FinishLogin(context.Background(), tr, response)
	require.NoError(t, err)
	require.Equal(t, u.ID, proof.UserID())
	_, err = s.FinishLogin(context.Background(), tr, response)
	require.Error(t, err)
	login, err := NewLoginSessionService(s.db).CreateWithProof(u.ID, "passkey", "", "", time.Now().Add(time.Hour), nil, proof)
	require.NoError(t, err)
	require.Equal(t, "passkey", login.LoginMethod)
	records, err = dao.NewPasskeyDAO(s.db).List(u.ID)
	require.NoError(t, err)
	require.NotNil(t, records[0].LastUsedAt)
	var stored webauthn.Credential
	require.NoError(t, json.Unmarshal([]byte(records[0].CredentialData), &stored))
	require.Equal(t, uint32(1), stored.Authenticator.SignCount)
	op, _, err := f.Reauthenticate(context.Background(), u.ID, fresh.ID, "current-password", "browser", "user")
	require.NoError(t, err)
	_, err = f.RemovePasskey(context.Background(), u.ID, fresh.ID, op, records[0].ID, "192.0.2.1")
	require.NoError(t, err)
	_, err = NewLoginSessionService(s.db).CreateWithProof(u.ID, "passkey", "", "", time.Now().Add(time.Hour), nil, proof)
	require.Error(t, err, "deletion invalidates already verified primary evidence")
}
func TestPasskeyRejectsInvalidAssertions(t *testing.T) {
	for _, scenario := range []string{"origin", "rp", "uv", "signature", "handle", "disabled", "deleted", "expired", "counter", "redis"} {
		t.Run(scenario, func(t *testing.T) {
			s, f, u, session := passkeyFixture(t)
			device, _ := addTestPasskey(t, s, f, u, session)
			tr := beginTestPasskeyLogin(t, s)
			origin, rp, handle, flags := tr.Origin, "localhost", u.UUID, byte(0x05)
			switch scenario {
			case "origin":
				origin = "https://evil.example"
			case "rp":
				rp = "evil.example"
			case "uv":
				flags = 0x01
			case "handle":
				handle = "different-user"
			case "signature":
				device.key = newTestPasskeyDevice(t).key
			case "disabled":
				require.NoError(t, s.db.Model(u).Update("status", model.UserStatusDisabled).Error)
			case "deleted":
				require.NoError(t, s.db.Where("user_id = ?", u.ID).Delete(&model.Passkey{}).Error)
			case "expired":
				tr.ExpiresAt = time.Now().Add(-time.Second)
			case "counter":
				first := device.assertion(t, tr, origin, rp, handle, flags)
				_, err := s.FinishLogin(context.Background(), tr, first)
				require.NoError(t, err)
				tr = beginTestPasskeyLogin(t, s)
				device.count = 0
			case "redis":
				require.NoError(t, s.redis.Close())
			}
			_, err := s.FinishLogin(context.Background(), tr, device.assertion(t, tr, origin, rp, handle, flags))
			require.Error(t, err)
		})
	}
}
func TestPasskeySecondStepSymmetryAndIsolation(t *testing.T) {
	s, f, u, session := passkeyFixture(t)
	device, session := addTestPasskey(t, s, f, u, session)
	ctx := context.Background()
	op, _, err := f.Reauthenticate(ctx, u.ID, session.ID, "current-password", "browser", "user")
	require.NoError(t, err)
	codes, _, err := f.Mutate(ctx, u.ID, session.ID, op, "enable", "", "192.0.2.1", false)
	require.NoError(t, err)
	require.Len(t, codes, 10)
	primary := beginTestPasskeyLogin(t, s)
	primaryProof, err := s.FinishLogin(ctx, primary, device.assertion(t, primary, primary.Origin, "localhost", u.UUID, 0x05))
	require.NoError(t, err)
	tr, err := f.BeginLogin(ctx, u.ID, "account", "passkey", "", false, "browser")
	require.NoError(t, err)
	require.NoError(t, f.BindPrimaryPasskey(ctx, tr, primaryProof))
	view, err := f.View(ctx, tr)
	require.NoError(t, err)
	for _, m := range view.Methods {
		require.NotEqual(t, "passkey", m.Method)
	}
	_, err = s.BeginSecondFactor(ctx, tr, "browser", primary.Origin)
	require.Error(t, err)
	_, err = NewLoginSessionService(s.db).CreateWithProof(u.ID, "passkey", "", "", time.Now().Add(time.Hour), nil, primaryProof)
	require.ErrorIs(t, err, ErrTwoFactorRequired)
	proof, err := f.Verify(ctx, tr, "password", "current-password", "192.0.2.1")
	require.NoError(t, err)
	_, err = NewLoginSessionService(s.db).CreateWithProof(u.ID, "passkey", "", "", time.Now().Add(time.Hour), proof)
	require.NoError(t, err)
	tr, err = f.BeginLogin(ctx, u.ID, "account", "password", "", false, "browser")
	require.NoError(t, err)
	_, err = f.Verify(ctx, tr, "password", "current-password", "192.0.2.1")
	require.Error(t, err)
	options, err := s.BeginSecondFactor(ctx, tr, "browser", primary.Origin)
	require.NoError(t, err)
	ceremony, err := s.Read(ctx, options.TransactionID, "browser", primary.Origin, "second")
	require.NoError(t, err)
	raw := device.assertion(t, ceremony, primary.Origin, "localhost", u.UUID, 0x05)
	other := *tr
	other.ID = strings.Repeat("a", 43)
	_, err = s.FinishSecondFactor(ctx, &other, ceremony, raw, "192.0.2.1")
	require.Error(t, err)
	proof, err = s.FinishSecondFactor(ctx, tr, ceremony, raw, "192.0.2.1")
	require.NoError(t, err)
	require.Equal(t, "passkey", proof.method)
	_, err = f.Verify(ctx, tr, "recovery", codes[0], "192.0.2.1")
	require.Error(t, err)
}
func TestPasskeyConcurrentSubmissionAndLastLoginGuard(t *testing.T) {
	s, f, u, session := passkeyFixture(t)
	device, session := addTestPasskey(t, s, f, u, session)
	tr := beginTestPasskeyLogin(t, s)
	raw := device.assertion(t, tr, tr.Origin, "localhost", u.UUID, 0x05)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.FinishLogin(context.Background(), tr, raw); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	require.Equal(t, 1, success)
	op, _, err := f.Reauthenticate(context.Background(), u.ID, session.ID, "current-password", "browser", "user")
	require.NoError(t, err)
	require.NoError(t, s.db.Model(u).Update("password_hash", "").Error)
	// Build a fresh management proof as if obtained via the remaining Passkey.
	require.NoError(t, s.db.First(u, u.ID).Error)
	op, err = f.Operation(context.Background(), u.ID, session.ID, "user", &TwoFactorProof{userID: u.ID, version: u.TwoFactorVersion, method: "passkey", at: time.Now()})
	require.NoError(t, err)
	items, err := dao.NewPasskeyDAO(s.db).List(u.ID)
	require.NoError(t, err)
	_, err = f.RemovePasskey(context.Background(), u.ID, session.ID, op, items[0].ID, "192.0.2.1")
	require.ErrorContains(t, err, "唯一")
	var count int64
	require.NoError(t, s.db.Model(&model.Passkey{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
func TestTwoFactorAllReadyMethodsAndDistinctCombinations(t *testing.T) {
	s, f, u, session := passkeyFixture(t)
	_, _ = addTestPasskey(t, s, f, u, session)
	f.send = func(context.Context, string, string, string, time.Duration) error { return nil }
	require.NoError(t, s.db.Model(u).Update("two_factor_enabled", true).Error)
	for _, first := range []string{"password", "passkey", "phone", "email"} {
		tr, err := f.BeginLogin(context.Background(), u.ID, "account", first, "", false, "browser")
		require.NoError(t, err)
		view, err := f.View(context.Background(), tr)
		require.NoError(t, err)
		require.Len(t, view.Methods, 3)
		for _, m := range view.Methods {
			require.NotEqual(t, first, m.Method)
			require.True(t, m.Available)
		}
	}
}
func TestPasskeyRegistrationRejectsInvalidAndDuplicateCredentials(t *testing.T) {
	for _, scenario := range []string{"origin", "rp", "uv", "duplicate", "user", "session"} {
		t.Run(scenario, func(t *testing.T) {
			s, f, u, session := passkeyFixture(t)
			device, session := addTestPasskey(t, s, f, u, session)
			ctx := context.Background()
			op, _, err := f.Reauthenticate(ctx, u.ID, session.ID, "current-password", "browser", "user")
			require.NoError(t, err)
			options, err := s.BeginRegistration(ctx, u.ID, session.ID, op, "Second", "browser", "http://localhost:8080")
			require.NoError(t, err)
			tr, err := s.Read(ctx, options.TransactionID, "browser", "http://localhost:8080", "registration")
			require.NoError(t, err)
			if scenario != "duplicate" {
				device = newTestPasskeyDevice(t)
			}
			origin, rp, flags := tr.Origin, "localhost", byte(0x45)
			id, sid := u.ID, session.ID
			switch scenario {
			case "origin":
				origin = "https://evil.example"
			case "rp":
				rp = "evil.example"
			case "uv":
				flags = 0x41
			case "user":
				id++
			case "session":
				sid = "different-session"
			}
			_, err = s.FinishRegistration(ctx, tr, id, sid, device.registration(t, tr, origin, rp, flags), "192.0.2.1")
			require.Error(t, err)
		})
	}
}

func TestPasskeyAllPrimaryAndSecondaryCombinations(t *testing.T) {
	s, f, u, session := passkeyFixture(t)
	device, _ := addTestPasskey(t, s, f, u, session)
	ctx := context.Background()
	sent := ""
	f.send = func(_ context.Context, _, _, code string, _ time.Duration) error { sent = code; return nil }
	require.NoError(t, s.db.Model(u).Update("two_factor_enabled", true).Error)
	for _, first := range []string{"password", "passkey", "phone", "email"} {
		for _, second := range []string{"password", "passkey", "phone", "email"} {
			t.Run(first+"/"+second, func(t *testing.T) {
				tr, err := f.BeginLogin(ctx, u.ID, "account", first, "", false, "browser")
				require.NoError(t, err)
				if first == second {
					_, err = f.Verify(ctx, tr, second, "current-password", "192.0.2.1")
					require.Error(t, err)
					return
				}
				if first == "passkey" {
					p := beginTestPasskeyLogin(t, s)
					primary, err := s.FinishLogin(ctx, p, device.assertion(t, p, p.Origin, "localhost", u.UUID, 0x05))
					require.NoError(t, err)
					require.NoError(t, f.BindPrimaryPasskey(ctx, tr, primary))
				}
				var proof *TwoFactorProof
				switch second {
				case "passkey":
					options, err := s.BeginSecondFactor(ctx, tr, "browser", "http://localhost:8080")
					require.NoError(t, err)
					ceremony, err := s.Read(ctx, options.TransactionID, "browser", "http://localhost:8080", "second")
					require.NoError(t, err)
					proof, err = s.FinishSecondFactor(ctx, tr, ceremony, device.assertion(t, ceremony, ceremony.Origin, "localhost", u.UUID, 0x05), "192.0.2.1")
					require.NoError(t, err)
				case "password":
					proof, err = f.Verify(ctx, tr, second, "current-password", "192.0.2.1")
					require.NoError(t, err)
				default:
					require.NoError(t, f.redis.Del(ctx, fmt.Sprintf("2fa:send:%d:%s", u.ID, second)).Err())
					_, err = f.SendCode(ctx, tr, second)
					require.NoError(t, err)
					proof, err = f.Verify(ctx, tr, second, sent, "192.0.2.1")
					require.NoError(t, err)
				}
				login, err := NewLoginSessionService(s.db).CreateWithProof(u.ID, first, "", "", time.Now().Add(time.Hour), proof)
				require.NoError(t, err)
				require.Equal(t, second, login.TwoFactorMethod)
			})
		}
	}
}

func TestPasskeyZeroCounterAndBackupState(t *testing.T) {
	s, f, u, session := passkeyFixture(t)
	ctx := context.Background()
	op, _, err := f.Reauthenticate(ctx, u.ID, session.ID, "current-password", "browser", "user")
	require.NoError(t, err)
	options, err := s.BeginRegistration(ctx, u.ID, session.ID, op, "Synced", "browser", "http://localhost:8080")
	require.NoError(t, err)
	tr, err := s.Read(ctx, options.TransactionID, "browser", "http://localhost:8080", "registration")
	require.NoError(t, err)
	d := newTestPasskeyDevice(t)
	_, err = s.FinishRegistration(ctx, tr, u.ID, session.ID, d.registration(t, tr, tr.Origin, "localhost", 0x4d), "192.0.2.1")
	require.NoError(t, err)
	for _, flags := range []byte{0x0d, 0x1d, 0x0d} {
		login := beginTestPasskeyLogin(t, s)
		d.count = ^uint32(0) // unsigned wrap generates the legitimate zero counter.
		_, err = s.FinishLogin(ctx, login, d.assertion(t, login, login.Origin, "localhost", u.UUID, flags))
		require.NoError(t, err)
		rows, err := dao.NewPasskeyDAO(s.db).List(u.ID)
		require.NoError(t, err)
		var cred webauthn.Credential
		require.NoError(t, json.Unmarshal([]byte(rows[0].CredentialData), &cred))
		require.True(t, cred.Flags.BackupEligible)
		require.Equal(t, flags&0x10 != 0, cred.Flags.BackupState)
		require.Zero(t, cred.Authenticator.SignCount)
	}
}
