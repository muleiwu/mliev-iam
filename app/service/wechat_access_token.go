package service

import (
	"bytes"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/social"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// WeChatAccessTokenCache 负责获取并缓存微信侧的 access_token（stable_token）。
// 用于服务端 API 调用，例如拉小程序码、解密 encryptedData 等。
// Redis key 为 wechat:mp_access_token:<appid>，TTL 自动取微信返回的 expires_in 减 5min 余量。
type WeChatAccessTokenCache struct {
	redis  *redis.Client
	client *http.Client
	prefix string
}

func NewWeChatAccessTokenCache(rdb *redis.Client) *WeChatAccessTokenCache {
	return &WeChatAccessTokenCache{
		redis:  rdb,
		client: social.NewWeChatHTTPClient(),
		prefix: "wechat:mp_access_token:",
	}
}

type stableTokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

// Resolve 先查 Redis；未命中则向微信获取并写缓存。
func (c *WeChatAccessTokenCache) Resolve(ctx context.Context, appID, appSecret string) (string, error) {
	if appID == "" || appSecret == "" {
		return "", fmt.Errorf("appid/secret 不能为空")
	}
	if c.redis != nil {
		key := c.prefix + appID
		if v, err := c.redis.Get(ctx, key).Result(); err == nil && v != "" {
			return v, nil
		}
	}
	return c.fetchAndCache(ctx, appID, appSecret)
}

func (c *WeChatAccessTokenCache) fetchAndCache(ctx context.Context, appID, appSecret string) (string, error) {
	body, _ := json.Marshal(map[string]string{"grant_type": "client_credential", "appid": appID, "secret": appSecret})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.weixin.qq.com/cgi-bin/stable_token",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	var out stableTokenResp
	if err := social.ReadWeChatJSON(c.client, req, &out); err != nil {
		return "", err
	}
	if out.ErrCode != 0 {
		return "", fmt.Errorf("微信服务凭证获取失败（%d）", out.ErrCode)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("stable_token 返回空 access_token")
	}

	if c.redis != nil {
		ttl := time.Duration(out.ExpiresIn-300) * time.Second
		if ttl <= 0 {
			ttl = 60 * time.Second
		}
		_ = c.redis.Set(ctx, c.prefix+appID, out.AccessToken, ttl).Err()
	}
	return out.AccessToken, nil
}
