package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type miniPhoneRoundTrip func(*http.Request) (*http.Response, error)

func (f miniPhoneRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMiniProgramPhoneBindsOpenIDAndValidatesWatermark(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer rdb.Close()
	require.NoError(t, rdb.Set(context.Background(), "wechat:mp_access_token:wx-app", "private-token", time.Hour).Err())
	for _, scenario := range []string{"ok", "wrong-app", "expired", "future", "wrong-user", "http-error"} {
		t.Run(scenario, func(t *testing.T) {
			client := newMiniWeChatClient(rdb)
			client.client = &http.Client{Transport: miniPhoneRoundTrip(func(r *http.Request) (*http.Response, error) {
				var body map[string]string
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "verified-openid", body["openid"])
				require.Equal(t, "phone-code", body["code"])
				app := "wx-app"
				now := time.Now().Unix()
				status := 200
				if scenario == "wrong-app" {
					app = "other-app"
				}
				if scenario == "expired" {
					now -= 600
				}
				if scenario == "future" {
					now += 600
				}
				if scenario == "http-error" {
					status = 502
				}
				raw := fmt.Sprintf(`{"phone_info":{"purePhoneNumber":"13800138000","countryCode":"86","watermark":{"appid":%q,"timestamp":%d}}}`, app, now)
				if scenario == "wrong-user" {
					raw = `{"errcode":40029,"errmsg":"private-token"}`
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(raw))}, nil
			})}
			phone, err := client.phone(context.Background(), "wx-app", "app-secret", "verified-openid", "phone-code")
			if scenario == "ok" {
				require.NoError(t, err)
				require.Equal(t, "+8613800138000", phone)
			} else {
				require.Error(t, err)
				require.Empty(t, phone)
				require.NotContains(t, err.Error(), "private-token")
			}
		})
	}
}
