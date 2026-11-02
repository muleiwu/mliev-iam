package protocol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/oidcclient"
	"github.com/redis/go-redis/v9"
)

// OIDCDiscoveryDoc 保留外部认证校验及管理端展示使用的 discovery 元数据。
type OIDCDiscoveryDoc struct {
	Issuer                   string   `json:"issuer"`
	AuthorizationEndpoint    string   `json:"authorization_endpoint"`
	TokenEndpoint            string   `json:"token_endpoint"`
	UserinfoEndpoint         string   `json:"userinfo_endpoint"`
	JwksURI                  string   `json:"jwks_uri"`
	ScopesSupported          []string `json:"scopes_supported,omitempty"`
	SigningAlgs              []string `json:"id_token_signing_alg_values_supported,omitempty"`
	TokenEndpointAuthMethods []string `json:"token_endpoint_auth_methods_supported,omitempty"`
}

// OIDCDiscoveryCache 带 Redis 缓存的 OIDC 发现文档解析器。
// key=`oidc:discovery:<sha256(issuer)>`，TTL 24h。
type OIDCDiscoveryCache struct {
	redis  *redis.Client
	client *http.Client
	prefix string
	ttl    time.Duration
}

func NewOIDCDiscoveryCache(rdb *redis.Client) *OIDCDiscoveryCache {
	return &OIDCDiscoveryCache{
		redis:  rdb,
		client: oidcclient.NewHTTPClient(),
		prefix: "oidc:discovery:",
		ttl:    24 * time.Hour,
	}
}

// Resolve 先查缓存；未命中或失败则走 HTTP 获取并写回缓存。
func (c *OIDCDiscoveryCache) Resolve(ctx context.Context, issuer string) (*OIDCDiscoveryDoc, error) {
	if err := oidcclient.ValidateURL(issuer, "issuer"); err != nil {
		return nil, err
	}

	if c.redis != nil {
		if doc, ok := c.readCache(ctx, issuer); ok {
			return doc, nil
		}
	}

	return c.FetchAndCache(ctx, issuer)
}

// FetchAndCache 总是走网络获取，然后写缓存（管理端"从 Issuer 拉取"按钮调用）。
func (c *OIDCDiscoveryCache) FetchAndCache(ctx context.Context, issuer string) (*OIDCDiscoveryDoc, error) {
	doc, err := c.fetch(ctx, issuer)
	if err != nil {
		return nil, err
	}
	if c.redis != nil {
		c.writeCache(ctx, issuer, doc)
	}
	return doc, nil
}

func (c *OIDCDiscoveryCache) fetch(ctx context.Context, issuer string) (*OIDCDiscoveryDoc, error) {
	if err := oidcclient.ValidateURL(issuer, "issuer"); err != nil {
		return nil, err
	}
	url := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取 discovery 文档失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery 响应异常: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 discovery 响应失败: %w", err)
	}

	var doc OIDCDiscoveryDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("解析 discovery 文档失败: %w", err)
	}

	if err := validateOIDCDiscovery(&doc, issuer); err != nil {
		return nil, err
	}

	return &doc, nil
}

func (c *OIDCDiscoveryCache) cacheKey(issuer string) string {
	sum := sha256.Sum256([]byte(issuer))
	return c.prefix + hex.EncodeToString(sum[:])
}

func (c *OIDCDiscoveryCache) readCache(ctx context.Context, issuer string) (*OIDCDiscoveryDoc, bool) {
	data, err := c.redis.Get(ctx, c.cacheKey(issuer)).Result()
	if err != nil {
		return nil, false
	}
	var doc OIDCDiscoveryDoc
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return nil, false
	}
	if err := validateOIDCDiscovery(&doc, issuer); err != nil {
		return nil, false
	}
	return &doc, true
}

func validateOIDCDiscovery(doc *OIDCDiscoveryDoc, issuer string) error {
	if doc.Issuer != issuer {
		return fmt.Errorf("discovery issuer 与配置不匹配")
	}
	for _, endpoint := range []struct{ name, value string }{
		{"authorization_endpoint", doc.AuthorizationEndpoint},
		{"token_endpoint", doc.TokenEndpoint},
		{"jwks_uri", doc.JwksURI},
	} {
		if err := oidcclient.ValidateURL(endpoint.value, endpoint.name); err != nil {
			return err
		}
	}
	if doc.UserinfoEndpoint != "" {
		return oidcclient.ValidateURL(doc.UserinfoEndpoint, "userinfo_endpoint")
	}
	return nil
}

func (c *OIDCDiscoveryCache) writeCache(ctx context.Context, issuer string, doc *OIDCDiscoveryDoc) {
	data, err := json.Marshal(doc)
	if err != nil {
		return
	}
	_ = c.redis.Set(ctx, c.cacheKey(issuer), data, c.ttl).Err()
}
