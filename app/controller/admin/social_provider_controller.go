package admin

import (
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// SocialProviderController 社会化登录提供商管理控制器（管理员）
type SocialProviderController struct {
	controller.BaseResponse
}

// CreateProvider 创建提供商
// POST /api/admin/social-providers
func (pc SocialProviderController) CreateProvider(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.CreateSocialProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	providerService := service.NewSocialProviderService(helper.GetDatabase())
	response, err := providerService.CreateProvider(&req)
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.SuccessWithMessage(c, "创建提供商成功", response)
}

// GetProvider 获取提供商详情
// GET /api/admin/social-providers/:id
func (pc SocialProviderController) GetProvider(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "无效的提供商ID")
		return
	}

	providerService := service.NewSocialProviderService(helper.GetDatabase())
	response, err := providerService.GetProvider(uint(id))
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.Success(c, response)
}

// GetProviderList 获取提供商列表
// GET /api/admin/social-providers
func (pc SocialProviderController) GetProviderList(c *gin.Context, helper interfaces.HelperInterface) {
	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	statusStr := c.Query("status")

	var status *model.ProviderStatus
	if statusStr != "" {
		statusInt, err := strconv.Atoi(statusStr)
		if err == nil {
			s := model.ProviderStatus(statusInt)
			status = &s
		}
	}

	providerService := service.NewSocialProviderService(helper.GetDatabase())
	items, total, err := providerService.GetProviderList(page, pageSize, status)
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.Success(c, gin.H{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// UpdateProvider 更新提供商
// PUT /api/admin/social-providers/:id
func (pc SocialProviderController) UpdateProvider(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "无效的提供商ID")
		return
	}

	var req dto.UpdateSocialProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	providerService := service.NewSocialProviderService(helper.GetDatabase())
	response, err := providerService.UpdateProvider(uint(id), &req)
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.SuccessWithMessage(c, "更新提供商成功", response)
}

// DeleteProvider 删除提供商
// DELETE /api/admin/social-providers/:id
func (pc SocialProviderController) DeleteProvider(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "无效的提供商ID")
		return
	}

	providerService := service.NewSocialProviderService(helper.GetDatabase())
	if err := providerService.DeleteProvider(uint(id)); err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.SuccessWithMessage(c, "删除提供商成功", nil)
}

// BatchOperation 批量操作提供商
// POST /api/admin/social-providers/batch
func (pc SocialProviderController) BatchOperation(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.SocialProviderBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	providerService := service.NewSocialProviderService(helper.GetDatabase())
	if err := providerService.BatchOperation(&req); err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.SuccessWithMessage(c, "批量操作成功", nil)
}

// GetProviderStats 获取提供商统计信息
// GET /api/admin/social-providers/stats
func (pc SocialProviderController) GetProviderStats(c *gin.Context, helper interfaces.HelperInterface) {
	providerService := service.NewSocialProviderService(helper.GetDatabase())
	stats, err := providerService.GetProviderStats()
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.Success(c, stats)
}
