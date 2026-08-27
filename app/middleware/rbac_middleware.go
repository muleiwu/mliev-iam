package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// resolveUserID 从上下文解析 user_id，支持 uint / string 两种存储。
// 失败时自动写回 401 并返回 (0, false)。
func resolveUserID(c httpInterfaces.RouterContextInterface) (uint, bool) {
	v := c.Get("user_id")
	if v == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
			"code":    401,
			"message": "用户未登录",
		})
		return 0, false
	}
	if id, ok := v.(uint); ok {
		return id, true
	}
	if s, ok := v.(string); ok {
		if id, err := strconv.ParseUint(s, 10, 32); err == nil {
			return uint(id), true
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
			"code":    401,
			"message": "用户ID格式错误",
		})
		return 0, false
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
		"code":    401,
		"message": "用户ID类型错误",
	})
	return 0, false
}

// RBACMiddleware RBAC 权限验证中间件
func RBACMiddleware() httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		userID, ok := resolveUserID(c)
		if !ok {
			return
		}

		userService := service.NewUserService(helper.GetDatabase())
		hasPermission, err := userService.CheckUserURLPermission(userID, c.Path(), c.Method())
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
				"code":    500,
				"message": "权限检查失败: " + err.Error(),
			})
			return
		}

		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"code":    403,
				"message": "没有访问权限",
			})
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

// RequirePermission 要求特定权限的中间件
func RequirePermission(resource, action string) httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		userID, ok := resolveUserID(c)
		if !ok {
			return
		}

		userService := service.NewUserService(helper.GetDatabase())
		hasPermission, err := userService.CheckUserPermission(userID, resource+":"+action)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
				"code":    500,
				"message": "权限检查失败: " + err.Error(),
			})
			return
		}

		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"code":    403,
				"message": "没有" + resource + ":" + action + "权限",
			})
			return
		}

		c.Next()
	}
}

// RequireRole 要求特定角色的中间件
func RequireRole(roleCode string) httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		userID, ok := resolveUserID(c)
		if !ok {
			return
		}

		userService := service.NewUserService(helper.GetDatabase())
		userRoles, err := userService.GetEffectiveRoleDefinitions(userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
				"code":    500,
				"message": "获取用户角色失败: " + err.Error(),
			})
			return
		}

		hasRole := false
		for _, role := range userRoles {
			if role.Code == roleCode && role.IsEnabled() {
				hasRole = true
				break
			}
		}

		if !hasRole {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"code":    403,
				"message": "需要" + roleCode + "角色权限",
			})
			return
		}

		c.Next()
	}
}

// RequireAnyRole 要求任意一个角色的中间件
func RequireAnyRole(roleCodes []string) httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		userID, ok := resolveUserID(c)
		if !ok {
			return
		}

		userService := service.NewUserService(helper.GetDatabase())
		userRoles, err := userService.GetEffectiveRoleDefinitions(userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
				"code":    500,
				"message": "获取用户角色失败: " + err.Error(),
			})
			return
		}

		hasRole := false
		for _, role := range userRoles {
			if !role.IsEnabled() {
				continue
			}
			for _, code := range roleCodes {
				if role.Code == code {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}

		if !hasRole {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"code":    403,
				"message": "需要以下任意一个角色权限: " + strings.Join(roleCodes, ", "),
			})
			return
		}

		c.Next()
	}
}
