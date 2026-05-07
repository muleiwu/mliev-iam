package autoload

import (
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type Middleware struct {
}

func (receiver Middleware) InitConfig() map[string]any {
	return map[string]any{
		"http.middleware": []httpInterfaces.HandlerFunc{
			middleware.CorsMiddleware(),         // 跨域中间件（读取 app.cors.* 配置）
			middleware.InstallCheckMiddleware(), // 安装检查中间件（未安装时拦截）
		},
	}
}
