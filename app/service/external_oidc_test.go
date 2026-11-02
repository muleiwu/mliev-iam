package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type externalOIDCProvider struct {
	server    *httptest.Server
	mu        sync.Mutex
	codes     map[string]url.Values
	jwksCalls atomic.Int32
}

type externalOIDCOptions struct {
	claims           map[string]any
	userinfo         map[string]any
	missingToken     bool
	invalidSignature bool
	authMethod       string
	redirectURI      string
	discoveryPath    string
	jwksHandler      http.HandlerFunc
}

func newExternalOIDCProvider(t *testing.T, options externalOIDCOptions) *externalOIDCProvider {
	t.Helper()
	if options.redirectURI == "" {
		options.redirectURI = "https://iam.test/external-auth/company/callback"
	}
	if options.discoveryPath == "" {
		options.discoveryPath = "/.well-known/openid-configuration"
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	p := &externalOIDCProvider{codes: map[string]url.Values{}}
	mux := http.NewServeMux()
	p.server = httptest.NewServer(mux)
	t.Cleanup(p.server.Close)
	mux.HandleFunc(options.discoveryPath, func(w http.ResponseWriter, r *http.Request) {
		doc := protocol.OIDCDiscoveryDoc{Issuer: p.server.URL, AuthorizationEndpoint: p.server.URL + "/authorize", TokenEndpoint: p.server.URL + "/token", JwksURI: p.server.URL + "/jwks", SigningAlgs: []string{"RS256"}}
		if options.userinfo != nil {
			doc.UserinfoEndpoint = p.server.URL + "/userinfo"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		p.jwksCalls.Add(1)
		if options.jwksHandler != nil {
			options.jwksHandler(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "test-key",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		clientID, secret, basic := r.BasicAuth()
		validClient := basic && clientID == "client" && secret == "secret"
		if options.authMethod == "client_secret_post" {
			validClient = !basic && r.FormValue("client_id") == "client" && r.FormValue("client_secret") == "secret"
		}
		if options.authMethod == "none" {
			validClient = !basic && r.FormValue("client_id") == "client" && r.FormValue("client_secret") == ""
		}
		if !validClient {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		p.mu.Lock()
		authorization, exists := p.codes[r.FormValue("code")]
		delete(p.codes, r.FormValue("code"))
		p.mu.Unlock()
		if !exists || r.FormValue("redirect_uri") != options.redirectURI || r.FormValue("grant_type") != "authorization_code" || oauth2.S256ChallengeFromVerifier(r.FormValue("code_verifier")) != authorization.Get("code_challenge") {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		claims := jwt.MapClaims{"iss": p.server.URL, "sub": "external-user", "aud": "client", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": authorization.Get("nonce"), "name": "OIDC User", "email": "oidc@example.test"}
		for k, v := range options.claims {
			if v == nil {
				delete(claims, k)
			} else {
				claims[k] = v
			}
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = "test-key"
		signed, err := token.SignedString(key)
		if err != nil {
			http.Error(w, "signing failed", 500)
			return
		}
		if options.invalidSignature {
			parts := strings.Split(signed, ".")
			parts[2] = base64.RawURLEncoding.EncodeToString(make([]byte, 256))
			signed = strings.Join(parts, ".")
		}
		response := map[string]any{"access_token": "access-token", "token_type": "Bearer", "expires_in": 3600}
		if !options.missingToken {
			response["id_token"] = signed
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token" {
			http.Error(w, "unauthorized", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(options.userinfo)
	})
	return p
}

func (p *externalOIDCProvider) code(t *testing.T, authURL string) (string, url.Values) {
	t.Helper()
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	q := u.Query()
	require.Equal(t, "code", q.Get("response_type"))
	require.Contains(t, strings.Fields(q.Get("scope")), "openid")
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.NotEmpty(t, q.Get("nonce"))
	code := oauth2.GenerateVerifier()
	p.mu.Lock()
	p.codes[code] = q
	p.mu.Unlock()
	return code, q
}

func TestExternalOIDCTokenValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		options   externalOIDCOptions
		wantError string
	}{
		{name: "ID Token profile"},
		{name: "userinfo profile", options: externalOIDCOptions{userinfo: map[string]any{"sub": "external-user", "id": "wrong-id", "name": "UserInfo User"}}},
		{name: "client secret post", options: externalOIDCOptions{authMethod: "client_secret_post"}},
		{name: "public client", options: externalOIDCOptions{authMethod: "none"}},
		{name: "missing id token", options: externalOIDCOptions{missingToken: true}, wantError: "缺少 id_token"},
		{name: "invalid signature", options: externalOIDCOptions{invalidSignature: true}, wantError: "校验失败"},
		{name: "JWKS failure body is private", options: externalOIDCOptions{jwksHandler: func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "private-upstream-content", http.StatusForbidden)
		}}, wantError: "JWKS"},
		{name: "wrong issuer", options: externalOIDCOptions{claims: map[string]any{"iss": "https://other.test"}}, wantError: "different provider"},
		{name: "wrong audience", options: externalOIDCOptions{claims: map[string]any{"aud": "other"}}, wantError: "audience"},
		{name: "expired", options: externalOIDCOptions{claims: map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}}, wantError: "expired"},
		{name: "missing subject", options: externalOIDCOptions{claims: map[string]any{"sub": nil}}, wantError: "sub"},
		{name: "missing expiry", options: externalOIDCOptions{claims: map[string]any{"exp": nil}}, wantError: "expired"},
		{name: "numeric subject", options: externalOIDCOptions{claims: map[string]any{"sub": 123}}, wantError: "校验失败"},
		{name: "missing issued at", options: externalOIDCOptions{claims: map[string]any{"iat": nil}}, wantError: "iat"},
		{name: "future issued at", options: externalOIDCOptions{claims: map[string]any{"iat": time.Now().Add(time.Hour).Unix()}}, wantError: "iat"},
		{name: "missing nonce", options: externalOIDCOptions{claims: map[string]any{"nonce": nil}}, wantError: "nonce"},
		{name: "wrong nonce", options: externalOIDCOptions{claims: map[string]any{"nonce": "other"}}, wantError: "nonce"},
		{name: "wrong azp", options: externalOIDCOptions{claims: map[string]any{"azp": "other"}}, wantError: "azp"},
		{name: "multiple audiences missing azp", options: externalOIDCOptions{claims: map[string]any{"aud": []string{"client", "other"}}}, wantError: "azp"},
		{name: "multiple audiences with azp", options: externalOIDCOptions{claims: map[string]any{"aud": []string{"client", "other"}, "azp": "client"}}},
		{name: "wrong access token hash", options: externalOIDCOptions{claims: map[string]any{"at_hash": "invalid"}}, wantError: "access token"},
		{name: "mismatched userinfo subject", options: externalOIDCOptions{userinfo: map[string]any{"sub": "other"}}, wantError: "UserInfo sub"},
		{name: "missing userinfo subject", options: externalOIDCOptions{userinfo: map[string]any{"id": "external-user"}}, wantError: "UserInfo sub"},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := newExternalOIDCProvider(t, test.options)
			connection := &model.IdentityConnection{Category: "enterprise", ProviderType: "oidc", Capabilities: model.StringList{"authentication"}, Config: model.JSONObject{"issuer": provider.server.URL, "redirectUrl": "https://iam.test/external-auth/company/callback", "scopes": "profile email", "tokenEndpointAuthMethod": test.options.authMethod}}
			adapter, err := protocol.NewAuthenticationAdapter(connection, map[string]any{"clientId": "client", "clientSecret": "secret"}, nil)
			require.NoError(t, err)
			require.NoError(t, adapter.ValidateConfig())
			require.NoError(t, adapter.(protocol.TransactionAuthenticationAdapter).ConfigureTransaction("expected-nonce", oauth2.GenerateVerifier()))
			code, _ := provider.code(t, adapter.AuthorizationURL("state"))
			token, err := adapter.ExchangeCode(context.Background(), code)
			require.NoError(t, err)
			profile, err := adapter.FetchProfile(context.Background(), token)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.NotContains(t, err.Error(), "private-upstream-content")
				require.Nil(t, profile)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "external-user", profile.Identifiers[0].Value)
			require.Equal(t, "oidc@example.test", profile.Email)
			if test.options.userinfo != nil {
				require.Equal(t, "UserInfo User", profile.DisplayName)
			} else {
				require.Equal(t, "OIDC User", profile.DisplayName)
			}
		})
	}
}

func TestExternalOIDCConnectionLoginAndBinding(t *testing.T) {
	for _, path := range []string{"", "/api/auth/.well-known/openid-configuration"} {
		t.Run("discovery="+path, func(t *testing.T) {
			testExternalOIDCConnectionLoginAndBinding(t, path)
		})
	}
}

func testExternalOIDCConnectionLoginAndBinding(t *testing.T, discoveryPath string) {
	t.Setenv(credentialcrypto.EnvironmentKeys, "")
	t.Setenv(credentialcrypto.EnvironmentKey, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	provider := newExternalOIDCProvider(t, externalOIDCOptions{discoveryPath: discoveryPath})
	discoveryURL := ""
	if discoveryPath != "" {
		discoveryURL = provider.server.URL + discoveryPath
	}
	db := loginVisibilityDB(t)
	require.NoError(t, db.AutoMigrate(&model.ExternalPrincipal{}, &model.ExternalIdentifier{}, &model.ExternalPrincipalConflict{}, &model.SyncJob{}, &model.User{}))
	require.NoError(t, db.Create(&model.User{ID: 42, Username: "local-user", Status: model.UserStatusActive, IsActivated: true}).Error)
	connections := NewIdentityConnectionService(db)
	connection, err := connections.Create(&dto.CreateIdentityConnectionRequest{
		Name: "Company", Identifier: "company", Category: "enterprise", ProviderType: "oidc", Capabilities: model.StringList{"authentication"}, Status: "enabled", ShowGlobally: true,
		Config:      map[string]any{"issuer": provider.server.URL, "discoveryUrl": discoveryURL, "redirectUrl": "https://iam.test/external-auth/company/callback"},
		Credentials: dto.IdentityConnectionCredentialInput{"authentication": {"clientId": "client", "clientSecret": "secret"}},
	})
	require.NoError(t, err, "issuer-only configuration must enable without Redis or manual endpoints")
	require.Equal(t, "healthy", connection.LastCheckStatus)
	require.NoError(t, connections.Test(connection.ID))
	public, err := connections.PublicLoginList(nil)
	require.NoError(t, err)
	require.Len(t, public, 1)
	require.Equal(t, "oidc", public[0].ProviderType)
	_, err = connections.Update(connection.ID, &dto.UpdateIdentityConnectionRequest{Credentials: dto.IdentityConnectionCredentialInput{"authentication": {"clientId": "", "clientSecret": ""}}})
	require.NoError(t, err)
	credentials, err := connections.CredentialValues(connection.ID, model.CredentialPurposeAuthentication)
	require.NoError(t, err)
	require.Equal(t, "secret", credentials["clientSecret"])

	redisServer := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	auth := NewExternalAuthService(db, rdb)
	checksBeforeLogin := provider.jwksCalls.Load()
	start, err := auth.GetAuthURL("company", "login-session", "https://iam.test", "browser-hash")
	require.NoError(t, err)
	require.Equal(t, checksBeforeLogin, provider.jwksCalls.Load(), "login must not run the management JWKS preflight")
	code, q := provider.code(t, start.AuthURL)
	state, valid := auth.ConsumeState(context.Background(), start.State)
	require.True(t, valid)
	require.Equal(t, q.Get("nonce"), state.OIDCNonce)
	require.Equal(t, q.Get("code_challenge"), oauth2.S256ChallengeFromVerifier(state.PKCEVerifier))
	require.Equal(t, "browser-hash", state.BrowserBindingHash)
	require.NotContains(t, start.AuthURL, state.PKCEVerifier)
	_, valid = auth.ConsumeState(context.Background(), start.State)
	require.False(t, valid, "state must be single use")
	response, err := auth.HandleCallback("company", code, state.SessionID, state)
	require.NoError(t, err)
	require.Equal(t, checksBeforeLogin+1, provider.jwksCalls.Load(), "callback should fetch JWKS only for ID Token verification")
	require.True(t, response.NeedsBind)
	profile, err := auth.GetTempProfile(response.ContinuationID)
	require.NoError(t, err)
	require.Equal(t, "external-user", profile.Profile.Identifiers[0].Value)

	bind, err := auth.GetBindAuthURL("company", 42, "https://iam.test")
	require.NoError(t, err)
	bindCode, _ := provider.code(t, bind.AuthURL)
	bindState, valid := auth.ConsumeState(context.Background(), bind.State)
	require.True(t, valid)
	require.True(t, bindState.IsBind())
	require.NotEqual(t, state.OIDCNonce, bindState.OIDCNonce)
	require.NoError(t, auth.BindWithCode(42, "company", bindCode, bindState))
	var principal model.ExternalPrincipal
	require.NoError(t, db.Where("connection_id = ? AND user_id = ?", connection.ID, 42).First(&principal).Error)
	require.Equal(t, "OIDC User", principal.DisplayName)
	login, err := auth.GetAuthURL("company", "login-session", "https://iam.test", "browser-hash")
	require.NoError(t, err)
	loginCode, _ := provider.code(t, login.AuthURL)
	loginState, valid := auth.ConsumeState(context.Background(), login.State)
	require.True(t, valid)
	loggedIn, err := auth.HandleCallback("company", loginCode, loginState.SessionID, loginState)
	require.NoError(t, err)
	require.False(t, loggedIn.NeedsBind)
	require.EqualValues(t, 42, loggedIn.User.ID)

	// A second issuer with the same sub/email must not inherit this binding.
	secondProvider := newExternalOIDCProvider(t, externalOIDCOptions{redirectURI: "https://iam.test/external-auth/second/callback"})
	secondConnection, err := connections.Create(&dto.CreateIdentityConnectionRequest{
		Name: "Second", Identifier: "second", Category: "social", ProviderType: "oidc", Capabilities: model.StringList{"authentication"}, Status: "enabled",
		Config:      map[string]any{"issuer": secondProvider.server.URL, "redirectUrl": "https://iam.test/external-auth/second/callback"},
		Credentials: dto.IdentityConnectionCredentialInput{"authentication": {"clientId": "client", "clientSecret": "secret"}},
	})
	require.NoError(t, err)
	second, err := auth.GetAuthURL("second", "login-session", "https://iam.test", "browser-hash")
	require.NoError(t, err)
	secondCode, _ := secondProvider.code(t, second.AuthURL)
	secondState, valid := auth.ConsumeState(context.Background(), second.State)
	require.True(t, valid)
	secondResponse, err := auth.HandleCallback("second", secondCode, secondState.SessionID, secondState)
	require.NoError(t, err)
	require.True(t, secondResponse.NeedsBind)
	var userCount int64
	require.NoError(t, db.Model(&model.User{}).Count(&userCount).Error)
	require.EqualValues(t, 1, userCount, "OIDC must not auto-create users")
	public, err = connections.PublicLoginList(nil)
	require.NoError(t, err)
	require.Len(t, public, 1, "the second connection is not globally visible")
	public, err = connections.PublicLoginList(&model.Application{LoginConnections: &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{secondConnection.ID}}})
	require.NoError(t, err)
	require.Len(t, public, 1)
	require.Equal(t, "second", public[0].Identifier)
	secondProfile, err := auth.GetTempProfile(secondResponse.ContinuationID)
	require.NoError(t, err)
	_, err = connections.Update(secondConnection.ID, &dto.UpdateIdentityConnectionRequest{Config: map[string]any{"issuer": provider.server.URL, "discoveryUrl": discoveryURL}})
	require.NoError(t, err, "an unbound connection may change issuer")
	require.ErrorContains(t, auth.attachProfile(db, secondConnection.ID, 42, secondProfile.Profile), "配置已变更")

	expiring, err := auth.GetAuthURL("company", "login-session", "https://iam.test", "browser-hash")
	require.NoError(t, err)
	redisServer.FastForward(5 * time.Minute)
	_, valid = auth.ConsumeState(context.Background(), expiring.State)
	require.False(t, valid)

	// Disabled local accounts remain blocked even after successful upstream authentication.
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 42).Update("status", model.UserStatusDisabled).Error)
	disabledLogin, err := auth.GetAuthURL("company", "login-session", "https://iam.test", "browser-hash")
	require.NoError(t, err)
	disabledCode, _ := provider.code(t, disabledLogin.AuthURL)
	disabledState, valid := auth.ConsumeState(context.Background(), disabledLogin.State)
	require.True(t, valid)
	_, err = auth.HandleCallback("company", disabledCode, disabledState.SessionID, disabledState)
	require.Error(t, err)
	_, err = connections.Update(connection.ID, &dto.UpdateIdentityConnectionRequest{Config: map[string]any{"issuer": "https://another.test"}})
	require.ErrorContains(t, err, "不能修改 OIDC Issuer")
	generic := "feishu"
	_, err = connections.Update(connection.ID, &dto.UpdateIdentityConnectionRequest{ProviderType: &generic})
	require.ErrorContains(t, err, "不能修改 OIDC Issuer")

	_, err = auth.HandleCallback("company", "code", "", nil)
	require.ErrorContains(t, err, "事务")
	err = auth.BindWithCode(42, "company", "code", &StateData{ConnectionIdentifier: "other"})
	require.ErrorContains(t, err, "不匹配")
}

func TestOIDCConnectionCapabilities(t *testing.T) {
	for _, category := range []model.ConnectionCategory{"enterprise", "social"} {
		require.NoError(t, validateIdentityConnectionFields("oidc", category, "oidc", model.StringList{"authentication"}, "enabled"))
		for _, capability := range []string{"directory_sync", "events"} {
			require.Error(t, validateIdentityConnectionFields("oidc", category, "oidc", model.StringList{capability}, "disabled"), fmt.Sprintf("%s %s", category, capability))
		}
	}
}

func TestExternalOIDCTransactionValidation(t *testing.T) {
	provider := newExternalOIDCProvider(t, externalOIDCOptions{})
	connection := &model.IdentityConnection{Category: "social", ProviderType: "oidc", Capabilities: model.StringList{"authentication"}, Config: model.JSONObject{"issuer": provider.server.URL, "redirectUrl": "https://iam.test/external-auth/company/callback", "tokenEndpointAuthMethod": "client_secret_basic"}}
	adapter, err := protocol.NewAuthenticationAdapter(connection, map[string]any{"clientId": "client", "clientSecret": "secret"}, nil)
	require.NoError(t, err)
	validState := StateData{ConnectionIdentifier: "company", SessionID: "session", Timestamp: time.Now().Unix(), OIDCNonce: "nonce", PKCEVerifier: oauth2.GenerateVerifier()}
	for _, test := range []struct {
		name   string
		modify func(*StateData)
	}{
		{"connection", func(s *StateData) { s.ConnectionIdentifier = "other" }},
		{"session", func(s *StateData) { s.SessionID = "other" }},
		{"bind mode", func(s *StateData) { s.Mode = StateModeBind }},
		{"bind user", func(s *StateData) { s.BindUserID = 42 }},
		{"expired", func(s *StateData) { s.Timestamp -= 300 }},
		{"missing nonce", func(s *StateData) { s.OIDCNonce = "" }},
		{"missing PKCE", func(s *StateData) { s.PKCEVerifier = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := validState
			test.modify(&state)
			require.Error(t, configureExternalTransaction(adapter, &state, "company", "session", 0))
		})
	}
	require.NoError(t, configureExternalTransaction(adapter, &validState, "company", "session", 0))
	code, _ := provider.code(t, adapter.AuthorizationURL("state"))
	// A wrong verifier must fail at the provider's token endpoint.
	require.NoError(t, adapter.(protocol.TransactionAuthenticationAdapter).ConfigureTransaction("nonce", oauth2.GenerateVerifier()))
	_, err = adapter.ExchangeCode(context.Background(), code)
	require.ErrorContains(t, err, "invalid_grant")
}

func TestExternalOIDCJWKSHealthAndEnablement(t *testing.T) {
	t.Setenv(credentialcrypto.EnvironmentKeys, "")
	t.Setenv(credentialcrypto.EnvironmentKey, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	for _, test := range []struct {
		name, body, wantError string
		status                int
	}{
		{"forbidden", "private-upstream-content", "HTTP 403", 403},
		{"HTML", "<html>private-upstream-content</html>", "HTML/验证页面", 200},
		{"invalid JSON", "private-upstream-content", "无效 JSON", 200},
		{"empty keys", `{"keys":[]}`, "无可用签名公钥", 200},
		{"symmetric keys", `{"keys":[{"kty":"oct","k":"c2VjcmV0","alg":"HS256"}]}`, "无可用签名公钥", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := newExternalOIDCProvider(t, externalOIDCOptions{jwksHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}})
			db := loginVisibilityDB(t)
			require.NoError(t, db.AutoMigrate(&model.ExternalPrincipal{}, &model.SyncJob{}))
			connections := NewIdentityConnectionService(db)
			request := &dto.CreateIdentityConnectionRequest{
				Name: "Blocked", Identifier: "blocked", Category: "enterprise", ProviderType: "oidc", Capabilities: model.StringList{"authentication"}, Status: "disabled",
				Config:      map[string]any{"issuer": provider.server.URL, "redirectUrl": "https://iam.test/external-auth/blocked/callback"},
				Credentials: dto.IdentityConnectionCredentialInput{"authentication": {"clientId": "client", "clientSecret": "secret"}},
			}
			connection, err := connections.Create(request)
			require.NoError(t, err, "disabled drafts do not need a reachable JWKS")
			require.Zero(t, provider.jwksCalls.Load())
			checks, err := connections.TestCapabilities(context.Background(), connection.ID)
			require.ErrorContains(t, err, test.wantError)
			require.Contains(t, err.Error(), "JWKS")
			require.NotContains(t, err.Error(), "private-upstream-content")
			require.Len(t, checks, 1)
			require.Equal(t, model.CapabilityHealthFailed, checks[0].Status)
			enabled := model.ConnectionStatusEnabled
			_, err = connections.Update(connection.ID, &dto.UpdateIdentityConnectionRequest{Status: &enabled})
			require.ErrorContains(t, err, test.wantError)
			saved, err := connections.Get(connection.ID)
			require.NoError(t, err)
			require.Equal(t, model.ConnectionStatusDisabled, saved.Status)
			require.Equal(t, "failed", saved.LastCheckStatus)
			request.Identifier, request.Status = "blocked-new", "enabled"
			_, err = connections.Create(request)
			require.ErrorContains(t, err, test.wantError)

			// Updating a live connection must run the same probe and roll back on failure.
			working := newExternalOIDCProvider(t, externalOIDCOptions{})
			request.Identifier, request.Config["issuer"] = "working", working.server.URL
			active, err := connections.Create(request)
			require.NoError(t, err)
			_, err = connections.Update(active.ID, &dto.UpdateIdentityConnectionRequest{Config: map[string]any{"issuer": provider.server.URL}})
			require.ErrorContains(t, err, test.wantError)
			saved, err = connections.Get(active.ID)
			require.NoError(t, err)
			require.Equal(t, working.server.URL, saved.Config["issuer"])
		})
	}
}
