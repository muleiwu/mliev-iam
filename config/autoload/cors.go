package autoload

import envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"

// Cors CORS 配置装配；支持通过环境变量覆盖
//
//	app.cors.allowed_origins  —— 允许的 Origin 白名单（逗号分隔或数组）
//	app.cors.allowed_methods  —— 允许的 HTTP 方法
//	app.cors.allowed_headers  —— 允许的请求头
//	app.cors.expose_headers   —— 暴露给前端的响应头
//	app.cors.allow_credentials—— 是否允许携带凭证（与 * 互斥，启动时校验）
//	app.cors.max_age          —— 预检缓存秒数
type Cors struct{}

func (receiver Cors) InitConfig(helper envInterface.HelperInterface) map[string]any {
	env := helper.GetEnv()
	return map[string]any{
		"app.cors.allowed_origins": env.GetStringSlice("app.cors.allowed_origins", []string{
			"http://localhost:5173",
			"http://localhost:5174",
			"http://localhost:5777",
		}),
		"app.cors.allowed_methods": env.GetStringSlice("app.cors.allowed_methods", []string{
			"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS",
		}),
		"app.cors.allowed_headers": env.GetStringSlice("app.cors.allowed_headers", []string{
			"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-Trace-Id",
		}),
		"app.cors.expose_headers": env.GetStringSlice("app.cors.expose_headers", []string{
			"Content-Length", "Content-Type", "X-Trace-Id",
		}),
		"app.cors.allow_credentials": env.GetBool("app.cors.allow_credentials", true),
		"app.cors.max_age":           env.GetInt("app.cors.max_age", 86400),
	}
}
