package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

func TestFeishuAdapterMapsUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"access_token": "feishu-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				},
			})
			return
		}
		if r.URL.Path == "/userinfo" {
			if got := r.Header.Get("Authorization"); got != "Bearer feishu-token" {
				t.Fatalf("unexpected authorization header %q", got)
			}
			writeJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"open_id":    "ou_123",
					"union_id":   "on_123",
					"tenant_key": "tenant_a",
					"name":       "Alice",
					"avatar_url": "https://avatar.example/a.png",
					"email":      "alice@example.com",
					"mobile":     "13800000000",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	adapter := NewFeishuAdapter(&model.IdentityProvider{
		Type:         model.IdentityProviderTypeFeishu,
		ClientID:     "app_id",
		ClientSecret: "secret",
		RedirectURL:  "https://iam.example/identity/callback/feishu",
		TokenURL:     server.URL + "/token",
		UserInfoURL:  server.URL + "/userinfo",
	})

	token, err := adapter.ExchangeToken(context.Background(), "code")
	if err != nil {
		t.Fatalf("ExchangeToken() error = %v", err)
	}
	user, err := adapter.GetUserInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("GetUserInfo() error = %v", err)
	}
	if user.ExternalUserID != "ou_123" || user.UnionID != "on_123" || user.TenantID != "tenant_a" {
		t.Fatalf("unexpected user identifiers: %+v", user)
	}
	if user.Nickname != "Alice" || user.Email != "alice@example.com" || user.Phone != "13800000000" {
		t.Fatalf("unexpected user profile: %+v", user)
	}
}

func TestWeComAdapterUsesCorpAndAgentConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.URL.Query().Get("corpid") != "corp_id" || r.URL.Query().Get("corpsecret") != "secret" {
				t.Fatalf("unexpected token query %s", r.URL.RawQuery)
			}
			writeJSON(t, w, map[string]any{
				"errcode":      0,
				"access_token": "wecom-token",
				"expires_in":   7200,
			})
		case "/userinfo":
			if r.URL.Query().Get("access_token") != "wecom-token" || r.URL.Query().Get("code") != "auth-code" {
				t.Fatalf("unexpected userinfo query %s", r.URL.RawQuery)
			}
			writeJSON(t, w, map[string]any{
				"errcode": 0,
				"userid":  "zhangsan",
				"name":    "张三",
				"email":   "zhangsan@example.com",
				"mobile":  "13900000000",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter := NewWeComAdapter(&model.IdentityProvider{
		Type:         model.IdentityProviderTypeWeCom,
		ClientID:     "corp_id",
		ClientSecret: "secret",
		RedirectURL:  "https://iam.example/identity/callback/wecom",
		TokenURL:     server.URL + "/token",
		UserInfoURL:  server.URL + "/userinfo",
		Attributes:   model.JSONObject{"agentId": "1000001"},
	})

	authURL := adapter.GetAuthURL("state")
	if !strings.Contains(authURL, "agentid=1000001") {
		t.Fatalf("auth url does not include agentid: %s", authURL)
	}

	token, err := adapter.ExchangeToken(context.Background(), "auth-code")
	if err != nil {
		t.Fatalf("ExchangeToken() error = %v", err)
	}
	user, err := adapter.GetUserInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("GetUserInfo() error = %v", err)
	}
	if user.ExternalUserID != "zhangsan" || user.TenantID != "corp_id" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestDingTalkAdapterMapsUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			writeJSON(t, w, map[string]any{
				"accessToken":  "ding-token",
				"refreshToken": "refresh",
				"expireIn":     3600,
				"corpId":       "ding-corp",
			})
		case "/userinfo":
			if got := r.Header.Get("x-acs-dingtalk-access-token"); got != "ding-token" {
				t.Fatalf("unexpected ding token header %q", got)
			}
			writeJSON(t, w, map[string]any{
				"userid":    "user-1",
				"unionId":   "union-1",
				"nick":      "Ding User",
				"avatarUrl": "https://avatar.example/ding.png",
				"email":     "ding@example.com",
				"mobile":    "13700000000",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter := NewDingTalkAdapter(&model.IdentityProvider{
		Type:         model.IdentityProviderTypeDingTalk,
		ClientID:     "app_key",
		ClientSecret: "secret",
		RedirectURL:  "https://iam.example/identity/callback/dingtalk",
		TokenURL:     server.URL + "/token",
		UserInfoURL:  server.URL + "/userinfo",
	})

	token, err := adapter.ExchangeToken(context.Background(), "auth-code")
	if err != nil {
		t.Fatalf("ExchangeToken() error = %v", err)
	}
	user, err := adapter.GetUserInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("GetUserInfo() error = %v", err)
	}
	if user.ExternalUserID != "user-1" || user.UnionID != "union-1" || user.TenantID != "ding-corp" {
		t.Fatalf("unexpected user identifiers: %+v", user)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}
