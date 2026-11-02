package social

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"

	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/oidcclient"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const oidcDefaultScp = "openid,profile,email"

// OIDCResolver avoids a dependency on the parent protocol package.
type OIDCResolver interface {
	Resolve(ctx context.Context, issuer, discoveryURL string) (*OIDCDoc, error)
}

type OIDCDoc struct {
	Issuer                   string
	AuthorizationEndpoint    string
	TokenEndpoint            string
	UserinfoEndpoint         string
	JwksURI                  string
	SigningAlgs              []string
	TokenEndpointAuthMethods []string
}

// OIDCAdapter belongs to one authorization transaction. Only verified ID Token
// claims can establish an identity; UserInfo is optional profile enrichment.
type OIDCAdapter struct {
	*GenericOAuth2Adapter
	Issuer      string
	client      *http.Client
	verifier    *oidc.IDTokenVerifier
	jwksURI     string
	signingAlgs []string
	authMethod  string
	nonce       string
	pkce        string
}

func NewOIDCAdapter(provider *Provider, resolver OIDCResolver) (*OIDCAdapter, error) {
	if provider == nil || resolver == nil {
		return nil, fmt.Errorf("OIDC provider 和 discovery resolver 不能为空")
	}
	issuer, _ := getAttrString(provider.Attributes, "issuer")
	if err := oidcclient.ValidateURL(issuer, "issuer"); err != nil {
		return nil, err
	}
	discoveryURL := ""
	if value, exists := provider.Attributes["discoveryUrl"]; exists {
		var ok bool
		discoveryURL, ok = value.(string)
		if !ok {
			return nil, fmt.Errorf("OIDC discoveryUrl 必须是字符串，留空使用标准发现地址")
		}
	}
	doc, err := resolver.Resolve(context.Background(), issuer, discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery 失败: %w", err)
	}
	if doc == nil || doc.Issuer != issuer {
		return nil, fmt.Errorf("OIDC discovery issuer 与配置不匹配")
	}
	for _, endpoint := range []struct{ name, value string }{
		{"authorization_endpoint", doc.AuthorizationEndpoint},
		{"token_endpoint", doc.TokenEndpoint},
		{"jwks_uri", doc.JwksURI},
	} {
		if err := oidcclient.ValidateURL(endpoint.value, endpoint.name); err != nil {
			return nil, err
		}
	}
	if doc.UserinfoEndpoint != "" {
		if err := oidcclient.ValidateURL(doc.UserinfoEndpoint, "userinfo_endpoint"); err != nil {
			return nil, err
		}
	}
	// Do not mutate the caller's configuration or fall back to stale endpoints.
	p := *provider
	p.AuthURL, p.TokenURL, p.UserInfoURL = doc.AuthorizationEndpoint, doc.TokenEndpoint, doc.UserinfoEndpoint
	if strings.TrimSpace(p.Scopes) == "" {
		p.Scopes = oidcDefaultScp
	}
	scopes := []string{"openid"}
	for _, scope := range strings.FieldsFunc(p.Scopes, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	p.Scopes = strings.Join(scopes, ",")
	method, _ := getAttrString(p.Attributes, "tokenEndpointAuthMethod")
	if method == "" {
		method = "auto"
	}
	if method == "auto" && len(doc.TokenEndpointAuthMethods) > 0 {
		switch {
		case slices.Contains(doc.TokenEndpointAuthMethods, "client_secret_basic"):
			method = "client_secret_basic"
		case slices.Contains(doc.TokenEndpointAuthMethods, "client_secret_post"):
			method = "client_secret_post"
		default:
			return nil, fmt.Errorf("OIDC Token 端点不支持客户端密钥认证；公共客户端请选择 none")
		}
	}
	base := NewGenericOAuth2Adapter(&p)
	switch method {
	case "auto":
		base.Config.Endpoint.AuthStyle = oauth2.AuthStyleAutoDetect
	case "client_secret_basic":
		base.Config.Endpoint.AuthStyle = oauth2.AuthStyleInHeader
	case "client_secret_post", "none":
		base.Config.Endpoint.AuthStyle = oauth2.AuthStyleInParams
	default:
		return nil, fmt.Errorf("不支持的 OIDC Token 端点认证方式 %q", method)
	}
	if method != "auto" && len(doc.TokenEndpointAuthMethods) > 0 && !slices.Contains(doc.TokenEndpointAuthMethods, method) {
		return nil, fmt.Errorf("OIDC Token 端点不支持认证方式 %s", method)
	}
	if method == "none" {
		base.Config.ClientSecret = ""
	}

	algorithms := []string{}
	for _, alg := range doc.SigningAlgs {
		if supportedOIDCSigningAlgorithm(alg) {
			algorithms = append(algorithms, alg)
		}
	}
	if len(doc.SigningAlgs) == 0 {
		algorithms = []string{"RS256"}
	} else if len(algorithms) == 0 {
		return nil, fmt.Errorf("OIDC 未提供受支持的非对称签名算法")
	}
	client := oidcclient.NewHTTPClient()
	ctx := oidc.ClientContext(context.Background(), client)
	verifier := oidc.NewVerifier(issuer, safeOIDCKeySet{oidc.NewRemoteKeySet(ctx, doc.JwksURI)}, &oidc.Config{
		ClientID: p.ClientID, SupportedSigningAlgs: algorithms,
	})
	return &OIDCAdapter{GenericOAuth2Adapter: base, Issuer: issuer, client: client, verifier: verifier, authMethod: method, jwksURI: doc.JwksURI, signingAlgs: algorithms}, nil
}

func supportedOIDCSigningAlgorithm(algorithm string) bool {
	switch algorithm {
	case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA":
		return true
	}
	return false
}

// RemoteKeySet includes upstream HTTP bodies in some errors. Keep its caching
// and verification, but direct users to the safe management diagnostics instead
// of reflecting those bodies (or endpoint query credentials) into login errors.
type safeOIDCKeySet struct{ oidc.KeySet }

func (s safeOIDCKeySet) VerifySignature(ctx context.Context, token string) ([]byte, error) {
	payload, err := s.KeySet.VerifySignature(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("OIDC JWKS 签名校验失败，请在管理端检查连接以确认公钥访问及签名算法配置")
	}
	return payload, nil
}

func (a *OIDCAdapter) ConfigureTransaction(nonce, verifier string) error {
	if nonce == "" || len(verifier) < 43 || len(verifier) > 128 || strings.IndexFunc(verifier, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("-._~", r))
	}) >= 0 {
		return fmt.Errorf("OIDC 认证事务缺少有效的 nonce 或 PKCE verifier")
	}
	a.nonce, a.pkce = nonce, verifier
	return nil
}

func (a *OIDCAdapter) GetAuthURL(state string) string {
	if a.nonce == "" || a.pkce == "" {
		return ""
	}
	return a.Config.AuthCodeURL(state, oidc.Nonce(a.nonce), oauth2.S256ChallengeOption(a.pkce))
}

func (a *OIDCAdapter) ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error) {
	if a.nonce == "" || a.pkce == "" {
		return nil, fmt.Errorf("OIDC 认证事务未配置")
	}
	return a.Config.Exchange(oidc.ClientContext(ctx, a.client), code, oauth2.VerifierOption(a.pkce))
}

func (a *OIDCAdapter) GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error) {
	if a.nonce == "" || token == nil {
		return nil, fmt.Errorf("OIDC 认证事务或令牌缺失")
	}
	rawToken, _ := token.Extra("id_token").(string)
	if rawToken == "" {
		return nil, fmt.Errorf("OIDC 响应缺少 id_token")
	}
	idToken, err := a.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("OIDC ID Token 校验失败: %w", err)
	}
	// Enforce exact matching even for providers with library compatibility exceptions.
	if idToken.Issuer != a.Issuer {
		return nil, fmt.Errorf("OIDC ID Token issuer 不匹配")
	}
	if idToken.Subject == "" {
		return nil, fmt.Errorf("OIDC ID Token 缺少 sub")
	}
	if idToken.IssuedAt.IsZero() || idToken.IssuedAt.After(time.Now().Add(time.Minute)) || idToken.IssuedAt.After(idToken.Expiry) {
		return nil, fmt.Errorf("OIDC ID Token iat 无效")
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(a.nonce)) != 1 {
		return nil, fmt.Errorf("OIDC ID Token nonce 不匹配")
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("OIDC ID Token claims 无效: %w", err)
	}
	azp, hasAZP := claims["azp"]
	if (hasAZP && azp != a.Config.ClientID) || (len(idToken.Audience) > 1 && !hasAZP) {
		return nil, fmt.Errorf("OIDC ID Token azp 不匹配或缺失")
	}
	if _, exists := claims["at_hash"]; exists {
		if err := idToken.VerifyAccessToken(token.AccessToken); err != nil {
			return nil, fmt.Errorf("OIDC access token 校验失败: %w", err)
		}
	}
	if a.Provider.UserInfoURL != "" {
		info, err := a.fetchUserInfo(ctx, token.AccessToken)
		if err != nil {
			return nil, err
		}
		if sub, ok := info["sub"].(string); !ok || sub != idToken.Subject {
			return nil, fmt.Errorf("OIDC UserInfo sub 与 ID Token 不匹配")
		}
		// Only profile claims can be enriched; preserve validated security claims.
		for _, key := range []string{"name", "nickname", "preferred_username", "picture", "email", "email_verified", "phone_number", "phone_number_verified"} {
			if value, exists := info[key]; exists {
				claims[key] = value
			}
		}
	}
	claimString := func(key string) string { value, _ := claims[key].(string); return value }
	name := claimString("name")
	if name == "" {
		name = claimString("preferred_username")
	}
	if name == "" {
		name = claimString("nickname")
	}
	return &SocialUser{OpenID: idToken.Subject, Nickname: name, Avatar: claimString("picture"), Email: claimString("email"), Phone: claimString("phone_number"), RawUserInfo: claims}, nil
}

func (a *OIDCAdapter) fetchUserInfo(ctx context.Context, accessToken string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Provider.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OIDC UserInfo 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC UserInfo 响应异常: %d", resp.StatusCode)
	}
	var claims map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&claims); err != nil {
		return nil, fmt.Errorf("OIDC UserInfo 格式无效: %w", err)
	}
	return claims, nil
}

func (a *OIDCAdapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("OIDC ClientID 不能为空")
	}
	if a.authMethod != "none" && a.Provider.ClientSecret == "" {
		return fmt.Errorf("OIDC ClientSecret 不能为空（公共客户端请选择 none）")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("OIDC 回调 URL 不能为空")
	}
	return nil
}

func getAttrString(attrs map[string]any, key string) (string, bool) {
	value, ok := attrs[key].(string)
	return value, ok
}
