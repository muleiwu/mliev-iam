package social

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

func NewWeChatHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect denied") }}
}

// ReadWeChatJSON deliberately discards transport errors and response bodies:
// both can contain credentials embedded in WeChat query strings.
func ReadWeChatJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("微信服务请求失败，请重新获取凭证后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("微信服务暂时不可用")
	}
	const maxSize = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil || len(body) > maxSize {
		return errors.New("微信服务响应无效")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return errors.New("微信服务响应无效")
	}
	return nil
}
