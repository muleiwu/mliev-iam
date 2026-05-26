package admin

import (
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type IdentityProviderController struct {
	controller.BaseResponse
}

func (pc IdentityProviderController) CreateProvider(c httpInterfaces.RouterContextInterface) {
	var req dto.CreateIdentityProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	providerService := service.NewIdentityProviderService(helper.GetDatabase())
	response, err := providerService.CreateProvider(&req)
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	pc.SuccessWithMessage(c, "创建身份源成功", response)
}

func (pc IdentityProviderController) GetProvider(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "无效的身份源ID")
		return
	}
	providerService := service.NewIdentityProviderService(helper.GetDatabase())
	response, err := providerService.GetProvider(id)
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	pc.Success(c, response)
}

func (pc IdentityProviderController) GetProviderList(c httpInterfaces.RouterContextInterface) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	statusStr := c.Query("status")

	var status *model.IdentityProviderStatus
	if statusStr != "" {
		statusInt, err := strconv.Atoi(statusStr)
		if err == nil {
			s := model.IdentityProviderStatus(statusInt)
			status = &s
		}
	}

	providerService := service.NewIdentityProviderService(helper.GetDatabase())
	items, total, err := providerService.GetProviderList(page, pageSize, status)
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	pc.Success(c, map[string]any{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (pc IdentityProviderController) UpdateProvider(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "无效的身份源ID")
		return
	}
	var req dto.UpdateIdentityProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	providerService := service.NewIdentityProviderService(helper.GetDatabase())
	response, err := providerService.UpdateProvider(id, &req)
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	pc.SuccessWithMessage(c, "更新身份源成功", response)
}

func (pc IdentityProviderController) DeleteProvider(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "无效的身份源ID")
		return
	}
	providerService := service.NewIdentityProviderService(helper.GetDatabase())
	if err := providerService.DeleteProvider(id); err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	pc.SuccessWithMessage(c, "删除身份源成功", nil)
}

func (pc IdentityProviderController) BatchOperation(c httpInterfaces.RouterContextInterface) {
	var req dto.IdentityProviderBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	providerService := service.NewIdentityProviderService(helper.GetDatabase())
	if err := providerService.BatchOperation(&req); err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	pc.SuccessWithMessage(c, "批量操作成功", nil)
}

func (pc IdentityProviderController) GetProviderStats(c httpInterfaces.RouterContextInterface) {
	providerService := service.NewIdentityProviderService(helper.GetDatabase())
	stats, err := providerService.GetProviderStats()
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	pc.Success(c, stats)
}

func parseUintParam(c httpInterfaces.RouterContextInterface, name string) (uint, error) {
	idStr := c.Param(name)
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
