package admin

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

func (c SettingController) GetAppearanceSettings(ctx httpInterfaces.RouterContextInterface) {
	config, err := service.NewSettingService(helper.GetDatabase()).GetAppearanceSettings()
	if err != nil {
		c.Error(ctx, 500, "读取界面配置失败")
		return
	}
	ctx.SetHeader("Cache-Control", "no-store")
	c.Success(ctx, config)
}

func (c SettingController) UpdateAppearanceSettings(ctx httpInterfaces.RouterContextInterface) {
	var config dto.AppearanceSettings
	if err := ctx.ShouldBindJSON(&config); err != nil {
		c.Error(ctx, 400, "界面配置格式无效")
		return
	}
	if err := service.ValidateAppearanceSettings(&config); err != nil {
		c.Error(ctx, 400, err.Error())
		return
	}
	if err := service.NewSettingService(helper.GetDatabase()).UpdateAppearanceSettings(config); err != nil {
		c.Error(ctx, 500, "保存界面配置失败")
		return
	}
	c.Success(ctx, config)
}
