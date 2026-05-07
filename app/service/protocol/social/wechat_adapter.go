package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"golang.org/x/oauth2"
)

const (
	wechatAuthURL     = "https://open.weixin.qq.com/connect/qrconnect"
	wechatTokenURL    = "https://api.weixin.qq.com/sns/oauth2/access_token"
	wechatUserInfoURL = "https://api.weixin.qq.com/sns/userinfo"
	wechatDefaultScp  = "snsapi_login"
)

// WeChatAdapter 微信网页/公众号（扫码）登录。
// 微信的 OAuth2 与标准流程有 3 个差异：
//  1. 授权 URL 用 appid 而非 client_id。
//  2. token URL 用 appid/secret 作为 query 参数，而不是标准的 Basic/Form。
//  3. userinfo 需要 access_token + openid 作 query 参数。
type WeChatAdapter struct {
	*GenericOAuth2Adapter
}

func NewWeChatAdapter(provider *model.SocialProvider) *WeChatAdapter {
	provider.AuthURL = wechatAuthURL
	provider.TokenURL = wechatTokenURL
	provider.UserInfoURL = wechatUserInfoURL
	if provider.Scopes == "" {
		provider.Scopes = wechatDefaultScp
	}
	return &WeChatAdapter{GenericOAuth2Adapter: NewGenericOAuth2Adapter(provider)}
}

func (a *WeChatAdapter) GetAuthURL(state string) string {
	q := url.Values{}
	q.Set("appid", a.Provider.ClientID)
	q.Set("redirect_uri", a.Provider.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", wechatDefaultScp)
	if a.Provider.Scopes != "" {
		q.Set("scope", a.Provider.Scopes)
	}
	q.Set("state", state)
	return wechatAuthURL + "?" + q.Encode() + "#wechat_redirect"
}

type wechatTokenResp struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	Scope        string `json:"scope"`
	UnionID      string `json:"unionid"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

func (a *WeChatAdapter) ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error) {
	q := url.Values{}
	q.Set("appid", a.Provider.ClientID)
	q.Set("secret", a.Provider.ClientSecret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wechatTokenURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("微信换 token 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var tr wechatTokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("微信 token 解析失败: %w", err)
	}
	if tr.ErrCode != 0 {
		return nil, fmt.Errorf("微信 token 错误 %d: %s", tr.ErrCode, tr.ErrMsg)
	}

	tok := &oauth2.Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}
	// 把 openid/unionid 一并塞进 token extras 给后面 userinfo 用。
	extras := map[string]any{"openid": tr.OpenID}
	if tr.UnionID != "" {
		extras["unionid"] = tr.UnionID
	}
	return tok.WithExtra(extras), nil
}

func (a *WeChatAdapter) GetUserInfo(ctx context.Context, token *oauth2.Token) (*SocialUser, error) {
	openID, _ := token.Extra("openid").(string)
	if openID == "" {
		return nil, fmt.Errorf("微信 access_token 缺少 openid")
	}

	q := url.Values{}
	q.Set("access_token", token.AccessToken)
	q.Set("openid", openID)
	q.Set("lang", "zh_CN")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wechatUserInfoURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("微信 userinfo 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("微信 userinfo 解析失败: %w", err)
	}
	if errcode, ok := raw["errcode"]; ok {
		if toFloat(errcode) != 0 {
			errmsg, _ := raw["errmsg"].(string)
			return nil, fmt.Errorf("微信 userinfo 错误 %v: %s", errcode, errmsg)
		}
	}

	user := &SocialUser{
		OpenID:      openID,
		RawUserInfo: raw,
	}
	if v, ok := raw["unionid"].(string); ok && v != "" {
		user.UnionID = v
	} else if v, ok := token.Extra("unionid").(string); ok && v != "" {
		user.UnionID = v
	}
	if v, ok := raw["nickname"].(string); ok {
		user.Nickname = v
	}
	if v, ok := raw["headimgurl"].(string); ok {
		user.Avatar = v
	}
	return user, nil
}

func (a *WeChatAdapter) ValidateConfig() error {
	if a.Provider.ClientID == "" {
		return fmt.Errorf("微信 AppID 不能为空")
	}
	if a.Provider.ClientSecret == "" {
		return fmt.Errorf("微信 AppSecret 不能为空")
	}
	if a.Provider.RedirectURL == "" {
		return fmt.Errorf("微信回调 URL 不能为空")
	}
	return nil
}
