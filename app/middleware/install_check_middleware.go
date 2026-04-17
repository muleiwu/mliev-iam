package middleware

import (
	"net/http"
	"path"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// 豁免路径：使用规范化后的绝对路径比较；前缀以 / 结尾确保边界匹配
var installExemptPrefixes = []string{
	"/api/install/",
	"/install/",
}

var installExemptExact = map[string]struct{}{
	"/install":       {},
	"/health":        {},
	"/health/simple": {},
	"/api/install":   {},
	"/favicon.ico":   {},
}

// InstallCheckMiddleware 安装检查中间件
// 如果系统未安装，除豁免路径外的所有请求都将重定向到安装页面
// 通过 path.Clean 规范化路径以防止 /api/install/../admin 形式的路径遍历绕过
func InstallCheckMiddleware(helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		if helper.GetConfig().GetBool("app.installed", false) {
			c.Next()
			return
		}

		// 规范化：先确保以 / 开头，再用 path.Clean 消除 . 和 ..
		raw := c.Request.URL.Path
		if !strings.HasPrefix(raw, "/") {
			raw = "/" + raw
		}
		cleaned := path.Clean(raw)

		if _, ok := installExemptExact[cleaned]; ok {
			c.Next()
			return
		}
		for _, prefix := range installExemptPrefixes {
			if strings.HasPrefix(cleaned+"/", prefix) || strings.HasPrefix(cleaned, prefix) {
				c.Next()
				return
			}
		}

		c.Redirect(http.StatusFound, "/install/")
		c.Abort()
	}
}
