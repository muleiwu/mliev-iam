package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"gorm.io/gorm"
)

type LoginSessionController struct{ BaseResponse }

func (lc LoginSessionController) ListMine(c httpInterfaces.RouterContextInterface)  { lc.list(c, false) }
func (lc LoginSessionController) ListAdmin(c httpInterfaces.RouterContextInterface) { lc.list(c, true) }
func (lc LoginSessionController) list(c httpInterfaces.RouterContextInterface, admin bool) {
	var query service.LoginSessionQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		lc.Error(c, 400, "会话查询参数错误")
		return
	}
	if !admin {
		query.UserID, _ = middleware.GetCurrentUserID(c)
	}
	result, err := service.NewLoginSessionService(helper.GetDatabase()).List(query, middleware.GetCurrentSessionID(c))
	if err != nil {
		if errors.Is(err, service.ErrInvalidSessionQuery) {
			lc.Error(c, 400, err.Error())
		} else {
			lc.Error(c, 500, "查询会话失败")
		}
		return
	}
	lc.Success(c, result)
}

func (lc LoginSessionController) RevokeMine(c httpInterfaces.RouterContextInterface) {
	lc.revokeOne(c, false)
}
func (lc LoginSessionController) RevokeAdmin(c httpInterfaces.RouterContextInterface) {
	lc.revokeOne(c, true)
}
func (lc LoginSessionController) revokeOne(c httpInterfaces.RouterContextInterface, admin bool) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	actor, _ := middleware.GetCurrentUserID(c)
	var session model.LoginSession
	query := helper.GetDatabase().Where("id = ?", c.Param("sid"))
	if !admin {
		query = query.Where("user_id = ?", actor)
	}
	if err := query.First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			lc.Error(c, 404, "会话不存在")
		} else {
			lc.Error(c, 500, "读取会话失败")
		}
		return
	}
	reason := "user_revoked"
	if admin {
		reason = "admin_revoked"
	}
	if session.ID == middleware.GetCurrentSessionID(c) && !admin {
		reason = "logout"
	}
	if err := service.NewLoginSessionService(helper.GetDatabase()).Revoke(session.UserID, session.ID, "", reason, &actor); err != nil {
		lc.Error(c, 500, "退出会话失败，请重试")
		return
	}
	ClearLoginSessionCookies(c, session.ID)
	lc.Success(c, map[string]any{"currentRevoked": session.ID == middleware.GetCurrentSessionID(c)})
}

func (lc LoginSessionController) RevokeOthers(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	userID, _ := middleware.GetCurrentUserID(c)
	sid := middleware.GetCurrentSessionID(c)
	if sid == "" {
		lc.Error(c, 401, "请重新登录")
		return
	}
	if err := service.NewLoginSessionService(helper.GetDatabase()).Revoke(userID, "", sid, "user_revoked", &userID); err != nil {
		lc.Error(c, 500, "退出其他会话失败")
		return
	}
	lc.Success(c, nil)
}

func (lc LoginSessionController) RevokeAllAdmin(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		lc.Error(c, 400, "用户 ID 无效")
		return
	}
	actor, _ := middleware.GetCurrentUserID(c)
	if err := service.NewLoginSessionService(helper.GetDatabase()).Revoke(uint(id), "", "", "admin_revoked", &actor); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			lc.Error(c, 404, "用户不存在")
		} else {
			lc.Error(c, 500, "清退用户会话失败")
		}
		return
	}
	currentRevoked := actor == uint(id)
	if currentRevoked {
		ClearLoginSessionCookies(c, middleware.GetCurrentSessionID(c))
	}
	lc.Success(c, map[string]any{"currentRevoked": currentRevoked})
}

// Clear only credentials belonging to the target login; another account's
// administrator cookie in the same browser must not be cleared accidentally.
func ClearLoginSessionCookies(c httpInterfaces.RouterContextInterface, sid string) {
	if sid == "" {
		return
	}
	jwtService, err := service.NewJWTAuthServiceWithError()
	if err == nil {
		for _, name := range []string{middleware.UserCookieName, middleware.AdminCookieName} {
			if raw, cookieErr := c.Cookie(name); cookieErr == nil {
				if claims, parseErr := jwtService.GetJWTService().LogoutClaims(raw); parseErr == nil && claims.SessionID == sid {
					clearLoginCookie(c, name)
				}
			}
		}
	}
	if raw, err := c.Cookie("refresh_token"); err == nil && raw != "" {
		if token, lookupErr := dao.NewOIDCTokenDAO(helper.GetDatabase()).GetByRefreshToken(raw); lookupErr == nil && token.LoginSessionID == sid {
			clearLoginCookie(c, "refresh_token")
		}
	}
}

func clearLoginCookie(c httpInterfaces.RouterContextInterface, name string) {
	secure := c.Request().TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, "", -1, "/", "", secure, true)
}

// LogoutBrowserSession accepts a signed expired/revoked login only for ending
// that login. It does not grant access to any protected operation.
func LogoutBrowserSession(c httpInterfaces.RouterContextInterface, adminFirst bool) error {
	var sid string
	var userID uint
	names := []string{middleware.UserCookieName, middleware.AdminCookieName}
	if adminFirst {
		names = []string{middleware.AdminCookieName}
	}
	jwtService, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		return err
	}
	if header := c.GetHeader("Authorization"); header != "" {
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return dao.ErrLoginSessionInvalid
		}
		claims, err := jwtService.GetJWTService().LogoutClaims(parts[1])
		if err != nil {
			return err
		}
		sid, userID = claims.SessionID, claims.UserID
	} else {
		var staleSID string
		var staleUserID uint
		for _, name := range names {
			if raw, err := c.Cookie(name); err == nil {
				if claims, err := jwtService.GetJWTService().LogoutClaims(raw); err == nil && claims.SessionID != "" {
					if staleSID == "" {
						staleSID, staleUserID = claims.SessionID, claims.UserID
					}
					if _, err := middleware.ValidateUserSessionToken(raw); err == nil {
						sid, userID = claims.SessionID, claims.UserID
						break
					}
				}
			}
		}
		if sid == "" {
			sid, userID = staleSID, staleUserID
		}
		if sid == "" && !adminFirst {
			if raw, err := c.Cookie("refresh_token"); err == nil && raw != "" {
				if token, err := dao.NewOIDCTokenDAO(helper.GetDatabase()).GetByRefreshToken(raw); err == nil {
					sid, userID = token.LoginSessionID, token.UserID
				}
			}
		}
	}
	if sid == "" {
		for _, name := range names {
			clearLoginCookie(c, name)
		}
		if !adminFirst {
			clearLoginCookie(c, "refresh_token")
		}
		return nil
	}
	if err := service.NewLoginSessionService(helper.GetDatabase()).Revoke(userID, sid, "", "logout", &userID); err != nil {
		return err
	}
	ClearLoginSessionCookies(c, sid)
	return nil
}
