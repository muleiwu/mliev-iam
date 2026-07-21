package directory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type directoryRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn directoryRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestParseRetryAfterSupportsSecondsAndHTTPDate(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if got := parseRetryAfter("3", time.Millisecond, now); got != 3*time.Second {
		t.Fatalf("秒数 Retry-After 解析错误: %s", got)
	}
	date := now.Add(4 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(date, time.Millisecond, now); got != 4*time.Second {
		t.Fatalf("HTTP-date Retry-After 解析错误: %s", got)
	}
}

func TestDirectoryJSONRequestRetriesRateLimitsButNotPermissionErrors(t *testing.T) {
	var attempts atomic.Int32
	client := directoryTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if current < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "rate limited"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	var response map[string]any
	if err := directoryJSONRequest(context.Background(), client, http.MethodGet, "https://provider.test/users", nil, &response, "测试平台"); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 3 || response["ok"] != true {
		t.Fatalf("限流重试错误: attempts=%d response=%#v", attempts.Load(), response)
	}

	var forbiddenAttempts atomic.Int32
	forbidden := directoryTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forbiddenAttempts.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	if err := directoryJSONRequest(context.Background(), forbidden, http.MethodGet, "https://provider.test/users", nil, &response, "测试平台"); err == nil {
		t.Fatal("权限错误必须直接返回")
	}
	if forbiddenAttempts.Load() != 1 {
		t.Fatalf("权限错误不应重试，实际 %d 次", forbiddenAttempts.Load())
	}
}

func directoryTestClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: directoryRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
}
