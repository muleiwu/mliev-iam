package controller

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/reload"
	"github.com/gin-gonic/gin"
)

// InstallController 系统安装控制器
type InstallController struct {
	BaseResponse
}

// CheckInstall 检查系统安装状态
// GET /api/install/check
func (ic InstallController) CheckInstall(c *gin.Context, helper interfaces.HelperInterface) {
	installService := service.NewInstallService(helper.GetDatabase(), helper)
	response := installService.CheckInstallStatus()

	ic.Success(c, response)
}

// TestConnection 测试数据库连接
// POST /api/install/test-connection
func (ic InstallController) TestConnection(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.DatabaseConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	installService := service.NewInstallService(helper.GetDatabase(), helper)

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

// SubmitInstall 提交系统安装
// POST /api/install/submit
func (ic InstallController) SubmitInstall(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.InstallSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	installService := service.NewInstallService(helper.GetDatabase(), helper)

	// 1. 检查系统是否已安装
	status := installService.CheckInstallStatus()
	if status.Installed {
		ic.Error(c, constants.ErrCodeBadRequest, "系统已安装，无法重复安装")
		return
	}

	// 2. 测试数据库连接并获取连接实例
	testDB, err := installService.TestDatabaseConnection(req.Database)
	if err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "数据库连接测试失败: "+err.Error())
		return
	}
	// 确保在函数退出时关闭数据库连接
	defer func() {
		if sqlDB, err := testDB.DB(); err == nil {
			sqlDB.Close()
		}
	}()

	// 3. 使用测试成功的数据库连接创建新的 service 实例
	newInstallService := service.NewInstallService(testDB, helper)

	// 4. 更新数据库配置到配置文件
	if err := newInstallService.UpdateDatabaseConfig(req.Database); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "更新数据库配置失败: "+err.Error())
		return
	}

	// 5. 创建初始数据（部门、角色、权限、管理员）
	if err := newInstallService.CreateInitialData(req.Admin); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "创建初始数据失败: "+err.Error())
		return
	}

	// 6. 标记系统为已安装
	if err := newInstallService.MarkAsInstalled(); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "标记系统已安装失败: "+err.Error())
		return
	}

	// 7. 返回成功响应
	response := dto.InstallSubmitResponse{
		Success: true,
		Message: "系统安装成功，服务将自动重启以应用新配置，请稍后使用管理员账户登录",
	}

	ic.SuccessWithMessage(c, "系统安装成功", response)

	// 8. 异步触发配置重载（在响应返回后）
	go func() {
		time.Sleep(500 * time.Millisecond) // 短暂延迟，确保响应已发送
		reload.TriggerReload()
		helper.GetLogger().Info("安装完成，已触发配置重载")
	}()
}
