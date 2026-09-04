package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"gorm.io/gorm"
)

// AdminCookieName 管理员 Access Token 的 HttpOnly Cookie 名
const AdminCookieName = "admin_access_token"

// UserCookieName 普通用户会话 Access Token 的 HttpOnly Cookie 名（个人中心使用）
const UserCookieName = "user_access_token"

// extractTokenCandidates 按优先级返回待校验的凭据候选：
// Authorization 头存在时仅使用该头（严格校验，不回退 Cookie，保持 API 语义）；
// 否则依次尝试 admin_access_token、user_access_token Cookie，
// 由调用方逐个校验，第一个有效者生效（过期的 admin Cookie 可落到 user Cookie）。
func extractTokenCandidates(c httpInterfaces.RouterContextInterface) []string {
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return []string{strings.TrimSpace(parts[1])}
		}
		return nil
	}
	var candidates []string
	for _, name := range []string{AdminCookieName, UserCookieName} {
		if cookie, err := c.Cookie(name); err == nil && cookie != "" {
			candidates = append(candidates, cookie)
		}
	}
	return candidates
}

// validateTokenCandidates 依次校验候选凭据，返回第一个有效的 Claims
func validateTokenCandidates(jwtAuthService *service.JWTAuthService, candidates []string) (*jwt.Claims, error) {
	var lastErr error
	for _, token := range candidates {
		claims, err := jwtAuthService.ValidateToken(token)
		if err == nil {
			if err = validateUserSession(claims); err == nil {
				return claims, nil
			}
		}
		lastErr = err
	}
	return nil, lastErr
}

func validateUserSession(claims *jwt.Claims) error {
	return validateUserSessionWithDB(helper.GetDatabase(), claims)
}

// ValidateUserSessionToken validates one raw IAM user-session token without
// consulting request headers. OIDC registration-intent detection uses this for
// the HttpOnly user cookie and deliberately ignores JavaScript-supplied bearer
// tokens.
func ValidateUserSessionToken(token string) (*jwt.Claims, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("认证令牌为空")
	}
	jwtAuthService, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		return nil, err
	}
	return validateTokenCandidates(jwtAuthService, []string{token})
}

func validateUserSessionWithDB(database *gorm.DB, claims *jwt.Claims) error {
	if claims == nil || claims.IssuedAt == nil {
		return fmt.Errorf("令牌缺少签发时间")
	}
	if database == nil {
		return fmt.Errorf("用户状态校验失败")
	}
	var state struct {
		Status           int
		IsActivated      bool
		SessionRevokedAt *time.Time
	}
	// 显式 SELECT * 让数据库只返回实际存在的列。旧部署在执行
	// 20260720150000 迁移前没有 session_revoked_at；直接点名该列会让一个
	// 已成功登录的有效用户被误判为“不存在”。迁移完成后 GORM 仍会正常把该列
	// 扫描进 SessionRevokedAt，从而保留会话撤销检查。
	if err := database.Table("users").Select("*").Where("id = ? AND deleted_at IS NULL", claims.UserID).Take(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户不存在或不可用")
		}
		return fmt.Errorf("用户状态校验失败")
	}
	if state.Status != int(model.UserStatusActive) || !state.IsActivated {
		return fmt.Errorf("用户已停用")
	}
	if state.SessionRevokedAt != nil && !claims.IssuedAt.Time.After(*state.SessionRevokedAt) {
		return fmt.Errorf("会话已撤销")
	}
	return nil
}

// authenticate 校验请求凭据并写入用户上下文（不推进 handler 链）。
// 返回 false 表示已写入错误响应并中止。
func authenticate(c httpInterfaces.RouterContextInterface) bool {
	candidates := extractTokenCandidates(c)
	if len(candidates) == 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, dto.Response{
			Code:    constants.ErrCodeUnauthorized,
			Message: "缺少认证令牌",
		})
		return false
	}

	jwtAuthService, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, dto.Response{
			Code:    constants.ErrCodeInternal,
			Message: "JWT配置错误: " + err.Error(),
		})
		return false
	}
	claims, err := validateTokenCandidates(jwtAuthService, candidates)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, dto.Response{
			Code:    constants.ErrCodeUnauthorized,
			Message: "认证令牌无效: " + err.Error(),
		})
		return false
	}

	c.Set("user_id", claims.UserID)
	c.Set("username", claims.Username)
	c.Set("role", claims.Role)
	c.Set("claims", claims)
	return true
}

// AuthMiddleware JWT认证中间件
// 支持两种凭据来源（任选其一）：
//  1. Authorization: Bearer <token>
//  2. HttpOnly Cookie admin_access_token / user_access_token（浏览器推荐，防 XSS 窃取）
func AuthMiddleware() httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		if !authenticate(c) {
			return
		}
		c.Next()
	}
}

// AdminAuthMiddleware 管理员认证中间件。
// 角色检查必须发生在 c.Next() 之前——不能复用 AuthMiddleware() 再补检查，
// 其内部的 c.Next() 会先把业务 handler 跑完，403 就形同虚设。
func AdminAuthMiddleware() httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		if !authenticate(c) {
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
		claims, ok := c.Get("claims").(*jwt.Claims)
		if !ok || claims == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, dto.Response{
				Code:    constants.ErrCodeForbidden,
				Message: "管理员会话声明无效",
			})
			return
		}
		isAdmin, err := service.IsSuperAdmin(helper.GetDatabase(), claims.UserID)
		if err != nil || !isAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, dto.Response{
				Code:    constants.ErrCodeForbidden,
				Message: "超级管理员角色已失效",
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
		candidates := extractTokenCandidates(c)
		if len(candidates) == 0 {
			c.Next()
			return
		}
		jwtAuthService, err := service.NewJWTAuthServiceWithError()
		if err != nil {
			c.Next()
			return
		}
		claims, err := validateTokenCandidates(jwtAuthService, candidates)
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
