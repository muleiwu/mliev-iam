package social

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/require"
)

func TestOIDCConnectionSigningKeys(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	edKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	for _, test := range []struct {
		name, alg, use, keyAlg string
		key                    any
		keyOps                 []string
		wantError              bool
	}{
		{name: "RSA", alg: "RS256", key: &rsaKey.PublicKey, keyAlg: "RS256", use: "sig"},
		{name: "RSA PSS", alg: "PS256", key: &rsaKey.PublicKey, keyAlg: "PS256"},
		{name: "EC", alg: "ES256", key: &ecKey.PublicKey, keyAlg: "ES256"},
		{name: "EdDSA", alg: "EdDSA", key: edKey, keyAlg: "EdDSA"},
		{name: "optional alg and use", alg: "RS256", key: &rsaKey.PublicKey},
		{name: "verify operation", alg: "RS256", key: &rsaKey.PublicKey, keyOps: []string{"verify"}},
		{name: "private key", alg: "RS256", key: rsaKey, wantError: true},
		{name: "shared secret", alg: "RS256", key: []byte("secret"), wantError: true},
		{name: "encryption key", alg: "RS256", key: &rsaKey.PublicKey, use: "enc", wantError: true},
		{name: "sign only operation", alg: "RS256", key: &rsaKey.PublicKey, keyOps: []string{"sign"}, wantError: true},
		{name: "different algorithm", alg: "RS256", key: &rsaKey.PublicKey, keyAlg: "RS512", wantError: true},
		{name: "different key type", alg: "RS256", key: &ecKey.PublicKey, wantError: true},
		{name: "different curve", alg: "ES384", key: &ecKey.PublicKey, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(jose.JSONWebKey{Key: test.key, Algorithm: test.keyAlg, Use: test.use})
			require.NoError(t, err)
			var key map[string]any
			require.NoError(t, json.Unmarshal(raw, &key))
			if test.keyOps != nil {
				key["key_ops"] = test.keyOps
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Authorization"), "JWKS must be public; do not send client credentials")
				_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{key}})
			}))
			defer server.Close()
			adapter := newOIDCCheckAdapter(t, server.URL, test.alg)
			err = adapter.CheckConnection(context.Background())
			if test.wantError {
				require.ErrorContains(t, err, "无可用签名公钥")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestOIDCConnectionCheckRejectsRedirects(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer server.Close()
	err := newOIDCCheckAdapter(t, server.URL, "RS256").CheckConnection(context.Background())
	require.ErrorContains(t, err, "JWKS 请求失败（HTTP 302）")
	require.Zero(t, targetCalls.Load())
}

func newOIDCCheckAdapter(t *testing.T, jwksURI, algorithm string) *OIDCAdapter {
	t.Helper()
	provider := &Provider{ClientID: "client", ClientSecret: "secret", RedirectURL: "https://iam.test/callback", Attributes: map[string]any{"issuer": "https://idp.test"}}
	doc := &OIDCDoc{Issuer: "https://idp.test", AuthorizationEndpoint: "https://idp.test/auth", TokenEndpoint: "https://idp.test/token", JwksURI: jwksURI, SigningAlgs: []string{algorithm}}
	adapter, err := NewOIDCAdapter(provider, oidcTestResolver{doc})
	require.NoError(t, err)
	return adapter
}
