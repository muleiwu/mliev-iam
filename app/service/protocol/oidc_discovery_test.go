package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
			doc, err := NewOIDCDiscoveryCache(nil).Resolve(context.Background(), issuer, "")
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
		doc, err := cache.Resolve(context.Background(), issuer, "")
		require.NoError(t, err)
		require.Equal(t, []string{"ES256"}, doc.SigningAlgs)
	}
	require.EqualValues(t, 1, calls.Load())
	// An old or tampered cache entry must not bypass issuer/JWKS checks.
	require.NoError(t, rdb.Set(context.Background(), cache.cacheKey(issuer, issuer+"/.well-known/openid-configuration"), `{"issuer":"https://other.test"}`, 0).Err())
	_, err := cache.Resolve(context.Background(), issuer, "")
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
	_, err = cache.FetchAndCache(context.Background(), issuer, "")
	require.NoError(t, err)
	require.EqualValues(t, 3, calls.Load())

	server.Close()
	_, err = NewOIDCDiscoveryCache(nil).Resolve(context.Background(), issuer, "")
	require.ErrorContains(t, err, "discovery")
	for _, invalid := range []string{"", "http://idp.example.test", "file:///tmp/issuer", "https://user:pass@idp.test", "https://idp.test/#fragment"} {
		_, err := cache.Resolve(context.Background(), invalid, "")
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
	_, err := NewOIDCDiscoveryCache(nil).Resolve(context.Background(), server.URL, "")
	require.ErrorContains(t, err, "302")
	require.Zero(t, requests.Load())
}

func TestOIDCCustomDiscoveryCacheIsolation(t *testing.T) {
	var issuer string
	var calls atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() || r.URL.Path == "/broken" {
			http.Error(w, "upstream-secret", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(OIDCDiscoveryDoc{Issuer: issuer, AuthorizationEndpoint: issuer + r.URL.Path + "/authorize", TokenEndpoint: issuer + "/token", JwksURI: issuer + "/jwks"})
	}))
	defer server.Close()
	issuer = server.URL
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	defer rdb.Close()
	cache := NewOIDCDiscoveryCache(rdb)
	standard := issuer + "/.well-known/openid-configuration"
	custom := issuer + "/api/auth/.well-known/openid-configuration"
	other := issuer + "/metadata"
	for _, endpoint := range []string{standard, custom, other} {
		for range 2 {
			doc, err := cache.Resolve(context.Background(), issuer, endpoint)
			require.NoError(t, err)
			require.Equal(t, issuer, doc.Issuer)
			require.Equal(t, endpoint+"/authorize", doc.AuthorizationEndpoint)
		}
	}
	require.EqualValues(t, 3, calls.Load())
	// Clearing a custom address restores the standard source, including its cache.
	doc, err := cache.Resolve(context.Background(), issuer, "")
	require.NoError(t, err)
	require.Equal(t, standard+"/authorize", doc.AuthorizationEndpoint)
	require.EqualValues(t, 3, calls.Load())
	require.Equal(t, 24*time.Hour, rdb.TTL(context.Background(), cache.cacheKey(issuer, custom)).Val())
	_, err = cache.Resolve(context.Background(), issuer, issuer+"/broken")
	require.ErrorContains(t, err, "HTTP 403")
	require.NotContains(t, err.Error(), "upstream-secret")
	// Force-refresh failures must invalidate that source's previously cached document.
	fail.Store(true)
	_, err = cache.FetchAndCache(context.Background(), issuer, custom)
	require.Error(t, err)
	_, err = cache.Resolve(context.Background(), issuer, custom)
	require.Error(t, err)
	require.EqualValues(t, 6, calls.Load())
	_, err = cache.Resolve(context.Background(), issuer, "")
	require.NoError(t, err, "a custom source refresh must not evict the standard source")
}

func TestOIDCCustomDiscoveryValidationAndDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name, body, wantError string
		status                int
	}{
		{"HTML", "<html><script>upstream-secret</script></html>", "HTML/验证页面", 200},
		{"invalid JSON", "upstream-secret", "无效 JSON", 200},
		{"forbidden", "upstream-secret", "HTTP 403", 403},
		{"redirect", "upstream-secret", "重定向", 302},
		{"issuer mismatch", `{"issuer":"https://other.test"}`, "issuer 与配置不匹配", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			var paths []string
			var mu sync.Mutex
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			_, err := NewOIDCDiscoveryCache(nil).Resolve(context.Background(), server.URL, server.URL+"/custom")
			require.ErrorContains(t, err, test.wantError)
			require.Contains(t, err.Error(), "discovery")
			require.NotContains(t, err.Error(), "upstream-secret")
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, []string{"/custom"}, paths, "must not retry the standard discovery path")
		})
	}
	for _, invalid := range []string{"http://idp.test/config", "file:///tmp/config", "/metadata", "https://user:pass@idp.test/config", "https://idp.test/config#fragment"} {
		_, err := NewOIDCDiscoveryCache(nil).Resolve(context.Background(), "https://idp.test", invalid)
		require.Error(t, err)
	}
}
