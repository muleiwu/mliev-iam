package controller

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// CaptchaController 验证码公开接口控制器
type CaptchaController struct {
	BaseResponse
}

// GetCaptchaParams 获取指定场景的验证码前端参数
// POST /api/captcha/params
func (c CaptchaController) GetCaptchaParams(ctx httpInterfaces.RouterContextInterface) {
	ctx.SetHeader("Cache-Control", "no-store")
	var req dto.CaptchaParamsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误: "+err.Error())
		return
	}

	// 验证场景是否合法
	validScenes := map[string]bool{
		"login":         true,
		"register":      true,
		"resetPassword": true,
		"sendSms":       true,
		"sendEmail":     true,
	}
	if !validScenes[req.Scene] {
		c.Error(ctx, 400, "无效的场景参数")
		return
	}

	// 获取客户端IP
	clientIP := ctx.ClientIP()

	// 先确认场景是否启用验证码：未启用时不应创建 Redis 会话，
	// 否则任何人可无成本刷出大量无主会话键。
	manager := captcha.NewManager(helper.GetDatabase())
	params, err := manager.GetClientParamsForScene(req.Scene)
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}
	if !params.Enabled {
		c.Success(ctx, params)
		return
	}

	// 创建一次性会话
	sessionService := captcha.NewCaptchaSessionService(helper.GetRedis())
	sessionUUID, err := sessionService.CreateSession(&req, clientIP)
	if err != nil {
		c.Error(ctx, 500, "创建验证码会话失败: "+err.Error())
		return
	}
	params.SessionUuid = sessionUUID

	c.Success(ctx, params)

}
