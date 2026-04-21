package middleware

import (
	"net/http"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// AdminCookieName 管理员 Access Token 的 HttpOnly Cookie 名
const AdminCookieName = "admin_access_token"

// extractBearerToken 从 Authorization 头或 HttpOnly Cookie 提取 JWT
// 优先 Authorization 头（便于 API 调试与移动端），其次 Cookie（浏览器端防 XSS）
func extractBearerToken(c httpInterfaces.RouterContextInterface) string {
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return strings.TrimSpace(parts[1])
		}
		return ""
	}
	if cookie, err := c.Cookie(AdminCookieName); err == nil && cookie != "" {
		return cookie
	}
	return ""
}

// AuthMiddleware JWT认证中间件
// 支持两种凭据来源（任选其一）：
//  1. Authorization: Bearer <token>
//  2. HttpOnly Cookie admin_access_token（浏览器推荐，防 XSS 窃取）
func AuthMiddleware() httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		tokenString := extractBearerToken(c)
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.Response{
				Code:    constants.ErrCodeUnauthorized,
				Message: "缺少认证令牌",
			})
			return
		}

		jwtAuthService := service.NewJWTAuthService()
		claims, err := jwtAuthService.ValidateToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.Response{
				Code:    constants.ErrCodeUnauthorized,
				Message: "认证令牌无效: " + err.Error(),
			})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("claims", claims)

		c.Next()
	}
}

// AdminAuthMiddleware 管理员认证中间件
func AdminAuthMiddleware() httpInterfaces.HandlerFunc {
	authMw := AuthMiddleware()
	return func(c httpInterfaces.RouterContextInterface) {
		authMw(c)
		if c.IsAborted() {
			return
		}

		role := c.Get("role")
		if role == nil || role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, dto.Response{
				Code:    constants.ErrCodeForbidden,
				Message: "权限不足，仅限管理员访问",
			})
			return
		}

		c.Next()
	}
}

// OptionalAuthMiddleware 可选认证中间件
// 如果提供了token则验证，如果没有提供则继续执行，但不设置用户信息
func OptionalAuthMiddleware() httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		tokenString := extractBearerToken(c)
		if tokenString == "" {
			c.Next()
			return
		}
		jwtAuthService := service.NewJWTAuthService()
		claims, err := jwtAuthService.ValidateToken(tokenString)
		if err != nil {
			c.Next()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("claims", claims)

		c.Next()
	}
}

// GetCurrentUser 从上下文中获取当前用户信息
func GetCurrentUser(c httpInterfaces.RouterContextInterface) (uint, string, bool) {
	userID := c.Get("user_id")
	username := c.Get("username")
	if userID == nil || username == nil {
		return 0, "", false
	}
	uid, ok1 := userID.(uint)
	uname, ok2 := username.(string)
	if !ok1 || !ok2 {
		return 0, "", false
	}
	return uid, uname, true
}

// GetCurrentUserID 从上下文中获取当前用户ID
func GetCurrentUserID(c httpInterfaces.RouterContextInterface) (uint, bool) {
	v := c.Get("user_id")
	if v == nil {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}

// GetCurrentRole 从上下文中获取当前用户角色
func GetCurrentRole(c httpInterfaces.RouterContextInterface) (string, bool) {
	v := c.Get("role")
	if v == nil {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}
