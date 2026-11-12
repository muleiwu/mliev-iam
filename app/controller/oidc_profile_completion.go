package controller

import (
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// CompleteProfile validates contact ownership and rotates this browser's login
// without treating profile completion as a new authentication interaction.
func (c OIDCAuthFlowController) CompleteProfile(ctx httpInterfaces.RouterContextInterface) {
	var req CompleteProfileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Error(ctx, 400, "请求参数错误")
		return
	}
	if !req.SessionRotationSupported {
		c.Error(ctx, 400, "登录页面已更新，请刷新页面后重新提交")
		return
	}
	sessions := service.NewOIDCSessionService(helper.GetRedis())
	session, err := getBoundOIDCSession(ctx, sessions, req.SessionID)
	if err != nil {
		// Concurrent losers must not clear the winning request's credentials.
		c.Error(ctx, 409, "登录状态已变化，请刷新页面或重新发起登录")
		return
	}
	if !session.IsAuthenticated || session.UserID == nil {
		c.Error(ctx, 409, "请先完成登录")
		return
	}
	app, failure, err := checkOIDCUserAccess(session, *session.UserID)
	if err != nil || failure != nil {
		c.Error(ctx, 403, "当前应用授权已失效，请重新发起登录")
		return
	}
	input := service.ProfileCompletionInput{Email: req.Email, EmailCode: req.EmailCode, Phone: req.Phone, PhoneCode: req.PhoneCode, Issuer: utils.GetBaseURL(ctx, "/oidc/"+app.UUID)}
	if raw, err := ctx.Cookie(middleware.UserCookieName); err == nil {
		if claims, err := middleware.ValidateUserSessionToken(raw); err == nil && claims.SessionID == session.LoginSessionID && claims.UserID == *session.UserID && claims.ExpiresAt != nil {
			input.UserTokenDeadline = claims.ExpiresAt.Time
		}
	}
	if raw, err := ctx.Cookie("refresh_token"); err == nil {
		if old, err := dao.NewOIDCTokenDAO(helper.GetDatabase()).GetValidByRefreshToken(raw); err == nil && old.LoginSessionID == session.LoginSessionID && old.UserID == *session.UserID {
			if old.RefreshExpiresAt != nil {
				input.RefreshDeadline = *old.RefreshExpiresAt
			}
			input.RefreshCookieDeadline = old.CreatedAt.Add(7 * 24 * time.Hour)
		}
	}
	result, err := service.NewOIDCProfileCompletionService(helper.GetDatabase(), helper.GetRedis()).Complete(ctx.Request().Context(), session, input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProfileCompletionRelogin):
			ClearLoginSessionCookies(ctx, session.LoginSessionID)
			failure, _ := newOIDCFailure(session, "login_required", service.ErrProfileCompletionRelogin.Error())
			c.Success(ctx, CompleteProfileResponse{Success: false, Message: service.ErrProfileCompletionRelogin.Error(), Error: failure})
		case errors.Is(err, service.ErrProfileCompletionConflict):
			c.Error(ctx, 409, err.Error())
		case errors.Is(err, service.ErrProfileCompletionInvalid), errors.Is(err, service.ErrVerificationCodesInvalid):
			c.Error(ctx, 400, err.Error())
		case errors.Is(err, constants.ErrAccessDenied), errors.Is(err, constants.ErrInvalidScope):
			c.Error(ctx, 403, "当前应用授权已失效，请重新发起登录")
		default:
			helper.GetLogger().Error(fmt.Sprintf("[oidc-profile] stage=request session=%s err=%v", req.SessionID, err))
			c.Error(ctx, 500, "保存资料失败，请重新获取验证码后重试")
		}
		return
	}
	ClearLoginSessionCookies(ctx, session.LoginSessionID)
	userAge, refreshAge := 0, 0
	if result.RememberMe {
		userAge = cookieRemainingAge(result.UserTokenDeadline)
		refreshAge = cookieRemainingAge(result.RefreshCookieDeadline)
	}
	setUserSessionCookie(ctx, result.UserToken, userAge)
	// setUserSessionCookie has already applied the transport's SameSite policy.
	ctx.SetCookie("refresh_token", result.Tokens.RefreshToken, refreshAge, "/", "", ctx.Request().TLS != nil, true)
	c.Success(ctx, CompleteProfileResponse{Success: true, Message: "信息完善成功", NextURL: "/auth/#/oidc/consent?session_id=" + req.SessionID, AccessToken: result.Tokens.AccessToken, ExpiresIn: result.Tokens.ExpiresIn, RememberMe: result.RememberMe})
}

func cookieRemainingAge(deadline time.Time) int {
	age := int(time.Until(deadline).Seconds())
	if age < 1 {
		return -1
	}
	return age
}
