package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// CorsConfig 配置CORS中间件的选项
type CorsConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCorsConfig 返回默认CORS配置（安全默认：本地开发白名单，非通配符）
func DefaultCorsConfig() CorsConfig {
	return CorsConfig{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:5174", "http://localhost:5777", "http://localhost:5211"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-Trace-Id"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type", "X-Trace-Id"},
		AllowCredentials: true,
		MaxAge:           86400,
	}
}

// CorsMiddleware 使用配置文件中的 app.cors.* 构建 CORS 中间件。
//
// 注意：本函数在 autoload 阶段被调用构造 HandlerFunc，此时 gsr.Provider 尚未装配完成；
// 因此配置读取必须延迟到首次请求（用 sync.Once 只做一次），避免触发
// "provider gsr.Provider not found" panic。配置非法时（* + AllowCredentials）会 panic。
func CorsMiddleware() httpInterfaces.HandlerFunc {
	var (
		once     sync.Once
		resolved httpInterfaces.HandlerFunc
	)
	return func(c httpInterfaces.RouterContextInterface) {
		once.Do(func() {
			cfg := DefaultCorsConfig()

			if provider := helper.GetConfig(); provider != nil {
				if origins := provider.GetStringSlice("app.cors.allowed_origins", cfg.AllowOrigins); len(origins) > 0 {
					cfg.AllowOrigins = origins
				}
				if methods := provider.GetStringSlice("app.cors.allowed_methods", cfg.AllowMethods); len(methods) > 0 {
					cfg.AllowMethods = methods
				}
				if headers := provider.GetStringSlice("app.cors.allowed_headers", cfg.AllowHeaders); len(headers) > 0 {
					cfg.AllowHeaders = headers
				}
				if exposed := provider.GetStringSlice("app.cors.expose_headers", cfg.ExposeHeaders); len(exposed) > 0 {
					cfg.ExposeHeaders = exposed
				}
				cfg.AllowCredentials = provider.GetBool("app.cors.allow_credentials", cfg.AllowCredentials)
				cfg.MaxAge = provider.GetInt("app.cors.max_age", cfg.MaxAge)
			}

			assertSafeCorsConfig(cfg)
			resolved = CorsWithConfig(cfg)
		})
		resolved(c)
	}
}

// assertSafeCorsConfig 防止出现 `*` 与 AllowCredentials 同时启用的致命误配
func assertSafeCorsConfig(cfg CorsConfig) {
	for _, o := range cfg.AllowOrigins {
		if o == "*" && cfg.AllowCredentials {
			panic("CORS 配置非法：AllowOrigins 含 * 时不能启用 AllowCredentials（违反 CORS 规范）")
		}
	}
}

// CorsWithConfig 使用自定义配置创建CORS中间件（按精确白名单返回 origin）
func CorsWithConfig(config CorsConfig) httpInterfaces.HandlerFunc {
	originSet := make(map[string]struct{}, len(config.AllowOrigins))
	allowAny := false
	for _, o := range config.AllowOrigins {
		if o == "*" {
			allowAny = true
			continue
		}
		originSet[o] = struct{}{}
	}

	return func(c httpInterfaces.RouterContextInterface) {
		origin := c.GetHeader("Origin")
		allowed := ""

		if origin != "" {
			if _, ok := originSet[origin]; ok {
				allowed = origin
			} else if allowAny && !config.AllowCredentials {
				allowed = "*"
			}
		} else if allowAny && !config.AllowCredentials {
			allowed = "*"
		}

		if allowed != "" {
			c.SetHeader("Access-Control-Allow-Origin", allowed)
			c.SetHeader("Vary", "Origin")
		}

		c.SetHeader("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
		c.SetHeader("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))

		if len(config.ExposeHeaders) > 0 {
			c.SetHeader("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ", "))
		}

		if config.AllowCredentials && allowed != "" && allowed != "*" {
			c.SetHeader("Access-Control-Allow-Credentials", "true")
		}

		if config.MaxAge > 0 {
			c.SetHeader("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
		}

		if c.Method() == http.MethodOptions {
			if origin != "" && allowed == "" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
