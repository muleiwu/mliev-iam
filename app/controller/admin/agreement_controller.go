package admin

import (
	"errors"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// AgreementController 管理端协议控制器。
type AgreementController struct {
	controller.BaseResponse
}

// ListGlobalAgreements 获取全局协议。
// GET /api/admin/agreements/global
func (c AgreementController) ListGlobalAgreements(ctx httpInterfaces.RouterContextInterface) {
	agreementService := service.NewAgreementService(helper.GetDatabase())
	resp, err := agreementService.ListGlobalAgreements(true)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}
	c.Success(ctx, resp)
}

// GetGlobalAgreement 获取单个全局协议。
// GET /api/admin/agreements/global/:code
func (c AgreementController) GetGlobalAgreement(ctx httpInterfaces.RouterContextInterface) {
	agreementService := service.NewAgreementService(helper.GetDatabase())
	resp, err := agreementService.GetGlobalAgreement(ctx.Param("code"))
	if err != nil {
		c.writeAdminAgreementError(ctx, err)
		return
	}
	c.Success(ctx, resp)
}

// UpsertGlobalAgreement 更新全局协议。
// PUT /api/admin/agreements/global/:code
func (c AgreementController) UpsertGlobalAgreement(ctx httpInterfaces.RouterContextInterface) {
	var req dto.AgreementUpsertRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	agreementService := service.NewAgreementService(helper.GetDatabase())
	resp, err := agreementService.UpsertGlobalAgreement(ctx.Param("code"), &req)
	if err != nil {
		c.writeAdminAgreementError(ctx, err)
		return
	}
	c.SuccessWithMessage(ctx, "协议已保存", resp)
}

// ListApplicationAgreements 获取应用协议。
// GET /api/admin/applications/:id/agreements
func (c AgreementController) ListApplicationAgreements(ctx httpInterfaces.RouterContextInterface) {
	appID, ok := c.parseUintParam(ctx, "id")
	if !ok {
		return
	}
	agreementService := service.NewAgreementService(helper.GetDatabase())
	resp, err := agreementService.ListApplicationAgreements(appID, true)
	if err != nil {
		c.writeAdminAgreementError(ctx, err)
		return
	}
	c.Success(ctx, resp)
}

// CreateApplicationAgreement 创建应用协议。
// POST /api/admin/applications/:id/agreements
func (c AgreementController) CreateApplicationAgreement(ctx httpInterfaces.RouterContextInterface) {
	appID, ok := c.parseUintParam(ctx, "id")
	if !ok {
		return
	}
	var req dto.AgreementUpsertRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	agreementService := service.NewAgreementService(helper.GetDatabase())
	resp, err := agreementService.CreateApplicationAgreement(appID, &req)
	if err != nil {
		c.writeAdminAgreementError(ctx, err)
		return
	}
	c.SuccessWithMessage(ctx, "协议已创建", resp)
}

// UpdateApplicationAgreement 更新应用协议。
// PUT /api/admin/applications/:id/agreements/:agreementId
func (c AgreementController) UpdateApplicationAgreement(ctx httpInterfaces.RouterContextInterface) {
	appID, ok := c.parseUintParam(ctx, "id")
	if !ok {
		return
	}
	agreementID, ok := c.parseUintParam(ctx, "agreementId")
	if !ok {
		return
	}
	var req dto.AgreementUpsertRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	agreementService := service.NewAgreementService(helper.GetDatabase())
	resp, err := agreementService.UpdateApplicationAgreement(appID, agreementID, &req)
	if err != nil {
		c.writeAdminAgreementError(ctx, err)
		return
	}
	c.SuccessWithMessage(ctx, "协议已保存", resp)
}

// DeleteApplicationAgreement 删除应用协议。
// DELETE /api/admin/applications/:id/agreements/:agreementId
func (c AgreementController) DeleteApplicationAgreement(ctx httpInterfaces.RouterContextInterface) {
	appID, ok := c.parseUintParam(ctx, "id")
	if !ok {
		return
	}
	agreementID, ok := c.parseUintParam(ctx, "agreementId")
	if !ok {
		return
	}
	agreementService := service.NewAgreementService(helper.GetDatabase())
	if err := agreementService.DeleteApplicationAgreement(appID, agreementID); err != nil {
		c.writeAdminAgreementError(ctx, err)
		return
	}
	c.Success(ctx, map[string]any{"message": "删除成功"})
}

func (c AgreementController) parseUintParam(ctx httpInterfaces.RouterContextInterface, name string) (uint, bool) {
	raw := ctx.Param(name)
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "参数格式错误: "+name)
		return 0, false
	}
	return uint(value), true
}

func (c AgreementController) writeAdminAgreementError(ctx httpInterfaces.RouterContextInterface, err error) {
	switch {
	case errors.Is(err, service.ErrAgreementNotFound), errors.Is(err, service.ErrApplicationNotFound):
		c.Error(ctx, constants.ErrCodeNotFound, err.Error())
	case errors.Is(err, service.ErrAgreementDisabled):
		c.Error(ctx, constants.ErrCodeForbidden, err.Error())
	default:
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
	}
}
