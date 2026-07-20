package directory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// directoryJSONRequest provides the same bounded retry policy to all vendor
// adapters. Authentication/permission errors are intentionally not retried.
func directoryJSONRequest(ctx context.Context, client *http.Client, method, endpoint string, payload any, target any, provider string) error {
	var encoded []byte
	var err error
	if payload != nil {
		encoded, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return err
		}
		if payload != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("%s API 网络请求失败: %w", provider, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		_ = response.Body.Close()
		if readErr != nil {
			return readErr
		}
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			if attempt == 2 {
				return fmt.Errorf("%s API 暂时不可用: HTTP %d", provider, response.StatusCode)
			}
			delay, _ := strconv.Atoi(response.Header.Get("Retry-After"))
			if delay < 1 || delay > 2 {
				delay = attempt + 1
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(delay) * 100 * time.Millisecond):
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("%s API 请求失败: HTTP %d: %s", provider, response.StatusCode, strings.TrimSpace(string(body)))
		}
		if err := json.Unmarshal(body, target); err != nil {
			return fmt.Errorf("解析%s API 响应失败: %w", provider, err)
		}
		return nil
	}
	return fmt.Errorf("%s API 请求重试耗尽", provider)
}
