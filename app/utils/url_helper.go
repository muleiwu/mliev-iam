package utils

import (
	"fmt"
	"strings"

	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// GetBaseURL 获取基础URL，支持反向代理场景
// 优先级：X-Forwarded-{Proto,Host} > TLS 探测 + Request.Host
func GetBaseURL(c httpInterfaces.RouterContextInterface, path string) string {
	forwardedProto := c.GetHeader("X-Forwarded-Proto")
	forwardedHost := c.GetHeader("X-Forwarded-Host")

	if forwardedProto != "" && forwardedHost != "" {
		return strings.TrimSuffix(fmt.Sprintf("%s://%s%s", forwardedProto, forwardedHost, path), "/")
	}

	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	if forwardedProto != "" {
		scheme = forwardedProto
	}

	host := c.Request().Host
	if forwardedHost != "" {
		host = forwardedHost
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}
