package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// RBACMiddleware RBAC权限验证中间件
func RBACMiddleware(helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取当前用户ID（从JWT中间件或session中获取）
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "用户未登录",
			})
			c.Abort()
			return
		}

		userID, ok := userIDInterface.(uint)
		if !ok {
			// 尝试字符串转换
			if userIDStr, ok := userIDInterface.(string); ok {
				if id, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
					userID = uint(id)
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{
						"code":    401,
						"message": "用户ID格式错误",
					})
					c.Abort()
					return
				}
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "用户ID类型错误",
				})
				c.Abort()
				return
			}
		}

		// 获取请求路径和方法
		path := c.Request.URL.Path
		method := c.Request.Method

		// 检查用户是否有URL访问权限
		userService := service.NewUserService(helper.GetDatabase())
		hasPermission, err := userService.CheckUserURLPermission(userID, path, method)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "权限检查失败: " + err.Error(),
			})
			c.Abort()
			return
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "没有访问权限",
			})
			c.Abort()
			return
		}

		// 将用户ID和权限信息存储到上下文中，供后续处理使用
		c.Set("user_id", userID)
		c.Next()
	}
}

// RequirePermission 要求特定权限的中间件
func RequirePermission(resource, action string, helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取当前用户ID
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "用户未登录",
			})
			c.Abort()
			return
		}

		userID, ok := userIDInterface.(uint)
		if !ok {
			// 尝试字符串转换
			if userIDStr, ok := userIDInterface.(string); ok {
				if id, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
					userID = uint(id)
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{
						"code":    401,
						"message": "用户ID格式错误",
					})
					c.Abort()
					return
				}
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "用户ID类型错误",
				})
				c.Abort()
				return
			}
		}

		// 检查用户是否有特定权限
		userService := service.NewUserService(helper.GetDatabase())
		permissionCode := resource + ":" + action
		hasPermission, err := userService.CheckUserPermission(userID, permissionCode)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "权限检查失败: " + err.Error(),
			})
			c.Abort()
			return
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "没有" + resource + ":" + action + "权限",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireRole 要求特定角色的中间件
func RequireRole(roleCode string, helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取当前用户ID
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "用户未登录",
			})
			c.Abort()
			return
		}

		userID, ok := userIDInterface.(uint)
		if !ok {
			// 尝试字符串转换
			if userIDStr, ok := userIDInterface.(string); ok {
				if id, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
					userID = uint(id)
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{
						"code":    401,
						"message": "用户ID格式错误",
					})
					c.Abort()
					return
				}
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "用户ID类型错误",
				})
				c.Abort()
				return
			}
		}

		// 获取用户角色
		userService := service.NewUserService(helper.GetDatabase())
		userRoles, err := userService.GetUserRoles(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "获取用户角色失败: " + err.Error(),
			})
			c.Abort()
			return
		}

		// 检查是否有指定角色
		hasRole := false
		for _, role := range userRoles {
			if role.Code == roleCode && role.Status == model.RoleStatusEnabled {
				hasRole = true
				break
			}
		}

		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "需要" + roleCode + "角色权限",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAnyRole 要求任意一个角色的中间件
func RequireAnyRole(roleCodes []string, helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取当前用户ID
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "用户未登录",
			})
			c.Abort()
			return
		}

		userID, ok := userIDInterface.(uint)
		if !ok {
			// 尝试字符串转换
			if userIDStr, ok := userIDInterface.(string); ok {
				if id, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
					userID = uint(id)
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{
						"code":    401,
						"message": "用户ID格式错误",
					})
					c.Abort()
					return
				}
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "用户ID类型错误",
				})
				c.Abort()
				return
			}
		}

		// 获取用户角色
		userService := service.NewUserService(helper.GetDatabase())
		userRoles, err := userService.GetUserRoles(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "获取用户角色失败: " + err.Error(),
			})
			c.Abort()
			return
		}

		// 检查是否有任意一个指定角色
		hasRole := false
		for _, role := range userRoles {
			if role.Status == model.RoleStatusEnabled {
				for _, roleCode := range roleCodes {
					if role.Code == roleCode {
						hasRole = true
						break
					}
				}
				if hasRole {
					break
				}
			}
		}

		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "需要以下任意一个角色权限: " + strings.Join(roleCodes, ", "),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
