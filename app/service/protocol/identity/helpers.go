package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

const defaultHTTPTimeout = 10 * time.Second

type baseAdapter struct {
	Provider *model.IdentityProvider
	Client   *http.Client
}

func newBaseAdapter(provider *model.IdentityProvider) baseAdapter {
	return baseAdapter{
		Provider: provider,
		Client:   &http.Client{Timeout: defaultHTTPTimeout},
	}
}

func (a *baseAdapter) GetProviderType() model.IdentityProviderType {
	return a.Provider.Type
}

func (a *baseAdapter) scopes(defaultScopes []string) []string {
	scopes := a.Provider.GetScopes()
	if len(scopes) > 0 {
		return scopes
	}
	return defaultScopes
}

func getString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := raw[key]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func getMap(raw map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if v, ok := raw[key]; ok && v != nil {
			if typed, ok := v.(map[string]any); ok {
				return typed
			}
		}
	}
	return nil
}

func endpoint(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

func buildURL(raw string, values url.Values) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	for key, vals := range values {
		for _, val := range vals {
			q.Set(key, val)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func postJSON(ctx context.Context, client *http.Client, rawURL string, body any, headers map[string]string) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	for key, val := range headers {
		req.Header.Set(key, val)
	}
	return doJSON(client, req)
}

func getJSON(ctx context.Context, client *http.Client, rawURL string, headers map[string]string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for key, val := range headers {
		req.Header.Set(key, val)
	}
	return doJSON(client, req)
}

func doJSON(client *http.Client, req *http.Request) (map[string]any, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s status %d: %s", req.URL.Host, resp.StatusCode, string(body))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func responseCode(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if code, ok := raw["code"]; ok {
		return fmt.Sprintf("%v", code)
	}
	if errCode, ok := raw["errcode"]; ok {
		return fmt.Sprintf("%v", errCode)
	}
	return ""
}

func ensureSuccess(raw map[string]any, platform string) error {
	code := responseCode(raw)
	if code == "" || code == "0" || code == "0.0" {
		return nil
	}
	msg := getString(raw, "msg", "errmsg", "error_description", "error")
	if msg == "" {
		msg = fmt.Sprintf("%v", raw)
	}
	return fmt.Errorf("%s API 返回错误 code=%s: %s", platform, code, msg)
}

func flattenData(raw map[string]any) map[string]any {
	if data := getMap(raw, "data", "result"); data != nil {
		return data
	}
	return raw
}
