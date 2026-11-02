package oidcclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
)

// GetJSON reads public OIDC metadata with bounded responses and diagnostics
// that never include upstream response bodies or URL query credentials.
func GetJSON(ctx context.Context, client *http.Client, endpoint, stage string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("OIDC %s 请求地址无效，请检查端点配置", stage)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		var networkError net.Error
		switch {
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("OIDC %s 请求已取消", stage)
		case errors.As(err, &networkError) && networkError.Timeout():
			return fmt.Errorf("OIDC %s 请求超时，请检查网络和提供方服务", stage)
		default:
			return fmt.Errorf("OIDC %s 请求失败，请检查地址、网络和 TLS 证书", stage)
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		advice := "请检查端点配置及提供方服务状态"
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			advice = "请检查提供方访问控制及 CDN/WAF 规则，允许 IAM 服务端直接读取公开配置"
		} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			advice = "不支持重定向，请填写可直接返回 JSON 的端点地址"
		}
		return fmt.Errorf("OIDC %s 请求失败（HTTP %d）：%s", stage, resp.StatusCode, advice)
	}
	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return fmt.Errorf("OIDC %s 响应读取失败（HTTP 200），请检查网络及提供方服务", stage)
	}
	if len(body) > maxBody {
		return fmt.Errorf("OIDC %s 响应超过 1 MiB（HTTP 200），请检查端点配置", stage)
	}
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("<")) {
		return fmt.Errorf("OIDC %s 返回 HTML/验证页面（HTTP 200），请确认 JSON 端点并检查提供方 CDN/WAF 放行规则", stage)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("OIDC %s 返回无效 JSON 或数据格式（HTTP 200），请检查端点配置", stage)
	}
	return nil
}
