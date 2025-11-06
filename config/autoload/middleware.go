package autoload

import (
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

type Middleware struct {
}

func (receiver Middleware) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"http.middleware": []gin.HandlerFunc{
			middleware.CorsMiddleware(helper),         // 跨域中间件
			middleware.InstallCheckMiddleware(helper), // 安装检查中间件
		},
	}
}
