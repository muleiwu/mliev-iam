package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

// Seed an enrolled public credential and sign a real assertion to obtain the
// same server-only evidence consumed by PasskeyController.LoginVerify.
func consentTestPasskeyProof(t *testing.T, f *accountAuthFixture, oidcID string) *service.PasskeyProof {
	t.Helper()
	require.NoError(t, f.db.AutoMigrate(&model.Passkey{}))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	id := []byte("consent-test-passkey")
	publicKey, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: key.X.FillBytes(make([]byte, 32)), -3: key.Y.FillBytes(make([]byte, 32))})
	require.NoError(t, err)
	marshal := func(value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		require.NoError(t, err)
		return data
	}
	b64 := base64.RawURLEncoding.EncodeToString
	credential := webauthn.Credential{ID: id, PublicKey: publicKey, AttestationType: "none"}
	require.NoError(t, f.db.Create(&model.Passkey{
		UserID: f.user.ID, RPID: "localhost", UserHandle: f.user.UUID,
		CredentialID: b64(id), CredentialIDHash: dao.PasskeyCredentialHash(id),
		CredentialData: string(marshal(credential)), Name: "Consent test authenticator",
	}).Error)
	ctx := context.Background()
	const origin = "http://localhost:8080"
	svc, err := service.NewPasskeyService(f.db, f.redis, service.PasskeyConfig{
		Enabled: true, RPID: "localhost", Origins: []string{origin}, DisplayName: "Test IAM",
	}, service.NewTwoFactorService(f.db, f.redis, nil))
	require.NoError(t, err)
	options, err := svc.BeginLogin(ctx, "test-browser", origin, oidcID, false)
	require.NoError(t, err)
	transaction, err := svc.Read(ctx, options.TransactionID, "test-browser", origin, "login")
	require.NoError(t, err)
	client := marshal(map[string]any{"type": "webauthn.get", "challenge": transaction.Session.Challenge, "origin": origin, "crossOrigin": false})
	rpHash := sha256.Sum256([]byte("localhost"))
	authenticator := append(rpHash[:], 0x05, 0, 0, 0, 1) // user present + verified, counter 1
	clientHash := sha256.Sum256(client)
	hash := sha256.Sum256(append(append([]byte{}, authenticator...), clientHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
	require.NoError(t, err)
	assertion := marshal(map[string]any{
		"id": b64(id), "rawId": b64(id), "type": "public-key",
		"response":               map[string]any{"clientDataJSON": b64(client), "authenticatorData": b64(authenticator), "signature": b64(signature), "userHandle": b64([]byte(f.user.UUID))},
		"clientExtensionResults": map[string]any{},
	})
	proof, err := svc.FinishLogin(ctx, transaction, assertion)
	require.NoError(t, err)
	return proof
}
