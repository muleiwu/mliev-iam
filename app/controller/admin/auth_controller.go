package admin

import (
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// AuthController 管理员认证控制器
type AuthController struct {
	BaseController
}

// setAdminAccessCookie 将管理员 access_token 写入 HttpOnly/Secure/SameSite Cookie
func setAdminAccessCookie(c httpInterfaces.RouterContextInterface, token string, maxAgeSeconds int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.AdminCookieName, token, maxAgeSeconds, "/", "", requestIsSecure(c), true)
}

// clearAdminAccessCookie 清除管理员 cookie（登出时）
func clearAdminAccessCookie(c httpInterfaces.RouterContextInterface) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.AdminCookieName, "", -1, "/", "", requestIsSecure(c), true)
}

// Logout 管理员退出登录
// POST /api/admin/auth/logout
func (ac AuthController) Logout(c httpInterfaces.RouterContextInterface) {
	clearAdminAccessCookie(c)
	if refreshToken, err := c.Cookie("refresh_token"); err == nil && refreshToken != "" {
		if err := dao.NewOIDCTokenDAO(helper.GetDatabase()).RevokeByRefreshToken(refreshToken); err != nil {
			helper.GetLogger().Warn("撤销统一认证刷新令牌失败: " + err.Error())
		}
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", "", -1, "/", "", requestIsSecure(c), true)
	c.SetCookie(middleware.UserCookieName, "", -1, "/", "", requestIsSecure(c), true)
	ac.SuccessWithMessage(c, "退出登录成功", nil)
}

// GetAdminProfile 获取管理员个人信息
// GET /api/admin/auth/profile
func (ac AuthController) GetAdminProfile(c httpInterfaces.RouterContextInterface) {
	// 从认证中间件获取当前用户信息
	userID, _, exists := middleware.GetCurrentUser(c)
	if !exists {
		ac.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}

	// 通过用户服务获取完整的用户信息
	userService := service.NewUserService(helper.GetDatabase())
	userResponse, err := userService.GetUserByID(userID)
	if err != nil {
		ac.Error(c, constants.ErrCodeNotFound, "用户信息不存在")
		return
	}

	// 返回用户信息（不包含敏感信息）
	response := map[string]any{
		"id":       userResponse.ID,
		"username": userResponse.Username,
		"email":    userResponse.Email,
		"role":     "administrator",
		"profile":  userResponse,
	}

	ac.SuccessWithMessage(c, "获取管理员信息成功", response)
}
