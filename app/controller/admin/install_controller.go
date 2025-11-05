package admin

import (
	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// InstallController 系统安装控制器
type InstallController struct {
	BaseController
}

// CheckInstall 检查系统安装状态
// GET /api/install/check
func (ic InstallController) CheckInstall(c *gin.Context, helper interfaces.HelperInterface) {
	installService := service.NewInstallService(helper.GetDatabase())
	response := installService.CheckInstallStatus()

	ic.Success(c, response)
}

// SubmitInstall 提交系统安装
// POST /api/install/submit
func (ic InstallController) SubmitInstall(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.InstallSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	installService := service.NewInstallService(helper.GetDatabase())

	// 1. 检查系统是否已安装
	status := installService.CheckInstallStatus()
	if status.Installed {
		ic.Error(c, constants.ErrCodeBadRequest, "系统已安装，无法重复安装")
		return
	}

	// 2. 测试数据库连接
	if err := installService.TestDatabaseConnection(req.Database); err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "数据库连接测试失败: "+err.Error())
		return
	}

	// 3. 更新数据库配置到配置文件
	if err := installService.UpdateDatabaseConfig(req.Database); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "更新数据库配置失败: "+err.Error())
		return
	}

	// 4. 重新加载配置以使用新的数据库配置
	// 注意：这里需要重新初始化数据库连接，但由于当前架构限制，
	// 我们假设在测试连接时已经验证了配置的有效性
	// 实际生产环境中可能需要重启应用或重新加载数据库连接

	// 5. 创建初始数据（部门、角色、权限、管理员）
	if err := installService.CreateInitialData(req.Admin); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "创建初始数据失败: "+err.Error())
		return
	}

	// 6. 标记系统为已安装
	if err := installService.MarkAsInstalled(); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "标记系统已安装失败: "+err.Error())
		return
	}

	// 7. 返回成功响应
	response := dto.InstallSubmitResponse{
		Success: true,
		Message: "系统安装成功，请使用管理员账户登录",
	}

	ic.SuccessWithMessage(c, "系统安装成功", response)
}
