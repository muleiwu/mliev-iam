package controller

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/migrations"
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

	ic.Success(c, response)
}

// TestConnection 测试数据库连接
// POST /api/install/test-connection
func (ic InstallController) TestConnection(c httpInterfaces.RouterContextInterface) {
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

	installService := service.NewInstallService(helper.GetDatabase())

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

	// 3. 测试Redis连接
	testRedis, err := installService.TestRedisConnection(req.Redis)
	if err != nil {
		ic.Error(c, constants.ErrCodeBadRequest, "Redis连接测试失败: "+err.Error())
		return
	}
	// 确保在函数退出时关闭Redis连接
	defer func() {
		if err := testRedis.Close(); err != nil {
			helper.GetLogger().Warn("关闭Redis测试连接时出错: " + err.Error())
		}
	}()

	// 4. 使用测试成功的数据库连接创建新的 service 实例
	newInstallService := service.NewInstallService(testDB)

	// 5. 更新数据库配置到配置文件
	if err := newInstallService.UpdateDatabaseConfig(req.Database); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "更新数据库配置失败: "+err.Error())
		return
	}

	// 6. 更新Redis配置到配置文件
	if err := newInstallService.UpdateRedisConfig(req.Redis); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "更新Redis配置失败: "+err.Error())
		return
	}

	// 7. 执行数据库迁移（使用 goose 版本化迁移）
	sqlDB, err := testDB.DB()
	if err != nil {
		ic.Error(c, constants.ErrCodeInternal, "获取数据库连接失败: "+err.Error())
		return
	}

	// 执行 goose 迁移，使用用户选择的数据库驱动分派到对应方言 SQL 目录。
	// 传入 gormDB 让 runner 能识别"遗留 MySQL schema（AutoMigrate 起家、goose 无记录）"并 baseline。
	dialect := req.Database.Driver
	if dialect == "" {
		dialect = "mysql"
	}
	if err := migrations.RunGooseMigrationsWithGorm(sqlDB, testDB, dialect, nil); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "数据库迁移失败: "+err.Error())
		return
	}

	// 8. 创建初始数据（部门、角色、权限、管理员）
	if err := newInstallService.CreateInitialData(req.Admin); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "创建初始数据失败: "+err.Error())
		return
	}

	// 9. 标记系统为已安装
	if err := newInstallService.MarkAsInstalled(); err != nil {
		ic.Error(c, constants.ErrCodeInternal, "标记系统已安装失败: "+err.Error())
		return
	}

	// 10. 返回成功响应
	response := dto.InstallSubmitResponse{
		Success: true,
		Message: "系统安装成功，服务将自动重启以应用新配置，请稍后使用管理员账户登录",
	}

	ic.SuccessWithMessage(c, "系统安装成功", response)

	// 11. 异步触发配置重载（在响应返回后）
	go func() {
		time.Sleep(500 * time.Millisecond) // 短暂延迟，确保响应已发送
		reload.TriggerReload()
		helper.GetLogger().Info("安装完成，已触发配置重载")
	}()
}
