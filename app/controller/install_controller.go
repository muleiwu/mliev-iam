package controller

import (
	"context"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/spf13/viper"
)

type InstallController struct{ BaseResponse }

// CheckInstall retains the legacy read-only readiness contract for deployment
// probes and the development entrypoint. Installation writes use setup-kit.
func (ic InstallController) CheckInstall(c httpInterfaces.RouterContextInterface) {
	installer := container.MustGet[*service.SetupInstaller]()
	installed := installer.Kit.Status().Installed
	response := &dto.InstallCheckResponse{Installed: installed,
		DatabaseConfigured: installed || (viper.GetString("database.host") != "" && viper.GetString("database.database") != "") || (viper.GetString("database.driver") == "sqlite" && viper.GetString("database.filepath") != ""),
		CurrentDatabase:    viper.GetString("database.database"),
	}
	if db := helper.GetDatabase(); db != nil {
		if sqlDB, err := db.DB(); err == nil {
			ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
			response.DatabaseConnected = sqlDB.PingContext(ctx) == nil
			cancel()
		}
	}
	response.Ready = installed && response.DatabaseConnected && helper.GetConfig().GetBool("app.ready", false)
	if response.Ready {
		response.Message = "系统已安装"
	} else if installed {
		response.Message = "配置已保存，正在等待服务启动"
	} else {
		response.Message = "请完成安装引导"
	}
	c.SetHeader("Cache-Control", "no-store")
	ic.Success(c, response)
}
