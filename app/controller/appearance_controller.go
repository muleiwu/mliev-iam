package controller

import (
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type AppearanceController struct{ BaseResponse }

func (c AppearanceController) Get(ctx httpInterfaces.RouterContextInterface) {
	config, err := service.NewSettingService(helper.GetDatabase()).GetPublicAppearance()
	ctx.SetHeader("Cache-Control", "no-store")
	if err != nil {
		c.Error(ctx, 500, "读取界面配置失败")
		return
	}
	c.Success(ctx, config)
}
