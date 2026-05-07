package admin

import (
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"gorm.io/datatypes"
)

// CaptchaConfigController 验证码配置管理控制器
type CaptchaConfigController struct {
	controller.BaseResponse
}

// GetCaptchaConfigs 获取验证码配置列表
// GET /api/admin/captcha-configs
func (c CaptchaConfigController) GetCaptchaConfigs(ctx httpInterfaces.RouterContextInterface) {
	configDao := dao.NewCaptchaConfigDao(helper.GetDatabase())

	configs, err := configDao.GetAll()
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	// 转换为响应格式
	responses := make([]dto.CaptchaConfigResponse, len(configs))
	for i, config := range configs {
		responses[i] = c.convertToResponse(&config)
	}

	c.Success(ctx, responses)
}

// GetCaptchaConfig 获取单个验证码配置
// GET /api/admin/captcha-configs/:id
func (c CaptchaConfigController) GetCaptchaConfig(ctx httpInterfaces.RouterContextInterface) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.Error(ctx, 400, "无效的配置ID")
		return
	}

	configDao := dao.NewCaptchaConfigDao(helper.GetDatabase())
	config, err := configDao.GetByID(uint(id))
	if err != nil {
		c.Error(ctx, 404, "配置不存在")
		return
	}

	c.Success(ctx, c.convertToResponse(config))
}

// CreateCaptchaConfig 创建验证码配置
// POST /api/admin/captcha-configs
func (c CaptchaConfigController) CreateCaptchaConfig(ctx httpInterfaces.RouterContextInterface) {
	var req dto.CreateCaptchaConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误: "+err.Error())
		return
	}

	configDao := dao.NewCaptchaConfigDao(helper.GetDatabase())

	// 检查名称是否重复
	exists, err := configDao.ExistsByName(req.Name, 0)
	if err != nil {
		c.Error(ctx, 500, "检查配置名称失败")
		return
	}
	if exists {
		c.Error(ctx, 400, "配置名称已存在")
		return
	}

	// 转换参数为 JSONMap
	paramsMap := make(datatypes.JSONMap)
	for k, v := range req.Params {
		paramsMap[k] = v
	}

	config := &model.CaptchaConfig{
		Name:     req.Name,
		Provider: req.Provider,
		Enabled:  req.Enabled,
		Params:   paramsMap,
		Remark:   req.Remark,
	}

	if err := configDao.Create(config); err != nil {
		c.Error(ctx, 500, "创建配置失败: "+err.Error())
		return
	}

	c.Success(ctx, c.convertToResponse(config))
}

// UpdateCaptchaConfig 更新验证码配置
// PUT /api/admin/captcha-configs/:id
func (c CaptchaConfigController) UpdateCaptchaConfig(ctx httpInterfaces.RouterContextInterface) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.Error(ctx, 400, "无效的配置ID")
		return
	}

	var req dto.UpdateCaptchaConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误: "+err.Error())
		return
	}

	configDao := dao.NewCaptchaConfigDao(helper.GetDatabase())

	// 检查配置是否存在
	config, err := configDao.GetByID(uint(id))
	if err != nil {
		c.Error(ctx, 404, "配置不存在")
		return
	}

	// 检查名称是否重复
	exists, err := configDao.ExistsByName(req.Name, uint(id))
	if err != nil {
		c.Error(ctx, 500, "检查配置名称失败")
		return
	}
	if exists {
		c.Error(ctx, 400, "配置名称已存在")
		return
	}

	// 转换参数为 JSONMap
	paramsMap := make(datatypes.JSONMap)
	for k, v := range req.Params {
		paramsMap[k] = v
	}

	// 更新配置
	config.Name = req.Name
	config.Provider = req.Provider
	config.Enabled = req.Enabled
	config.Params = paramsMap
	config.Remark = req.Remark

	if err := configDao.Update(config); err != nil {
		c.Error(ctx, 500, "更新配置失败: "+err.Error())
		return
	}

	c.Success(ctx, c.convertToResponse(config))
}

// DeleteCaptchaConfig 删除验证码配置
// DELETE /api/admin/captcha-configs/:id
func (c CaptchaConfigController) DeleteCaptchaConfig(ctx httpInterfaces.RouterContextInterface) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.Error(ctx, 400, "无效的配置ID")
		return
	}

	configDao := dao.NewCaptchaConfigDao(helper.GetDatabase())

	// 检查配置是否存在
	exists, err := configDao.Exists(uint(id))
	if err != nil {
		c.Error(ctx, 500, "检查配置失败")
		return
	}
	if !exists {
		c.Error(ctx, 404, "配置不存在")
		return
	}

	if err := configDao.Delete(uint(id)); err != nil {
		c.Error(ctx, 500, "删除配置失败: "+err.Error())
		return
	}

	c.Success(ctx, map[string]any{"message": "删除成功"})
}

// GetCaptchaProviders 获取支持的验证码提供商列表
// GET /api/admin/captcha-configs/providers
func (c CaptchaConfigController) GetCaptchaProviders(ctx httpInterfaces.RouterContextInterface) {
	manager := captcha.NewManager(helper.GetDatabase())
	providers := manager.GetSupportedProviders()

	c.Success(ctx, providers)
}

// GetCaptchaSceneBindings 获取场景绑定配置
// GET /api/admin/settings/captcha/bindings
func (c CaptchaConfigController) GetCaptchaSceneBindings(ctx httpInterfaces.RouterContextInterface) {
	settingService := service.NewSettingService(helper.GetDatabase())

	bindings, err := settingService.GetCaptchaSceneBindings()
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, bindings)
}

// UpdateCaptchaSceneBindings 更新场景绑定配置
// PUT /api/admin/settings/captcha/bindings
func (c CaptchaConfigController) UpdateCaptchaSceneBindings(ctx httpInterfaces.RouterContextInterface) {
	var req dto.UpdateCaptchaSceneBindingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误: "+err.Error())
		return
	}

	settingService := service.NewSettingService(helper.GetDatabase())

	if err := settingService.UpdateCaptchaSceneBindings(&req); err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, map[string]any{"message": "更新成功"})
}

// convertToResponse 转换为响应格式
func (c CaptchaConfigController) convertToResponse(config *model.CaptchaConfig) dto.CaptchaConfigResponse {
	// 将 JSONMap 转换为 map[string]string
	params := make(map[string]string)
	if config.Params != nil {
		for k, v := range config.Params {
			if str, ok := v.(string); ok {
				params[k] = str
			}
		}
	}

	return dto.CaptchaConfigResponse{
		ID:        config.ID,
		Name:      config.Name,
		Provider:  config.Provider,
		Enabled:   config.Enabled,
		Params:    params,
		Remark:    config.Remark,
		CreatedAt: config.CreatedAt,
		UpdatedAt: config.UpdatedAt,
	}
}
