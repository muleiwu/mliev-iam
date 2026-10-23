package social

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

type miniRoundTrip func(*http.Request) (*http.Response, error)

func (f miniRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestWeChatMiniProgramExchangeKeepsSecretsOutOfProfileAndErrors(t *testing.T) {
	a := NewWeChatMiniProgramAdapter(&Provider{ClientID: "wx-app", ClientSecret: "secret-must-not-leak"})
	a.client = &http.Client{Transport: miniRoundTrip(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "wx-app", r.URL.Query().Get("appid"))
		require.Equal(t, "js-code", r.URL.Query().Get("js_code"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"openid":"open","unionid":"union","session_key":"session-secret"}`))}, nil
	})}
	user, err := a.LoginWithCode(context.Background(), "js-code")
	require.NoError(t, err)
	require.Equal(t, "open", user.OpenID)
	require.NotContains(t, user.RawUserInfo, "session_key")
	a.client.Transport = miniRoundTrip(func(r *http.Request) (*http.Response, error) { return nil, errors.New(r.URL.String()) })
	_, err = a.LoginWithCode(context.Background(), "js-code")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-must-not-leak")
	require.NotContains(t, err.Error(), "js-code")
	for _, body := range []string{`{"errcode":40029,"errmsg":"secret-must-not-leak"}`, `{`, strings.Repeat("x", (1<<20)+1), `{"openid":""}`} {
		a.client.Transport = miniRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		_, err = a.LoginWithCode(context.Background(), "js-code")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret-must-not-leak")
	}
}
