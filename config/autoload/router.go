package autoload

import (
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type Router struct {
}

// InitConfig 返回路由注册函数。真实路由表在 Phase 5 完成后写入，
// 当前为占位实现以便整体工程能编译通过。
func (receiver Router) InitConfig() map[string]any {
	return map[string]any{
		"http.router": func(router httpInterfaces.RouterInterface) {
			// TODO(phase5): 在 migrate controller/middleware 完成后重建完整路由表
		},
	}
}
