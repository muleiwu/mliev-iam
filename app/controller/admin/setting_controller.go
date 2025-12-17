package admin

import (
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// SettingController 设置管理控制器
type SettingController struct {
	controller.BaseResponse
}

// GetAllSettings 获取所有设置
// GET /api/admin/settings
func (c SettingController) GetAllSettings(ctx *gin.Context, helper interfaces.HelperInterface) {
	settingService := service.NewSettingService(helper.GetDatabase())

	settings, err := settingService.GetAll()
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, settings)
}

// GetSettingByKey 根据key获取设置
// GET /api/admin/settings/:key
func (c SettingController) GetSettingByKey(ctx *gin.Context, helper interfaces.HelperInterface) {
	key := ctx.Param("key")
	if key == "" {
		c.Error(ctx, 400, "设置key不能为空")
		return
	}

	settingService := service.NewSettingService(helper.GetDatabase())

	setting, err := settingService.GetByKey(key)
	if err != nil {
		c.Error(ctx, 404, err.Error())
		return
	}

	c.Success(ctx, setting)
}

// CreateSetting 创建设置
// POST /api/admin/settings
func (c SettingController) CreateSetting(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req dto.CreateSettingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误: "+err.Error())
		return
	}

	settingService := service.NewSettingService(helper.GetDatabase())

	setting, err := settingService.Create(&req)
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, setting)
}

// UpdateSetting 更新设置
// PUT /api/admin/settings/:key
func (c SettingController) UpdateSetting(ctx *gin.Context, helper interfaces.HelperInterface) {
	key := ctx.Param("key")
	if key == "" {
		c.Error(ctx, 400, "设置key不能为空")
		return
	}

	var req dto.UpdateSettingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误: "+err.Error())
		return
	}

	settingService := service.NewSettingService(helper.GetDatabase())

	setting, err := settingService.Update(key, &req)
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, setting)
}

// DeleteSetting 删除设置
// DELETE /api/admin/settings/:key
func (c SettingController) DeleteSetting(ctx *gin.Context, helper interfaces.HelperInterface) {
	key := ctx.Param("key")
	if key == "" {
		c.Error(ctx, 400, "设置key不能为空")
		return
	}

	settingService := service.NewSettingService(helper.GetDatabase())

	if err := settingService.Delete(key); err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, gin.H{"message": "删除成功"})
}

// GetRegisterFieldsSettings 获取注册字段配置
// GET /api/admin/settings/register-fields
func (c SettingController) GetRegisterFieldsSettings(ctx *gin.Context, helper interfaces.HelperInterface) {
	settingService := service.NewSettingService(helper.GetDatabase())

	config, err := settingService.GetRegisterFieldsSettings()
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, config)
}

// UpdateRegisterFieldsSettings 更新注册字段配置
// PUT /api/admin/settings/register-fields
func (c SettingController) UpdateRegisterFieldsSettings(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req dto.UpdateRegisterFieldsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误: "+err.Error())
		return
	}

	settingService := service.NewSettingService(helper.GetDatabase())

	if err := settingService.UpdateRegisterFieldsSettings(&req); err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, gin.H{"message": "更新成功"})
}

// GetPushSettings 获取推送服务配置
// GET /api/admin/settings/push/config
func (c SettingController) GetPushSettings(ctx *gin.Context, helper interfaces.HelperInterface) {
	settingService := service.NewSettingService(helper.GetDatabase())

	config, err := settingService.GetPushSettings()
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, config)
}

// UpdatePushSettings 更新推送服务配置
// PUT /api/admin/settings/push/config
func (c SettingController) UpdatePushSettings(ctx *gin.Context, helper interfaces.HelperInterface) {
	var req dto.UpdatePushSettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误: "+err.Error())
		return
	}

	settingService := service.NewSettingService(helper.GetDatabase())

	if err := settingService.UpdatePushSettings(&req); err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, gin.H{"message": "更新成功"})
}
