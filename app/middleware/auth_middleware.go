package middleware

import (
	"net/http"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware JWT认证中间件
func AuthMiddleware(helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头获取token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, dto.Response{
				Code:    constants.ErrCodeUnauthorized,
				Message: "缺少认证令牌",
			})
			c.Abort()
			return
		}

		// 检查Bearer前缀
		tokenParts := strings.SplitN(authHeader, " ", 2)
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, dto.Response{
				Code:    constants.ErrCodeUnauthorized,
				Message: "认证令牌格式错误",
			})
			c.Abort()
			return
		}

		tokenString := tokenParts[1]

		// 创建JWT认证服务实例
		jwtAuthService := service.NewJWTAuthService()

		// 验证token
		claims, err := jwtAuthService.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, dto.Response{
				Code:    constants.ErrCodeUnauthorized,
				Message: "认证令牌无效: " + err.Error(),
			})
			c.Abort()
			return
		}

		// 将用户信息存储到上下文中
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("claims", claims)

		c.Next()
	}
}

// AdminAuthMiddleware 管理员认证中间件
func AdminAuthMiddleware(helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 先执行基础认证
		AuthMiddleware(helper)(c)

		// 如果基础认证失败，直接返回
		if c.IsAborted() {
			return
		}

		// 检查是否为管理员角色
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, dto.Response{
				Code:    constants.ErrCodeForbidden,
				Message: "权限不足，仅限管理员访问",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuthMiddleware 可选认证中间件
// 如果提供了token则验证，如果没有提供则继续执行，但不设置用户信息
func OptionalAuthMiddleware(helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		tokenParts := strings.SplitN(authHeader, " ", 2)
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.Next()
			return
		}

		tokenString := tokenParts[1]
		jwtAuthService := service.NewJWTAuthService()

		claims, err := jwtAuthService.ValidateToken(tokenString)
		if err != nil {
			c.Next()
			return
		}

		// 将用户信息存储到上下文中
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("claims", claims)

		c.Next()
	}
}

// GetCurrentUser 从上下文中获取当前用户信息
func GetCurrentUser(c *gin.Context) (uint, string, bool) {
	userID, exists1 := c.Get("user_id")
	username, exists2 := c.Get("username")

	if !exists1 || !exists2 {
		return 0, "", false
	}

	return userID.(uint), username.(string), true
}

// GetCurrentUserID 从上下文中获取当前用户ID
func GetCurrentUserID(c *gin.Context) (uint, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	return userID.(uint), true
}

// GetCurrentRole 从上下文中获取当前用户角色
func GetCurrentRole(c *gin.Context) (string, bool) {
	role, exists := c.Get("role")
	if !exists {
		return "", false
	}
	return role.(string), true
}
