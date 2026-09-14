package autoload

import (
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type Middleware struct{ Extra []httpInterfaces.HandlerFunc }

func (receiver Middleware) InitConfig() map[string]any {
	return map[string]any{
		"http.middleware": append([]httpInterfaces.HandlerFunc{
			middleware.CorsMiddleware(), // 跨域中间件（读取 app.cors.* 配置）
			func(c httpInterfaces.RouterContextInterface) {
				container.MustGet[*service.SetupInstaller]().Middleware()(c)
			},
		}, receiver.Extra...),
	}
}
