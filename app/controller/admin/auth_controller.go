package admin

import (
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
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
	if !ac.isAdminUser(userResponse, helper) {
		ac.Error(c, constants.ErrCodeForbidden, "权限不足，仅限管理员登录")
		return
	}

	// 生成访问令牌
	jwtAuthService := service.NewJWTAuthService()
	accessToken, err := jwtAuthService.GenerateToken(userResponse)
	if err != nil {
		ac.Error(c, constants.ErrCodeInternal, "生成访问令牌失败: "+err.Error())
		return
	}

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
		ExpiresIn:   int64(jwtAuthService.GetConfig().GetAccessTokenHours() * 3600), // 转换为秒
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
	jwtAuthService := service.NewJWTAuthService()
	newAccessToken, err := jwtAuthService.RefreshToken(refreshToken)
	if err != nil {
		ac.Error(c, constants.ErrCodeUnauthorized, "刷新令牌无效或已过期: "+err.Error())
		return
	}

	response := gin.H{
		"accessToken": newAccessToken,
		"tokenType":   "Bearer",
		"expiresIn":   jwtAuthService.GetConfig().GetAccessTokenHours() * 3600, // 转换为秒
	}

	ac.SuccessWithMessage(c, "令牌刷新成功", response)
}

// GetAdminProfile 获取管理员个人信息
// GET /api/admin/auth/profile
func (ac AuthController) GetAdminProfile(c *gin.Context, helper interfaces.HelperInterface) {
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
	response := gin.H{
		"id":       userResponse.ID,
		"username": userResponse.Username,
		"email":    userResponse.Email,
		"role":     "administrator",
		"profile":  userResponse,
	}

	ac.SuccessWithMessage(c, "获取管理员信息成功", response)
}

// isAdminUser 检查用户是否为管理员
// 通过查询用户是否关联 super_admin 系统角色来判断
func (ac AuthController) isAdminUser(user *dto.UserResponse, helper interfaces.HelperInterface) bool {
	// 创建 RoleDao 实例
	roleDao := dao.NewRoleDao(helper.GetDatabase())

	// 查询 super_admin 角色
	superAdminRole, err := roleDao.GetByCode("super_admin")
	if err != nil {
		// 如果角色不存在，返回 false
		return false
	}

	// 验证是否为系统角色
	if !superAdminRole.IsSystem() {
		return false
	}

	// 创建 UserRoleDao 实例
	userRoleDao := dao.NewUserRoleDao(helper.GetDatabase())

	// 查询用户和角色的关联
	userRole, err := userRoleDao.GetByUserIDAndRoleID(user.ID, superAdminRole.ID)
	if err != nil {
		// 如果关联不存在，返回 false
		return false
	}

	// 检查关联是否有效（激活、已批准且未过期）
	return userRole.IsValid()
}
