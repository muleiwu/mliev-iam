package controller

import (
	"fmt"
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// completeUserLogin 完成普通用户登录：签发个人中心 Cookie，并记录登录统计。
// 登录统计属于辅助信息，写入失败只记录日志，不阻断已经成功的认证流程。
func completeUserLogin(ctx httpInterfaces.RouterContextInterface, user *dto.UserResponse, rememberMe bool) {
	if user == nil {
		return
	}

	issueUserSessionCookie(ctx, user, rememberMe)

	if err := service.NewUserService(helper.GetDatabase()).RecordSuccessfulLogin(user.ID, ctx.ClientIP()); err != nil {
		helper.GetLogger().Error(fmt.Sprintf("记录用户登录信息失败: %v", err))
	}
}

// issueUserSessionCookie 登录成功后签发个人中心会话 Cookie（HttpOnly user_access_token）。
// 签发失败仅记录日志，绝不阻断登录主流程。
func issueUserSessionCookie(ctx httpInterfaces.RouterContextInterface, user *dto.UserResponse, rememberMe bool) {
	if user == nil {
		return
	}
	jwtAuthService, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		helper.GetLogger().Error(fmt.Sprintf("签发用户会话 Cookie 失败（JWT 配置错误）: %v", err))
		return
	}
	token, err := jwtAuthService.GenerateUserToken(user)
	if err != nil {
		helper.GetLogger().Error(fmt.Sprintf("签发用户会话 Cookie 失败（生成 Token 错误）: %v", err))
		return
	}

	maxAge := 0 // 默认会话期，随浏览器关闭失效
	if rememberMe {
		// 与 Token 有效期对齐：Cookie 活得更久只会带来无意义的 401
		maxAge = jwtAuthService.GetConfig().GetAccessTokenHours() * 3600
	}
	setUserSessionCookie(ctx, token, maxAge)
}

// clearUserSessionCookie 清除个人中心会话 Cookie（登出/切换账号时调用）
func clearUserSessionCookie(ctx httpInterfaces.RouterContextInterface) {
	setUserSessionCookie(ctx, "", -1)
}

func setUserSessionCookie(ctx httpInterfaces.RouterContextInterface, token string, maxAge int) {
	isSecure := ctx.Request().TLS != nil
	// SameSite 必须在 SetCookie 之前调用才能生效
	// HTTPS 使用 None 支持 iframe 跨域场景；HTTP 使用 Lax（开发环境兼容）
	if isSecure {
		ctx.SetSameSite(http.SameSiteNoneMode)
	} else {
		ctx.SetSameSite(http.SameSiteLaxMode)
	}
	ctx.SetCookie(middleware.UserCookieName, token, maxAge, "/", "", isSecure, true)
}
