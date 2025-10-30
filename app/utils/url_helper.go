package utils

import (
	"fmt"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// GetBaseURL 获取基础URL，支持反向代理场景
// 优先级：X-Forwarded headers > 配置文件 > 请求headers
func GetBaseURL(c *gin.Context, helper interfaces.HelperInterface) string {
	// 1. 检查反向代理 headers
	forwardedProto := c.GetHeader("X-Forwarded-Proto")
	forwardedHost := c.GetHeader("X-Forwarded-Host")

	if forwardedProto != "" && forwardedHost != "" {
		return strings.TrimSuffix(fmt.Sprintf("%s://%s/oidc", forwardedProto, forwardedHost), "/")
	}

	// 2. 从配置文件获取 issuer
	configuredIssuer := helper.GetEnv().GetString("oidc.issuer", "")
	if configuredIssuer != "" && configuredIssuer != "http://localhost:8080/oidc" {
		return strings.TrimSuffix(configuredIssuer, "/")
	}

	// 3. 回退到请求信息
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	// 如果有 X-Forwarded-Proto 但没有 X-Forwarded-Host
	if forwardedProto != "" {
		scheme = forwardedProto
	}

	host := c.Request.Host
	if forwardedHost != "" {
		host = forwardedHost
	}

	return fmt.Sprintf("%s://%s/oidc", scheme, host)
}
