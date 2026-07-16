package controller

import (
	"errors"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// AgreementController 公开协议控制器。
type AgreementController struct {
	BaseResponse
}

// GetGlobalAgreement 获取公开全局协议。
// GET /api/agreements/global/:code
func (c AgreementController) GetGlobalAgreement(ctx httpInterfaces.RouterContextInterface) {
	code := ctx.Param("code")
	agreementService := service.NewAgreementService(helper.GetDatabase())
	resp, err := agreementService.GetPublicGlobalAgreement(code)
	if err != nil {
		c.writePublicAgreementError(ctx, err)
		return
	}
	c.Success(ctx, resp)
}

// GetApplicationAgreement 获取公开应用协议。
// GET /api/agreements/application?client_id=xxx&code=xxx
func (c AgreementController) GetApplicationAgreement(ctx httpInterfaces.RouterContextInterface) {
	clientID := ctx.Query("client_id")
	code := ctx.Query("code")
	agreementService := service.NewAgreementService(helper.GetDatabase())
	resp, err := agreementService.GetPublicApplicationAgreement(clientID, code)
	if err != nil {
		c.writePublicAgreementError(ctx, err)
		return
	}
	c.Success(ctx, resp)
}

func (c AgreementController) writePublicAgreementError(ctx httpInterfaces.RouterContextInterface, err error) {
	switch {
	case errors.Is(err, service.ErrAgreementNotFound), errors.Is(err, service.ErrAgreementDisabled), errors.Is(err, service.ErrApplicationNotFound):
		c.Error(ctx, constants.ErrCodeNotFound, "协议不存在或未启用")
	case errors.Is(err, service.ErrApplicationDisabled):
		c.Error(ctx, constants.ErrCodeForbidden, err.Error())
	default:
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
	}
}
