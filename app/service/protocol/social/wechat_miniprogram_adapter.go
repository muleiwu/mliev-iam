package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

const jscode2sessionURL = "https://api.weixin.qq.com/sns/jscode2session"

// WeChatMiniProgramAdapter 微信小程序登录。
// 与浏览器 OAuth 不同：前端 wx.login 拿到 js_code 后，由后端直接调用 jscode2session 换取 openid/session_key。
// 仅实现 CodeFlowAdapter。
type WeChatMiniProgramAdapter struct {
	provider *model.SocialProvider
	client   *http.Client
}

func NewWeChatMiniProgramAdapter(provider *model.SocialProvider) *WeChatMiniProgramAdapter {
	return &WeChatMiniProgramAdapter{provider: provider, client: http.DefaultClient}
}

func (a *WeChatMiniProgramAdapter) GetProviderType() model.ProviderType {
	return a.provider.Type
}

type jscodeSessionResp struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func (a *WeChatMiniProgramAdapter) LoginWithCode(ctx context.Context, code string) (*SocialUser, error) {
	if code == "" {
		return nil, fmt.Errorf("js_code 不能为空")
	}
	q := url.Values{}
	q.Set("appid", a.provider.ClientID)
	q.Set("secret", a.provider.ClientSecret)
	q.Set("js_code", code)
	q.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jscode2sessionURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jscode2session 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out jscodeSessionResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("jscode2session 解析失败: %w", err)
	}
	if out.ErrCode != 0 {
		return nil, fmt.Errorf("微信小程序登录错误 %d: %s", out.ErrCode, out.ErrMsg)
	}
	if out.OpenID == "" {
		return nil, fmt.Errorf("jscode2session 未返回 openid")
	}

	return &SocialUser{
		OpenID:  out.OpenID,
		UnionID: out.UnionID,
		RawUserInfo: map[string]any{
			"openid":      out.OpenID,
			"session_key": out.SessionKey,
			"unionid":     out.UnionID,
		},
	}, nil
}

func (a *WeChatMiniProgramAdapter) ValidateConfig() error {
	if a.provider.ClientID == "" {
		return fmt.Errorf("小程序 AppID 不能为空")
	}
	if a.provider.ClientSecret == "" {
		return fmt.Errorf("小程序 AppSecret 不能为空")
	}
	return nil
}
