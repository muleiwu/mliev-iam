package autoload

import (
	"strings"
	"testing"

	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

func TestRouterKeepsUserAuthAndAddsOIDCAuthRoutes(t *testing.T) {
	router := newRecordingRouter()
	routerConfig := Router{}.InitConfig()["http.router"].(func(httpInterfaces.RouterInterface))

	routerConfig(router)

	router.assertRoute(t, "POST", "/api/auth/refresh")
	router.assertRoute(t, "POST", "/api/auth/logout")
	router.assertRoute(t, "GET", "/api/oidc/client-config")
	router.assertRoute(t, "POST", "/api/oidc/auth/refresh")
	router.assertRoute(t, "POST", "/api/oidc/auth/logout")
	router.assertRoute(t, "POST", "/api/oidc/auth/switch-account")
	router.assertRoute(t, "GET", "/api/agreements/global/:code")
	router.assertRoute(t, "GET", "/api/agreements/application")
	router.assertRoute(t, "GET", "/api/admin/agreements/global")
	router.assertRoute(t, "PUT", "/api/admin/agreements/global/:code")
	router.assertRoute(t, "GET", "/api/admin/applications/:id/agreements")
	router.assertRoute(t, "POST", "/api/admin/applications/:id/agreements")
	router.assertRoute(t, "POST", "/api/admin/attachments/application-icon")
	router.assertRoute(t, "GET", "/api/admin/auth/authorize")
	router.assertRoute(t, "GET", "/api/admin/auth/callback")
	router.assertRoute(t, "POST", "/api/admin/auth/logout")
	router.assertNoRoute(t, "POST", "/api/admin/auth/login")
	router.assertNoRoute(t, "POST", "/api/admin/auth/refresh")
	router.assertRoute(t, "POST", "/oidc/:client_id/token")
	router.assertNoRoute(t, "GET", "/oidc/:client_id/token")

	// 个人中心：资料/密码/绑定管理
	router.assertRoute(t, "GET", "/api/user/profile")
	router.assertRoute(t, "PUT", "/api/user/profile")
	router.assertRoute(t, "PUT", "/api/user/password")
	router.assertRoute(t, "GET", "/api/identity-connections")
	router.assertRoute(t, "GET", "/api/user/external-identities")
	router.assertRoute(t, "DELETE", "/api/user/external-identities/:id")
	router.assertRoute(t, "GET", "/external-auth/:identifier/authorize")
	router.assertRoute(t, "GET", "/external-auth/:identifier/callback")
	router.assertRoute(t, "GET", "/external-auth/:identifier/bind")
	router.assertRoute(t, "POST", "/external-auth/bind-with-login")
	router.assertRoute(t, "POST", "/external-auth/register")
	router.assertRoute(t, "POST", "/external-auth/:identifier/miniprogram/login")
	router.assertRoute(t, "POST", "/external-auth/:identifier/miniprogram/qrcode")
	router.assertRoute(t, "GET", "/external-auth/:identifier/miniprogram/qrcode/:scanHandle")
	router.assertRoute(t, "POST", "/external-auth/:identifier/miniprogram/qrcode/scan")
	router.assertRoute(t, "POST", "/external-auth/:identifier/miniprogram/qrcode/confirm")
	router.assertRoute(t, "GET", "/api/admin/identity-connections")
	router.assertRoute(t, "POST", "/api/admin/identity-connections/:id/test")
	router.assertRoute(t, "GET", "/api/admin/identity-connections/:id/directory-policy")
	router.assertRoute(t, "POST", "/api/admin/identity-connections/:id/sync-runs")
	router.assertRoute(t, "GET", "/api/admin/directory-sync-runs/:runId/changes")
	router.assertRoute(t, "POST", "/api/admin/directory-sync-runs/:runId/apply")
	router.assertRoute(t, "POST", "/api/admin/directory-sync-changes/:changeId/approve")
	router.assertRoute(t, "GET", "/api/admin/sync-jobs")
	router.assertRoute(t, "POST", "/api/admin/sync-jobs/:id/retry")
	router.assertRoute(t, "GET", "/api/admin/identity-connections/:id/sync-schedules")
	router.assertRoute(t, "GET", "/api/admin/external-events")
	router.assertRoute(t, "POST", "/api/admin/external-events/:id/replay")
	router.assertRoute(t, "GET", "/api/admin/sync-center/summary")
	router.assertRoute(t, "POST", "/external-events/:identifier")
	router.assertRoute(t, "GET", "/external-events/:identifier")

	// 用户组织部门关系管理
	router.assertRoute(t, "POST", "/api/admin/user-departments")
	router.assertRoute(t, "DELETE", "/api/admin/user-departments/:userId/:departmentId")
	router.assertRoute(t, "PUT", "/api/admin/users/:id/departments/:departmentId/primary")

	for _, route := range []string{
		"/social/providers", "/social/auth/:provider", "/social/callback/:provider",
		"/identity/providers", "/identity/auth/:provider", "/identity/callback/:provider",
		"/api/admin/social-providers", "/api/admin/identity-providers",
		"/api/user/social-accounts", "/api/user/identity-accounts",
	} {
		router.assertNoRoute(t, "GET", route)
	}

	if router.middlewarePrefixes["/api/auth"] == 0 {
		t.Fatal("/api/auth must keep AuthMiddleware protection")
	}
	if router.middlewarePrefixes["/api/user"] == 0 {
		t.Fatal("/api/user must keep AuthMiddleware protection")
	}
	if router.middlewarePrefixes["/api/oidc/auth"] != 0 {
		t.Fatal("/api/oidc/auth must stay public so it can use the OIDC refresh_token cookie")
	}
	if router.middlewarePrefixes["/api/admin/user-departments"] == 0 {
		t.Fatal("/api/admin/user-departments must keep AdminAuthMiddleware protection")
	}
	if router.middlewarePrefixes["/api/admin/users"] == 0 {
		t.Fatal("/api/admin/users must keep AdminAuthMiddleware protection")
	}
}

type recordingRouter struct {
	prefix             string
	routes             map[string]bool
	middlewarePrefixes map[string]int
}

func newRecordingRouter() *recordingRouter {
	return &recordingRouter{
		routes:             map[string]bool{},
		middlewarePrefixes: map[string]int{},
	}
}

func (r *recordingRouter) assertRoute(t *testing.T, method, path string) {
	t.Helper()
	key := method + " " + path
	if !r.routes[key] {
		t.Fatalf("route %s was not registered", key)
	}
}

func (r *recordingRouter) assertNoRoute(t *testing.T, method, path string) {
	t.Helper()
	key := method + " " + path
	if r.routes[key] {
		t.Fatalf("route %s should not be registered", key)
	}
}

func (r *recordingRouter) GET(path string, _ httpInterfaces.HandlerFunc) {
	r.record("GET", path)
}

func (r *recordingRouter) POST(path string, _ httpInterfaces.HandlerFunc) {
	r.record("POST", path)
}

func (r *recordingRouter) PUT(path string, _ httpInterfaces.HandlerFunc) {
	r.record("PUT", path)
}

func (r *recordingRouter) DELETE(path string, _ httpInterfaces.HandlerFunc) {
	r.record("DELETE", path)
}

func (r *recordingRouter) PATCH(path string, _ httpInterfaces.HandlerFunc) {
	r.record("PATCH", path)
}

func (r *recordingRouter) HEAD(path string, _ httpInterfaces.HandlerFunc) {
	r.record("HEAD", path)
}

func (r *recordingRouter) OPTIONS(path string, _ httpInterfaces.HandlerFunc) {
	r.record("OPTIONS", path)
}

func (r *recordingRouter) Group(path string) httpInterfaces.RouterInterface {
	return &recordingRouter{
		prefix:             joinRoute(r.prefix, path),
		routes:             r.routes,
		middlewarePrefixes: r.middlewarePrefixes,
	}
}

func (r *recordingRouter) Use(middleware ...httpInterfaces.HandlerFunc) {
	r.middlewarePrefixes[r.prefix] += len(middleware)
}

func (r *recordingRouter) RegexGroup(prefix string, _ ...httpInterfaces.HandlerFunc) httpInterfaces.RegexRouterInterface {
	return &recordingRegexRouter{router: r.Group(prefix).(*recordingRouter)}
}

func (r *recordingRouter) record(method, path string) {
	r.routes[method+" "+joinRoute(r.prefix, path)] = true
}

type recordingRegexRouter struct {
	router *recordingRouter
}

func (r *recordingRegexRouter) GET(pattern string, _ ...httpInterfaces.HandlerFunc) {
	r.router.record("GET", pattern)
}

func (r *recordingRegexRouter) POST(pattern string, _ ...httpInterfaces.HandlerFunc) {
	r.router.record("POST", pattern)
}

func (r *recordingRegexRouter) PUT(pattern string, _ ...httpInterfaces.HandlerFunc) {
	r.router.record("PUT", pattern)
}

func (r *recordingRegexRouter) DELETE(pattern string, _ ...httpInterfaces.HandlerFunc) {
	r.router.record("DELETE", pattern)
}

func (r *recordingRegexRouter) PATCH(pattern string, _ ...httpInterfaces.HandlerFunc) {
	r.router.record("PATCH", pattern)
}

func (r *recordingRegexRouter) HEAD(pattern string, _ ...httpInterfaces.HandlerFunc) {
	r.router.record("HEAD", pattern)
}

func (r *recordingRegexRouter) OPTIONS(pattern string, _ ...httpInterfaces.HandlerFunc) {
	r.router.record("OPTIONS", pattern)
}

func (r *recordingRegexRouter) Any(pattern string, _ ...httpInterfaces.HandlerFunc) {
	r.router.record("ANY", pattern)
}

func joinRoute(prefix, path string) string {
	if path == "" {
		if prefix == "" {
			return "/"
		}
		return prefix
	}
	if prefix == "" {
		return path
	}
	return strings.TrimRight(prefix, "/") + path
}
