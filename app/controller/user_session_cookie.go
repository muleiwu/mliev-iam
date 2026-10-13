package controller

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// completeUserLogin 完成普通用户登录：签发个人中心 Cookie，并记录登录统计。
// 登录统计属于辅助信息，写入失败只记录日志，不阻断已经成功的认证流程。
func completeUserLogin(ctx httpInterfaces.RouterContextInterface, user *dto.UserResponse, rememberMe bool, oidcIDs ...string) error {
	if user == nil {
		return fmt.Errorf("用户为空")
	}
	var sid, oidcID string
	if len(oidcIDs) > 0 {
		oidcID = oidcIDs[0]
	}
	sessions := service.NewOIDCSessionService(helper.GetRedis())
	if oidcID != "" {
		transaction, err := getBoundOIDCSession(ctx, sessions, oidcID)
		if err != nil {
			return err
		}
		if transaction.UserID != nil && *transaction.UserID != user.ID {
			return fmt.Errorf("登录账号与授权事务不匹配")
		}
		sid = transaction.LoginSessionID
	}
	kind := "account"
	if strings.Contains(ctx.Request().URL.Path, "external") {
		kind = "external"
	}
	pendingBinding, _ := ctx.Get("two_factor_continuation").(string)
	if sid == "" || pendingBinding != "" {
		if err := beginTwoFactorLogin(ctx, user, kind, oidcID, rememberMe); err != nil {
			return err
		}
	}
	token, maxAge, sid, err := prepareUserSessionCookie(ctx, user, rememberMe, sid)
	if err != nil {
		return err
	}
	if oidcID != "" {
		if err := sessions.MarkAsAuthenticated(ctx.Request().Context(), oidcID, user.ID, sid); err != nil {
			_ = service.NewLoginSessionService(helper.GetDatabase()).Revoke(user.ID, sid, "", "login_failed", nil)
			return err
		}
	}
	if err := finishUserSession(ctx, token, maxAge, sid, user.ID); err != nil {
		return err
	}
	ctx.Set("login_response_token", token)
	recordSuccessfulUserLogin(ctx, user)
	return nil
}

// finishUserSession only replaces browser credentials after durable creation.
func finishUserSession(ctx httpInterfaces.RouterContextInterface, token string, maxAge int, sid string, userID uint) error {
	jwtService, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		return err
	}
	if old, err := ctx.Cookie(middleware.UserCookieName); err == nil && old != "" {
		if claims, err := jwtService.GetJWTService().LogoutClaims(old); err == nil && claims.SessionID != "" && claims.SessionID != sid {
			if err := service.NewLoginSessionService(helper.GetDatabase()).Revoke(claims.UserID, claims.SessionID, "", "replaced", &userID); err != nil {
				_ = service.NewLoginSessionService(helper.GetDatabase()).Revoke(userID, sid, "", "login_failed", nil)
				return err
			}
			ClearLoginSessionCookies(ctx, claims.SessionID)
		}
	}
	setUserSessionCookie(ctx, token, maxAge)
	return nil
}

func recordSuccessfulUserLogin(ctx httpInterfaces.RouterContextInterface, user *dto.UserResponse) {
	if user == nil {
		return
	}
	if err := service.NewUserService(helper.GetDatabase()).RecordSuccessfulLogin(user.ID, ctx.ClientIP()); err != nil {
		helper.GetLogger().Error(fmt.Sprintf("记录用户登录信息失败: %v", err))
	}
}

// prepareUserSessionCookie creates or reuses the server-validated login before signing.
func prepareUserSessionCookie(ctx httpInterfaces.RouterContextInterface, user *dto.UserResponse, rememberMe bool, sid string) (string, int, string, error) {
	if user == nil {
		return "", 0, "", fmt.Errorf("用户为空")
	}
	jwtService, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		return "", 0, "", err
	}
	if sid == "" {
		method := "password"
		path := ctx.Request().URL.Path
		switch {
		case strings.Contains(path, "qrcode"):
			method = "qrcode"
		case strings.Contains(path, "external") || strings.Contains(path, "miniprogram"):
			method = "external"
		case strings.Contains(path, "login-by-code"):
			method = "verification_code"
		case strings.Contains(path, "register"):
			method = "registration"
		}
		if primary, ok := ctx.Get("primary_auth_method").(string); ok {
			method = primary
		}
		proof, _ := ctx.Get("two_factor_proof").(*service.TwoFactorProof)
		session, createErr := service.NewLoginSessionService(helper.GetDatabase()).CreateWithProof(user.ID, method, ctx.ClientIP(), ctx.GetHeader("User-Agent"), time.Now().Add(jwtService.GetConfig().AccessTokenTTL), proof)
		if createErr != nil {
			return "", 0, "", createErr
		}
		sid = session.ID
	} else if _, err := dao.NewLoginSessionDAO(helper.GetDatabase()).Valid(sid, user.ID); err != nil {
		return "", 0, "", err
	}
	token, err := jwtService.GenerateUserToken(user, sid)
	if err != nil {
		return "", 0, sid, err
	}
	maxAge := 0
	if rememberMe {
		maxAge = jwtService.GetConfig().GetAccessTokenHours() * 3600
	}
	return token, maxAge, sid, nil
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

// EstablishExistingUserSession reuses a validated SID; it never creates/revokes
// a login session. First-party OIDC callbacks use it after consuming the code.
func EstablishExistingUserSession(ctx httpInterfaces.RouterContextInterface, user *dto.UserResponse, sid string) error {
	if user == nil || sid == "" {
		return fmt.Errorf("登录会话为空")
	}
	// The unified login has usually already set this cookie. Preserve its
	// original expiry and remember-me semantics instead of replacing it with
	// a session cookie on the first-party callback.
	if raw, err := ctx.Cookie(middleware.UserCookieName); err == nil {
		if claims, err := middleware.ValidateUserSessionToken(raw); err == nil && claims.UserID == user.ID && claims.SessionID == sid {
			return nil
		}
	}
	token, maxAge, _, err := prepareUserSessionCookie(ctx, user, false, sid)
	if err != nil {
		return err
	}
	setUserSessionCookie(ctx, token, maxAge)
	return nil
}
