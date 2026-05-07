package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"golang.org/x/oauth2"
)

const (
	qqAuthURL     = "https://graph.qq.com/oauth2.0/authorize"
	qqTokenURL    = "https://graph.qq.com/oauth2.0/token"
	qqOpenIDURL   = "https://graph.qq.com/oauth2.0/me"
	qqUserInfoURL = "https://graph.qq.com/user/get_user_info"
	qqDefaultScp  = "get_user_info"
)

type QQAdapter struct {
	*GenericOAuth2Adapter
}

func NewQQAdapter(provider *model.SocialProvider) *QQAdapter {
	provider.AuthURL = qqAuthURL
	provider.TokenURL = qqTokenURL
	// UserInfoURL 在这里仅作占位；真正的 userinfo 流程由 GetUserInfo 覆写。
	provider.UserInfoURL = qqUserInfoURL
	if provider.Scopes == "" {
		provider.Scopes = qqDefaultScp
	}
	return &QQAdapter{GenericOAuth2Adapter: NewGenericOAuth2Adapter(provider)}
}

// GetUserInfo QQ 互联的 userinfo 分两步：
//  1. GET /oauth2.0/me?access_token=... 返回 JSONP：callback({"client_id":..,"openid":..});
//     需要剥掉外层 callback(...) 拿 openid。
//  2. GET /user/get_user_info?access_token=&oauth_consumer_key=&openid=
func (a *QQAdapter) GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error) {
	openID, err := a.fetchOpenID(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}

	userinfoURL := fmt.Sprintf("%s?access_token=%s&oauth_consumer_key=%s&openid=%s",
		qqUserInfoURL,
		url.QueryEscape(token.AccessToken),
		url.QueryEscape(a.Provider.ClientID),
		url.QueryEscape(openID),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("QQ userinfo 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("QQ userinfo 解析失败: %w", err)
	}
	if ret, ok := raw["ret"]; ok {
		if code := toFloat(ret); code != 0 {
			msg, _ := raw["msg"].(string)
			return nil, fmt.Errorf("QQ userinfo 错误 ret=%v msg=%s", ret, msg)
		}
	}

	user := &SocialUser{
		OpenID:      openID,
		RawUserInfo: raw,
	}
	if v, ok := raw["nickname"].(string); ok {
		user.Nickname = v
	}
	if v, ok := raw["figureurl_qq_2"].(string); ok && v != "" {
		user.Avatar = v
	} else if v, ok := raw["figureurl_qq_1"].(string); ok {
		user.Avatar = v
	}
	return user, nil
}

func (a *QQAdapter) fetchOpenID(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		qqOpenIDURL+"?access_token="+url.QueryEscape(accessToken), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("QQ openid 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 响应形如：callback( {"client_id":"xxx","openid":"xxx"} );
	text := strings.TrimSpace(string(body))
	text = strings.TrimPrefix(text, "callback(")
	text = strings.TrimSuffix(text, ");")
	text = strings.TrimSuffix(text, ")")

	var out struct {
		ClientID string `json:"client_id"`
		OpenID   string `json:"openid"`
		Error    int    `json:"error"`
		ErrDesc  string `json:"error_description"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return "", fmt.Errorf("QQ openid 解析失败: %w (raw=%s)", err, text)
	}
	if out.Error != 0 {
		return "", fmt.Errorf("QQ openid 错误: %s", out.ErrDesc)
	}
	if out.OpenID == "" {
		return "", fmt.Errorf("QQ openid 为空")
	}
	return out.OpenID, nil
}

func (a *QQAdapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("QQ AppID 不能为空")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("QQ AppKey 不能为空")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("QQ 回调 URL 不能为空")
	}
	return nil
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return -1
	}
}
