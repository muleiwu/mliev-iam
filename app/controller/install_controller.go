package controller

import (
	"errors"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"cnb.cool/mliev/open/go-web/pkg/server/reload"
)

// InstallController 系统安装控制器
type InstallController struct {
	BaseResponse
}

// CheckInstall 检查系统安装状态
// GET /api/install/check
func (ic InstallController) CheckInstall(c httpInterfaces.RouterContextInterface) {
	installService := service.NewInstallService(helper.GetDatabase())
	response := installService.CheckInstallStatus()
	response.Ready = response.Installed && response.DatabaseConnected && helper.GetConfig().GetBool("app.ready", false)

	ic.Success(c, response)
}

// TestConnection 测试数据库连接
// POST /api/install/test-connection
func (ic InstallController) TestConnection(c httpInterfaces.RouterContextInterface) {
	if !ic.installationOpen(c) {
		return
	}
	var req dto.DatabaseConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	installService := service.NewInstallService(helper.GetDatabase())

	// 测试数据库连接
	testDB, err := installService.TestDatabaseConnection(req)
	if err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "数据库连接测试失败: "+err.Error())
		return
	}
	// 测试成功后关闭连接
	if sqlDB, err := testDB.DB(); err == nil {
		sqlDB.Close()
	}

	ic.SuccessWithMessage(c, "数据库连接测试成功", nil)
}

// TestRedisConnection 测试Redis连接
// POST /api/install/test-redis
func (ic InstallController) TestRedisConnection(c httpInterfaces.RouterContextInterface) {
	if !ic.installationOpen(c) {
		return
	}
	var req dto.RedisConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	installService := service.NewInstallService(helper.GetDatabase())

	// 测试Redis连接
	testRedis, err := installService.TestRedisConnection(req)
	if err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "Redis连接测试失败: "+err.Error())
		return
	}
	// 测试成功后关闭连接
	if err := testRedis.Close(); err != nil {
		helper.GetLogger().Warn("关闭Redis测试连接时出错: " + err.Error())
	}

	ic.SuccessWithMessage(c, "Redis连接测试成功", nil)
}

// SubmitInstall 提交系统安装
// POST /api/install/submit
func (ic InstallController) SubmitInstall(c httpInterfaces.RouterContextInterface) {
	var req dto.InstallSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := service.CompleteInstallation(req); err != nil {
		code := constants.ErrCodeBadRequest
		if errors.Is(err, service.ErrAlreadyInstalled) {
			code = 409
		}
		ic.Error(c, code, err.Error())
		return
	}
	ic.SuccessWithMessage(c, "安装配置已保存，正在重启", dto.InstallSubmitResponse{Success: true, Message: "安装配置已保存，正在重启"})
	go func() { time.Sleep(500 * time.Millisecond); reload.Restart() }()
}

// Connection probes belong only to the uninstalled wizard. Keeping them open
// after installation would expose unauthenticated network/filesystem probes.
func (ic InstallController) installationOpen(c httpInterfaces.RouterContextInterface) bool {
	if service.NewInstallService(nil).CheckInstallStatus().Installed {
		ic.Error(c, 409, "系统已安装，安装连接测试已关闭")
		return false
	}
	return true
}
