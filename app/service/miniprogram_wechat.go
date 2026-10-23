package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/social"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"github.com/redis/go-redis/v9"
)

// Kept separate from authentication transactions so HTTP fixtures can verify
// openid binding and sanitization without making calls to real WeChat accounts.
type miniWeChatClient struct {
	tokens *WeChatAccessTokenCache
	client *http.Client
}

func newMiniWeChatClient(rdb *redis.Client) *miniWeChatClient {
	return &miniWeChatClient{tokens: NewWeChatAccessTokenCache(rdb), client: social.NewWeChatHTTPClient()}
}
func (w *miniWeChatClient) phone(ctx context.Context, appID, secret, openID, code string) (string, error) {
	token, err := w.tokens.Resolve(ctx, appID, secret)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"code": code, "openid": openID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.weixin.qq.com/wxa/business/getuserphonenumber?access_token="+url.QueryEscape(token), bytes.NewReader(body))
	if err != nil {
		return "", errors.New("微信手机号请求无效")
	}
	req.Header.Set("Content-Type", "application/json")
	var result struct {
		ErrCode int `json:"errcode"`
		Phone   struct {
			Pure      string `json:"purePhoneNumber"`
			Country   string `json:"countryCode"`
			Watermark struct {
				AppID     string `json:"appid"`
				Timestamp int64  `json:"timestamp"`
			} `json:"watermark"`
		} `json:"phone_info"`
	}
	if err := social.ReadWeChatJSON(w.client, req, &result); err != nil {
		return "", err
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("微信手机号验证失败（%d），请重新授权", result.ErrCode)
	}
	mark := result.Phone.Watermark
	if mark.AppID != appID || mark.Timestamp < time.Now().Add(-5*time.Minute).Unix() || mark.Timestamp > time.Now().Add(time.Minute).Unix() {
		return "", errors.New("微信手机号凭证归属或有效期校验失败")
	}
	if result.Phone.Pure == "" || result.Phone.Country == "" {
		return "", errors.New("微信手机号响应无效")
	}
	return utils.NormalizePhoneToE164("+"+result.Phone.Country+result.Phone.Pure, utils.DefaultPhoneRegion)
}
