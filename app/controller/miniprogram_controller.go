package controller

import (
	"errors"
	"io"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type MiniProgramController struct{ BaseResponse }

func (mc MiniProgramController) Handle(c httpInterfaces.RouterContextInterface) {
	c.SetHeader("Cache-Control", "no-store")
	c.SetHeader("Pragma", "no-cache")
	clientID := c.Param("client_id")
	token := ""
	if parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2); len(parts) == 2 && parts[0] == "Bearer" {
		token = parts[1]
	}
	svc := service.NewMiniProgramService(helper.GetDatabase(), helper.GetRedis(), twoFactorService())
	svc.SetCaptcha(captcha.NewMiniProgramManager(helper.GetDatabase(), helper.GetRedis()))
	fail := func(err error) {
		status := 400
		if errors.Is(err, service.ErrMiniRateLimit) {
			status = 429
		}
		if errors.Is(err, service.ErrMiniTransaction) {
			status = 401
		}
		mc.Error(c, status, err.Error())
	}
	if err := svc.RateLimit(c.Request().Context(), clientID, c.ClientIP(), token); err != nil {
		fail(err)
		return
	}
	action := strings.TrimPrefix(c.Request().URL.Path, "/api/auth/miniprogram/"+clientID+"/")
	if action == "config" {
		config, err := svc.Config(clientID)
		if err != nil {
			fail(err)
			return
		}
		mc.Success(c, config)
		return
	}
	bind := func(v any) bool {
		if c.Request().ContentLength > 65536 {
			mc.Error(c, 413, "请求体过大")
			return false
		}
		c.Request().Body = io.NopCloser(io.LimitReader(c.Request().Body, 65536))
		if err := c.ShouldBindJSON(v); err != nil {
			mc.Error(c, 400, "请求参数无效")
			return false
		}
		return true
	}
	var response *dto.MiniProgramResponse
	var err error
	ctx := c.Request().Context()
	switch action {
	case "login":
		var req dto.MiniProgramLoginRequest
		if !bind(&req) {
			return
		}
		response, err = svc.Login(ctx, clientID, req, utils.GetBaseURL(c, "/oidc/"+clientID))
	case "phone":
		var req dto.MiniProgramPhoneRequest
		if !bind(&req) {
			return
		}
		response, err = svc.Phone(ctx, clientID, token, req.PhoneCode)
	case "bind":
		var req dto.MiniProgramBindRequest
		if !bind(&req) {
			return
		}
		response, err = svc.Bind(ctx, clientID, token, req.Username, req.Password, c.ClientIP(), req.CaptchaTicket)
	case "captcha/params":
		var req dto.MiniProgramCaptchaParamsRequest
		if !bind(&req) {
			return
		}
		params, paramsErr := svc.CaptchaParams(ctx, clientID, token, req, c.ClientIP())
		if paramsErr != nil {
			fail(paramsErr)
			return
		}
		mc.Success(c, params)
		return

	case "complete":
		var req dto.MiniProgramCompleteRequest
		if !bind(&req) {
			return
		}
		response, err = svc.Complete(ctx, clientID, token, req, c.ClientIP(), c.Request().UserAgent())
	case "two-factor/send", "two-factor/verify":
		var req dto.MiniProgramFactorRequest
		if !bind(&req) {
			return
		}
		response, err = svc.Factor(ctx, clientID, token, req, action == "two-factor/send", c.ClientIP(), c.Request().UserAgent())
	default:
		mc.Error(c, 404, "接口不存在")
		return
	}
	if err != nil {
		fail(err)
		return
	}
	mc.Success(c, response)
}
