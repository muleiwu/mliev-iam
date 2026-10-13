package admin

import (
	"errors"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type PushController struct {
	controller.BaseResponse
}

func respondPushError(ctx httpInterfaces.RouterContextInterface, err error) {
	status := 500
	var pushError *service.PushError
	if errors.As(err, &pushError) {
		status = pushError.Status
	}
	controller.BaseResponse{}.Error(ctx, status, err.Error())
}

func (c PushController) ListChannels(ctx httpInterfaces.RouterContextInterface) {
	var req dto.PushChannelsQueryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误")
		return
	}
	push, err := service.NewPushService(req.PushCredentials)
	if err != nil {
		respondPushError(ctx, err)
		return
	}
	page, err := push.ListChannels(ctx.Request().Context(), req)
	if err != nil {
		respondPushError(ctx, err)
		return
	}
	c.Success(ctx, page)
}

func (c PushController) GetChannel(ctx httpInterfaces.RouterContextInterface) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		c.Error(ctx, 400, "通道 ID 须为正数")
		return
	}
	var req dto.PushCredentials
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误")
		return
	}
	push, err := service.NewPushService(req)
	if err != nil {
		respondPushError(ctx, err)
		return
	}
	detail, err := push.GetChannel(ctx.Request().Context(), id)
	if err != nil {
		respondPushError(ctx, err)
		return
	}
	c.Success(ctx, detail)
}

func (c PushController) Test(ctx httpInterfaces.RouterContextInterface) {
	var req dto.PushTestRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误")
		return
	}
	push, err := service.NewPushService(req.Config.Credentials())
	if err != nil {
		respondPushError(ctx, err)
		return
	}
	result, err := push.Test(ctx.Request().Context(), req)
	if err != nil {
		respondPushError(ctx, err)
		return
	}
	c.SuccessWithMessage(ctx, "测试消息已提交", result)
}
