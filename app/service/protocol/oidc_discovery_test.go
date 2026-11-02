package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOIDCDiscoveryValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		modify    func(*OIDCDiscoveryDoc)
		wantError string
	}{
		{name: "valid"},
		{name: "missing issuer", modify: func(d *OIDCDiscoveryDoc) { d.Issuer = "" }, wantError: "issuer"},
		{name: "wrong issuer", modify: func(d *OIDCDiscoveryDoc) { d.Issuer = "https://other.test" }, wantError: "issuer"},
		{name: "missing JWKS", modify: func(d *OIDCDiscoveryDoc) { d.JwksURI = "" }, wantError: "jwks_uri"},
		{name: "insecure token endpoint", modify: func(d *OIDCDiscoveryDoc) { d.TokenEndpoint = "http://idp.test/token" }, wantError: "HTTPS"},
		{name: "relative authorization endpoint", modify: func(d *OIDCDiscoveryDoc) { d.AuthorizationEndpoint = "/authorize" }, wantError: "authorization_endpoint"},
		{name: "invalid userinfo", modify: func(d *OIDCDiscoveryDoc) { d.UserinfoEndpoint = "file:///etc/passwd" }, wantError: "userinfo_endpoint"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var issuer string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/realm/.well-known/openid-configuration", r.URL.Path)
				doc := OIDCDiscoveryDoc{Issuer: issuer, AuthorizationEndpoint: issuer + "/authorize", TokenEndpoint: issuer + "/token", JwksURI: issuer + "/keys"}
				if test.modify != nil {
					test.modify(&doc)
				}
				_ = json.NewEncoder(w).Encode(doc)
			}))
			defer server.Close()
			issuer = server.URL + "/realm/"
			doc, err := NewOIDCDiscoveryCache(nil).Resolve(context.Background(), issuer)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Nil(t, doc)
				return
			}
			require.NoError(t, err)
			require.Equal(t, issuer, doc.Issuer, "issuer must retain its trailing slash")
		})
	}
}

func TestOIDCDiscoveryCacheAndFailures(t *testing.T) {
	var issuer string
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(OIDCDiscoveryDoc{Issuer: issuer, AuthorizationEndpoint: issuer + "/auth", TokenEndpoint: issuer + "/token", JwksURI: issuer + "/keys", SigningAlgs: []string{"ES256"}})
	}))
	defer server.Close()
	issuer = server.URL
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	defer rdb.Close()
	cache := NewOIDCDiscoveryCache(rdb)
	for range 2 {
		doc, err := cache.Resolve(context.Background(), issuer)
		require.NoError(t, err)
		require.Equal(t, []string{"ES256"}, doc.SigningAlgs)
	}
	require.EqualValues(t, 1, calls.Load())
	// An old or tampered cache entry must not bypass issuer/JWKS checks.
	require.NoError(t, rdb.Set(context.Background(), cache.cacheKey(issuer), `{"issuer":"https://other.test"}`, 0).Err())
	_, err := cache.Resolve(context.Background(), issuer)
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
	_, err = cache.FetchAndCache(context.Background(), issuer)
	require.NoError(t, err)
	require.EqualValues(t, 3, calls.Load())

	server.Close()
	_, err = NewOIDCDiscoveryCache(nil).Resolve(context.Background(), issuer)
	require.ErrorContains(t, err, "discovery")
	for _, invalid := range []string{"", "http://idp.example.test", "file:///tmp/issuer", "https://user:pass@idp.test", "https://idp.test/#fragment"} {
		_, err := cache.Resolve(context.Background(), invalid)
		require.Error(t, err)
	}
}

func TestOIDCDiscoveryDoesNotFollowRedirects(t *testing.T) {
	var requests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()
	_, err := NewOIDCDiscoveryCache(nil).Resolve(context.Background(), server.URL)
	require.ErrorContains(t, err, "302")
	require.Zero(t, requests.Load())
}
