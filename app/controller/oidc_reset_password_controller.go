package controller

import (
	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// 找回密码（忘记密码）接口。
// 验证码 sessionID 约定 reset-password:{receiver}，
// 与登录（login:...）、注册（register:...）、改绑（change-email:{userID}）场景隔离。

// SendResetPasswordCodeRequest 发送找回密码验证码请求（无需登录认证）
type SendResetPasswordCodeRequest struct {
	Type          string             `json:"type" binding:"required,oneof=email phone"`
	Receiver      string             `json:"receiver" binding:"required"`
	CaptchaTicket *dto.CaptchaTicket `json:"captchaTicket,omitempty"` // 验证码凭证（可选）
}

// ResetPasswordByCodeRequest 验证码重置密码请求（无需登录认证）
type ResetPasswordByCodeRequest struct {
	Type          string             `json:"type" binding:"required,oneof=email phone"`
	Receiver      string             `json:"receiver" binding:"required"`
	Code          string             `json:"code" binding:"required,len=6"`
	NewPassword   string             `json:"newPassword" binding:"required,min=6"`
	CaptchaTicket *dto.CaptchaTicket `json:"captchaTicket,omitempty"` // 验证码凭证（可选）
}

func resetPasswordVerificationSessionID(receiver string) string {
	return "reset-password:" + receiver
}

// SendResetPasswordCode 发送找回密码验证码
// POST /api/oidc/send-reset-password-code
func (c OIDCAuthFlowController) SendResetPasswordCode(ctx httpInterfaces.RouterContextInterface) {
	traceID := newVerificationCodeTraceID()
	var req SendResetPasswordCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.logVerificationCodeControllerError(traceID, "找回密码验证码请求参数错误 err=%v", err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID("请求参数错误: "+err.Error(), traceID))
		return
	}

	verificationType := requestVerificationType(req.Type)
	receiver, err := normalizeContactReceiver(verificationType, req.Receiver)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID(err.Error(), traceID))
		return
	}

	captchaScene := "sendSms"
	if verificationType == service.VerificationTypeEmail {
		captchaScene = "sendEmail"
	}
	c.logVerificationCodeControllerInfo(traceID, "发送找回密码验证码请求 scene=%s type=%s receiver=%s clientIp=%s", captchaScene, req.Type, service.MaskVerificationReceiver(receiver), ctx.ClientIP())
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene(captchaScene, req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.logVerificationCodeControllerWarn(traceID, "找回密码验证码校验失败 scene=%s err=%v", captchaScene, err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID(err.Error(), traceID))
		return
	}

	// 确认该联系方式绑定了可登录且已验证的账号
	userService := service.NewUserService(helper.GetDatabase())
	if _, err := userService.GetVerifiedUserForPasswordReset(verificationType, receiver); err != nil {
		c.logVerificationCodeControllerWarn(traceID, "找回密码账号校验失败 receiver=%s err=%v", service.MaskVerificationReceiver(receiver), err)
		c.Error(ctx, constants.ErrCodeBadRequest, withTraceID(err.Error(), traceID))
		return
	}

	config := getVerificationCodeConfig()
	if config == nil {
		c.logVerificationCodeControllerError(traceID, "找回密码验证码服务未配置")
		c.Error(ctx, constants.ErrCodeInternal, withTraceID("验证码服务未配置", traceID))
		return
	}

	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)
	sessionID := resetPasswordVerificationSessionID(receiver)
	if err := verificationService.SendCode(ctx.Request().Context(), receiver, verificationType, sessionID, traceID); err != nil {
		c.logVerificationCodeControllerError(traceID, "找回密码验证码发送失败 type=%s receiver=%s err=%v", verificationType, service.MaskVerificationReceiver(receiver), err)
		c.Error(ctx, constants.ErrCodeInternal, withTraceID(err.Error(), traceID))
		return
	}

	c.Success(ctx, SendVerificationCodeResponse{
		Success: true,
		Message: "验证码已发送",
		TraceID: traceID,
	})
}

// ResetPasswordByCode 校验验证码并重置密码
// POST /api/oidc/reset-password
func (c OIDCAuthFlowController) ResetPasswordByCode(ctx httpInterfaces.RouterContextInterface) {
	var req ResetPasswordByCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	verificationType := requestVerificationType(req.Type)
	receiver, err := normalizeContactReceiver(verificationType, req.Receiver)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// resetPassword 场景 captcha（未配置该场景时自动放行），限制暴力尝试验证码
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene("resetPassword", req.CaptchaTicket, ctx.ClientIP(), helper.GetRedis()); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	user, err := userService.GetVerifiedUserForPasswordReset(verificationType, receiver)
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	config := getVerificationCodeConfig()
	if config == nil {
		c.Error(ctx, constants.ErrCodeInternal, "验证码服务未配置")
		return
	}

	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)
	sessionID := resetPasswordVerificationSessionID(receiver)
	if err := verificationService.VerifyCode(ctx.Request().Context(), receiver, verificationType, sessionID, req.Code); err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, err.Error())
		return
	}

	if err := userService.ResetPassword(user.ID, req.NewPassword); err != nil {
		c.Error(ctx, constants.ErrCodeInternal, "重置密码失败: "+err.Error())
		return
	}

	c.Success(ctx, map[string]any{
		"success": true,
		"message": "密码已重置，请使用新密码登录",
	})
}
