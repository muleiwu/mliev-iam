package controller

import (
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/captcha"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// UserController 普通用户控制器（非管理员操作）
type UserController struct {
	BaseResponse
}

// requireJSONContentType 拒绝非 JSON 请求体。
// 会话 Cookie 在 HTTPS 下为 SameSite=None，跨站表单（text/plain 等）可携带 Cookie 提交，
// 而 ShouldBindJSON 不校验 Content-Type，因此写接口必须显式限制。
func (uc UserController) requireJSONContentType(c httpInterfaces.RouterContextInterface) bool {
	contentType := c.GetHeader("Content-Type")
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "application/json") {
		uc.Error(c, constants.ErrCodeBadRequest, "Content-Type 必须为 application/json")
		return false
	}
	return true
}

// ChangePassword 修改用户密码（用户修改自己的密码）
// PUT /api/user/password
func (uc UserController) ChangePassword(c httpInterfaces.RouterContextInterface) {
	userId, ok := middleware.GetCurrentUserID(c)
	if !ok {
		uc.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}
	if !uc.requireJSONContentType(c) {
		return
	}

	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	err := userService.ChangePassword(userId, &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "用户不存在") {
			errCode = constants.ErrCodeUserNotFound
		} else if strings.Contains(errMsg, "旧密码错误") {
			errCode = constants.ErrCodeInvalidPassword
		}

		uc.Error(c, errCode, errMsg)
		return
	}

	uc.SuccessWithMessage(c, "密码修改成功", nil)
}

// ValidateLogin 个人中心独立登录（不依赖 OIDC session）。
// 校验凭证后签发 user_access_token Cookie，供 /api/user/* 接口使用。
// POST /api/user/login
func (uc UserController) ValidateLogin(c httpInterfaces.RouterContextInterface) {
	if !uc.requireJSONContentType(c) {
		return
	}

	var req dto.AccountLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	captchaManager := captcha.NewManager(helper.GetDatabase())
	if err := captchaManager.ValidateForScene("login", req.CaptchaTicket, c.ClientIP(), helper.GetRedis()); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.ValidateUserForLogin(req.Username, req.Password)
	if err != nil {
		errCode := constants.ErrCodeInvalidPassword
		errMsg := err.Error()

		if strings.Contains(errMsg, "用户账户已禁用") {
			errCode = constants.ErrCodeUserDisabled
		} else if strings.Contains(errMsg, "用户账户未激活") {
			errCode = constants.ErrCodeUserNotActivated
		}

		uc.Error(c, errCode, errMsg)
		return
	}

	issueUserSessionCookie(c, response, req.RememberMe)

	uc.SuccessWithMessage(c, "登录成功", response)
}

// GetUserProfile 获取当前用户信息
// GET /api/user/profile
func (uc UserController) GetUserProfile(c httpInterfaces.RouterContextInterface) {
	userId, ok := middleware.GetCurrentUserID(c)
	if !ok {
		uc.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUserByID(userId)
	if err != nil {
		errCode := constants.ErrCodeUserNotFound
		if !strings.Contains(err.Error(), "用户不存在") {
			errCode = constants.ErrCodeInternal
		}
		uc.Error(c, errCode, err.Error())
		return
	}

	uc.Success(c, response)
}

// UpdateUserProfile 更新当前用户信息（自助白名单字段）
// PUT /api/user/profile
func (uc UserController) UpdateUserProfile(c httpInterfaces.RouterContextInterface) {
	userId, ok := middleware.GetCurrentUserID(c)
	if !ok {
		uc.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}
	if !uc.requireJSONContentType(c) {
		return
	}

	var req dto.UpdateMyProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.UpdateMyProfile(userId, &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "用户不存在") {
			errCode = constants.ErrCodeUserNotFound
		}
		uc.Error(c, errCode, err.Error())
		return
	}

	uc.SuccessWithMessage(c, "个人信息更新成功", response)
}

// SetInitialPassword 无密码用户（社交注册）设置初始密码
// POST /api/user/set-initial-password
func (uc UserController) SetInitialPassword(c httpInterfaces.RouterContextInterface) {
	userId, ok := middleware.GetCurrentUserID(c)
	if !ok {
		uc.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}
	if !uc.requireJSONContentType(c) {
		return
	}

	var req dto.SetInitialPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		uc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	if err := userService.SetInitialPassword(userId, req.NewPassword); err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "已设置密码") {
			errCode = constants.ErrCodeConflict
		} else if strings.Contains(errMsg, "用户不存在") {
			errCode = constants.ErrCodeUserNotFound
		}

		uc.Error(c, errCode, errMsg)
		return
	}

	uc.SuccessWithMessage(c, "密码设置成功", nil)
}

// Logout 用户退出登录
// POST /api/auth/logout
func (uc UserController) Logout(c httpInterfaces.RouterContextInterface) {
	// 这里可以添加令牌失效逻辑
	// 例如将令牌加入黑名单或从Redis中删除

	uc.SuccessWithMessage(c, "退出登录成功", nil)
}

// RefreshToken 刷新访问令牌
// POST /api/auth/refresh
func (uc UserController) RefreshToken(c httpInterfaces.RouterContextInterface) {
	// 从请求头或请求体中获取刷新令牌
	refreshToken := c.GetHeader("Refresh-Token")
	if refreshToken == "" {
		var req struct {
			RefreshToken string `json:"refreshToken" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			uc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
			return
		}
		refreshToken = req.RefreshToken
	}

	// 验证刷新令牌并生成新的访问令牌
	jwtAuthService, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		uc.Error(c, constants.ErrCodeInternal, "JWT配置错误: "+err.Error())
		return
	}
	newAccessToken, err := jwtAuthService.RefreshToken(refreshToken)
	if err != nil {
		uc.Error(c, constants.ErrCodeUnauthorized, "刷新令牌无效或已过期: "+err.Error())
		return
	}

	response := map[string]any{
		"accessToken": newAccessToken,
		"tokenType":   "Bearer",
		"expiresIn":   jwtAuthService.GetConfig().GetAccessTokenHours() * 3600, // 转换为秒
	}

	uc.SuccessWithMessage(c, "令牌刷新成功", response)
}

// GetAccessCodes 获取用户权限码
// GET /api/auth/codes
func (uc UserController) GetAccessCodes(c httpInterfaces.RouterContextInterface) {
	userId, ok := middleware.GetCurrentUserID(c)
	if !ok {
		uc.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}

	// 获取用户的所有权限码
	userService := service.NewUserService(helper.GetDatabase())
	permissions, err := userService.GetUserPermissions(userId)
	if err != nil {
		uc.Error(c, constants.ErrCodeInternal, "获取权限码失败: "+err.Error())
		return
	}

	// 提取权限码
	accessCodes := make([]string, 0, len(permissions))
	for _, perm := range permissions {
		if perm.Code != "" {
			accessCodes = append(accessCodes, perm.Code)
		}
	}

	uc.Success(c, accessCodes)
}
