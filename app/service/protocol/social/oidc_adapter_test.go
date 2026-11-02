package social

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type oidcTestResolver struct{ doc *OIDCDoc }

func (r oidcTestResolver) Resolve(context.Context, string, string) (*OIDCDoc, error) {
	return r.doc, nil
}

func TestOIDCKeyRotationAndAlgorithms(t *testing.T) {
	var mu sync.Mutex
	var public *ecdsa.PublicKey
	var kid string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		encode := func(n []byte) string { return base64.RawURLEncoding.EncodeToString(n) }
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "EC", "crv": "P-256", "use": "sig", "kid": kid, "alg": "ES256",
			"x": encode(public.X.FillBytes(make([]byte, 32))), "y": encode(public.Y.FillBytes(make([]byte, 32))),
		}}})
	}))
	defer server.Close()
	provider := &Provider{Type: ProviderTypeOIDC, ClientID: "client", ClientSecret: "secret", RedirectURL: "https://iam.test/callback", Attributes: map[string]any{"issuer": server.URL}}
	doc := &OIDCDoc{Issuer: server.URL, AuthorizationEndpoint: server.URL + "/auth", TokenEndpoint: server.URL + "/token", JwksURI: server.URL + "/keys", SigningAlgs: []string{"ES256", "HS256", "none"}}
	adapter, err := NewOIDCAdapter(provider, oidcTestResolver{doc})
	require.NoError(t, err)
	require.NoError(t, adapter.ConfigureTransaction("nonce", oauth2.GenerateVerifier()))
	claims := jwt.MapClaims{"iss": server.URL, "sub": "user", "aud": "client", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": "nonce"}
	for i := range 2 {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		mu.Lock()
		public, kid = &key.PublicKey, fmt.Sprint(i)
		mu.Unlock()
		token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
		token.Header["kid"] = fmt.Sprint(i)
		raw, err := token.SignedString(key)
		require.NoError(t, err)
		for range 2 {
			profile, err := adapter.GetUserInfo(context.Background(), (&oauth2.Token{}).WithExtra(map[string]any{"id_token": raw}))
			require.NoError(t, err)
			require.Equal(t, "user", profile.OpenID)
		}
		mu.Lock()
		require.Equal(t, i+1, calls, "JWKS should be cached and refreshed when the signing key changes")
		mu.Unlock()
	}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS256, jwt.SigningMethodNone} {
		var key any = []byte("secret")
		if method == jwt.SigningMethodNone {
			key = jwt.UnsafeAllowNoneSignatureType
		}
		raw, err := jwt.NewWithClaims(method, claims).SignedString(key)
		require.NoError(t, err)
		profile, err := adapter.GetUserInfo(context.Background(), (&oauth2.Token{}).WithExtra(map[string]any{"id_token": raw}))
		require.Error(t, err)
		require.Nil(t, profile)
	}
}

func TestOIDCTokenAuthenticationAndScopeDefaults(t *testing.T) {
	for _, test := range []struct {
		name, method, secret string
		methods              []string
		wantStyle            oauth2.AuthStyle
		wantError            bool
	}{
		{name: "auto without metadata", secret: "secret", wantStyle: oauth2.AuthStyleAutoDetect},
		{name: "auto selects post", secret: "secret", methods: []string{"client_secret_post"}, wantStyle: oauth2.AuthStyleInParams},
		{name: "auto prefers basic", secret: "secret", methods: []string{"client_secret_post", "client_secret_basic"}, wantStyle: oauth2.AuthStyleInHeader},
		{name: "none ignores stored secret", method: "none", secret: "old-secret", methods: []string{"none"}, wantStyle: oauth2.AuthStyleInParams},
		{name: "none without secret", method: "none", wantStyle: oauth2.AuthStyleInParams},
		{name: "missing secret", wantError: true},
		{name: "unsupported method", method: "private_key_jwt", wantError: true},
		{name: "advertised methods mismatch", method: "client_secret_post", secret: "secret", methods: []string{"client_secret_basic"}, wantError: true},
		{name: "auto cannot silently become public", secret: "secret", methods: []string{"none"}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &Provider{ClientID: "client", ClientSecret: test.secret, RedirectURL: "https://iam.test/callback", Scopes: "profile email,openid profile", Attributes: map[string]any{"issuer": "https://idp.test", "tokenEndpointAuthMethod": test.method}}
			doc := &OIDCDoc{Issuer: "https://idp.test", AuthorizationEndpoint: "https://idp.test/auth", TokenEndpoint: "https://idp.test/token", JwksURI: "https://idp.test/keys", TokenEndpointAuthMethods: test.methods}
			adapter, err := NewOIDCAdapter(p, oidcTestResolver{doc})
			if err == nil {
				err = adapter.ValidateConfig()
			}
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantStyle, adapter.Config.Endpoint.AuthStyle)
			require.Equal(t, []string{"openid", "profile", "email"}, adapter.Config.Scopes)
			if test.method == "none" {
				require.Empty(t, adapter.Config.ClientSecret)
			}
			require.Equal(t, test.secret, p.ClientSecret, "do not mutate saved credentials")
			require.Empty(t, adapter.GetAuthURL("state"), "a transaction must be configured first")
			_, err = adapter.ExchangeToken(context.Background(), "code")
			require.ErrorContains(t, err, "事务")
		})
	}
}
