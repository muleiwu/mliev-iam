package controller

import (
	"fmt"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// 个人中心邮箱/手机改绑接口。
// 验证码 sessionID 约定 change-email:{userID} / change-phone:{userID}，
// 与登录（login:...）、注册（register:...）场景隔离，防止跨场景复用验证码。

func contactVerificationSessionID(verificationType service.VerificationType, userID uint) string {
	if verificationType == service.VerificationTypeEmail {
		return fmt.Sprintf("change-email:%d", userID)
	}
	return fmt.Sprintf("change-phone:%d", userID)
}

// normalizeContactReceiver 规范化联系方式。发码、校验、落库三处必须使用同一结果，
// 否则验证码 Redis key（含 receiver）对不上。
func normalizeContactReceiver(verificationType service.VerificationType, receiver string) (string, error) {
	receiver = strings.TrimSpace(receiver)
	if verificationType == service.VerificationTypePhone {
		return utils.NormalizePhoneToE164(receiver, utils.DefaultPhoneRegion)
	}
	if receiver == "" {
		return "", fmt.Errorf("邮箱不能为空")
	}
	return receiver, nil
}

// SendContactVerificationCode 发送改绑验证码
// POST /api/user/send-contact-verification-code
func (uc UserController) SendContactVerificationCode(c httpInterfaces.RouterContextInterface) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		uc.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}
	if !uc.requireJSONContentType(c) {
		return
	}

	var req dto.SendContactVerificationCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	verificationType := requestVerificationType(req.Type)
	receiver, err := normalizeContactReceiver(verificationType, req.Receiver)
	if err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}

	captchaScene := "sendSms"
	if verificationType == service.VerificationTypeEmail {
		captchaScene = "sendEmail"
	}
	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene(captchaScene, req.CaptchaTicket, c.ClientIP(), helper.GetRedis()); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}

	// 发码前先确认新值未被占用，避免给注定失败的改绑发码
	userService := service.NewUserService(helper.GetDatabase())
	if inUse, err := userService.ContactInUse(verificationType, receiver, userID); err != nil {
		uc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	} else if inUse {
		if verificationType == service.VerificationTypeEmail {
			uc.Error(c, constants.ErrCodeEmailAlreadyExists, "邮箱已被其他账号使用")
		} else {
			uc.Error(c, constants.ErrCodePhoneAlreadyExists, "手机号已被其他账号使用")
		}
		return
	}

	config := getVerificationCodeConfig()
	if config == nil {
		uc.Error(c, constants.ErrCodeUnavailable, "验证码服务未配置")
		return
	}

	traceID := newVerificationCodeTraceID()
	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)
	sessionID := contactVerificationSessionID(verificationType, userID)
	if err := verificationService.SendCode(c.Request().Context(), receiver, verificationType, sessionID, traceID); err != nil {
		uc.Error(c, constants.ErrCodeInternal, withTraceID(err.Error(), traceID))
		return
	}

	uc.SuccessWithMessage(c, "验证码已发送", map[string]any{"traceId": traceID})
}

// ChangeEmail 改绑邮箱
// PUT /api/user/email
func (uc UserController) ChangeEmail(c httpInterfaces.RouterContextInterface) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		uc.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}
	if !uc.requireJSONContentType(c) {
		return
	}

	var req dto.ChangeEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	uc.changeContact(c, userID, service.VerificationTypeEmail, req.Email, req.Code, req.OperationToken)
}

// ChangePhone 改绑手机号
// PUT /api/user/phone
func (uc UserController) ChangePhone(c httpInterfaces.RouterContextInterface) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		uc.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}
	if !uc.requireJSONContentType(c) {
		return
	}

	var req dto.ChangePhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	uc.changeContact(c, userID, service.VerificationTypePhone, req.Phone, req.Code, req.OperationToken)
}

// changeContact 校验验证码后更新联系方式并置 verified_at。
func (uc UserController) changeContact(c httpInterfaces.RouterContextInterface, userID uint, verificationType service.VerificationType, value, code string, operationTokens ...string) {
	receiver, err := normalizeContactReceiver(verificationType, value)
	if err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}

	config := getVerificationCodeConfig()
	if config == nil {
		uc.Error(c, constants.ErrCodeUnavailable, "验证码服务未配置")
		return
	}

	verificationService := service.NewVerificationCodeService(helper.GetRedis(), config)
	sessionID := contactVerificationSessionID(verificationType, userID)
	if err := verificationService.VerifyCode(c.Request().Context(), receiver, verificationType, sessionID, code); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	user, err := userService.GetUserModelByID(userID)
	if err != nil {
		uc.Error(c, 400, "用户不可用")
		return
	}
	var proof *service.TwoFactorProof
	if user.TwoFactorEnabled {
		token := ""
		if len(operationTokens) > 0 {
			token = operationTokens[0]
		}
		proof, err = twoFactorService().ChangeContact(c.Request().Context(), userID, middleware.GetCurrentSessionID(c), token, verificationType, receiver, c.ClientIP())
	} else {
		err = userService.UpdateVerifiedContact(userID, verificationType, receiver)
	}
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()
		if strings.Contains(errMsg, "邮箱已存在") {
			errCode = constants.ErrCodeEmailAlreadyExists
		} else if strings.Contains(errMsg, "手机号已存在") {
			errCode = constants.ErrCodePhoneAlreadyExists
		}
		uc.Error(c, errCode, errMsg)
		return
	}

	if proof != nil {
		if err := renewTwoFactorSession(c, userID, proof); err != nil {
			uc.Error(c, 500, "联系方式已修改，请重新登录")
			return
		}
	}
	response, err := userService.GetUserByID(userID)
	if err != nil {
		uc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	message := "邮箱修改成功"
	if verificationType == service.VerificationTypePhone {
		message = "手机号修改成功"
	}
	uc.SuccessWithMessage(c, message, sanitizeSelfProfile(response))
}
