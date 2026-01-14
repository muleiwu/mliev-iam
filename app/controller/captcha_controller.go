package controller

import (
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// CaptchaController 验证码公开接口控制器
type CaptchaController struct {
	BaseResponse
}

// GetCaptchaParams 获取指定场景的验证码前端参数
// GET /api/captcha/params?scene=login
func (c CaptchaController) GetCaptchaParams(ctx *gin.Context, helper interfaces.HelperInterface) {
	scene := ctx.Query("scene")
	if scene == "" {
		c.Error(ctx, 400, "场景参数不能为空")
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
	if !validScenes[scene] {
		c.Error(ctx, 400, "无效的场景参数")
		return
	}

	manager := captcha.NewManager(helper.GetDatabase())
	params, err := manager.GetClientParamsForScene(scene)
	if err != nil {
		c.Error(ctx, 500, err.Error())
		return
	}

	c.Success(ctx, params)
}
