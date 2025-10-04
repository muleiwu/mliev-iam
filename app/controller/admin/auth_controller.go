package admin

import (
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// AuthController 管理员认证控制器
type AuthController struct {
	BaseController
}

// AdminLoginRequest 管理员登录请求
type AdminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AdminLoginResponse 管理员登录响应
type AdminLoginResponse struct {
	User        dto.UserResponse `json:"user"`
	AccessToken string           `json:"accessToken"`
	TokenType   string           `json:"tokenType"`
	ExpiresIn   int64            `json:"expiresIn"`
}

// Login 管理员登录
// POST /api/admin/auth/login
func (ac AuthController) Login(c *gin.Context, helper interfaces.HelperInterface) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ac.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	userResponse, err := userService.ValidateUserForLogin(req.Username, req.Password)
	if err != nil {
		errCode := constants.ErrCodeInvalidPassword
		errMsg := err.Error()

		if strings.Contains(errMsg, "用户账户已禁用") {
			errCode = constants.ErrCodeUserDisabled
		} else if strings.Contains(errMsg, "用户账户已被锁定") {
			errCode = constants.ErrCodeUserLocked
		} else if strings.Contains(errMsg, "用户账户未激活") {
			errCode = constants.ErrCodeUserNotActivated
		}

		ac.Error(c, errCode, errMsg)
		return
	}

	// 这里可以添加额外的管理员权限验证
	// 例如检查用户是否有管理员角色或权限
	if !ac.isAdminUser(userResponse) {
		ac.Error(c, constants.ErrCodeForbidden, "权限不足，仅限管理员登录")
		return
	}

	// 生成访问令牌（这里应该使用JWT或其他令牌机制）
	accessToken := ac.generateAccessToken(userResponse)

	// 更新用户登录IP
	clientIP := c.ClientIP()
	if clientIP != "" {
		// 这里可以通过服务层更新用户的最后登录IP
		// userService.UpdateLastLoginIP(userResponse.ID, clientIP)
	}

	response := AdminLoginResponse{
		User:        *userResponse,
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   3600, // 1小时
	}

	ac.SuccessWithMessage(c, "管理员登录成功", response)
}

// Logout 管理员退出登录
// POST /api/admin/auth/logout
func (ac AuthController) Logout(c *gin.Context, helper interfaces.HelperInterface) {
	// 这里可以添加令牌失效逻辑
	// 例如将令牌加入黑名单或从Redis中删除

	ac.SuccessWithMessage(c, "退出登录成功", nil)
}

// RefreshToken 刷新访问令牌
// POST /api/admin/auth/refresh
func (ac AuthController) RefreshToken(c *gin.Context, helper interfaces.HelperInterface) {
	// 从请求头或请求体中获取刷新令牌
	refreshToken := c.GetHeader("Refresh-Token")
	if refreshToken == "" {
		var req struct {
			RefreshToken string `json:"refreshToken" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			ac.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
			return
		}
		refreshToken = req.RefreshToken
	}

	// 验证刷新令牌并生成新的访问令牌
	// 这里应该实现具体的令牌验证和生成逻辑
	newAccessToken := ac.refreshAccessToken(refreshToken)
	if newAccessToken == "" {
		ac.Error(c, constants.ErrCodeUnauthorized, "刷新令牌无效或已过期")
		return
	}

	response := gin.H{
		"accessToken": newAccessToken,
		"tokenType":   "Bearer",
		"expiresIn":   3600,
	}

	ac.SuccessWithMessage(c, "令牌刷新成功", response)
}

// GetAdminProfile 获取管理员个人信息
// GET /api/admin/auth/profile
func (ac AuthController) GetAdminProfile(c *gin.Context, helper interfaces.HelperInterface) {
	// 这里需要从认证中间件或JWT中获取当前管理员用户ID
	// 暂时使用查询参数模拟
	adminUserIDStr := c.Query("adminUserId")
	if adminUserIDStr == "" {
		ac.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}

	// userService := service.NewUserService(helper.GetDatabase())
	// 这里需要解析管理员用户ID并获取用户信息
	// 为了简化，暂时返回模拟数据
	response := gin.H{
		"id":       1,
		"username": "admin",
		"email":    "admin@example.com",
		"role":     "administrator",
	}

	ac.Success(c, response)
}

// isAdminUser 检查用户是否为管理员
// 这里应该根据实际的角色权限系统来实现
func (ac AuthController) isAdminUser(user *dto.UserResponse) bool {
	// 简单的检查逻辑，可以根据实际需求扩展
	// 例如检查用户名是否为admin，或者检查用户的角色权限
	adminUsernames := []string{"admin", "administrator", "root"}
	for _, adminUsername := range adminUsernames {
		if strings.EqualFold(user.Username, adminUsername) {
			return true
		}
	}

	// 这里还可以添加基于角色的检查
	// 例如检查用户是否有管理员角色
	// return user.HasRole("admin") || user.HasPermission("admin_access")

	return false
}

// generateAccessToken 生成访问令牌
// 这里应该使用JWT或其他安全的令牌机制
func (ac AuthController) generateAccessToken(user *dto.UserResponse) string {
	// 简化的令牌生成逻辑
	// 实际应用中应该使用JWT或类似的安全令牌
	return "admin_token_" + user.Username + "_" + "1234567890"
}

// refreshAccessToken 刷新访问令牌
// 这里应该验证刷新令牌并生成新的访问令牌
func (ac AuthController) refreshAccessToken(refreshToken string) string {
	// 简化的令牌刷新逻辑
	// 实际应用中应该验证刷新令牌的有效性
	if refreshToken != "" {
		return "new_admin_token_" + "1234567890"
	}
	return ""
}
