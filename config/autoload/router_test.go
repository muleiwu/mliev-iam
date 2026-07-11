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
	router.assertRoute(t, "POST", "/oidc/:client_id/token")
	router.assertNoRoute(t, "GET", "/oidc/:client_id/token")

	if router.middlewarePrefixes["/api/auth"] == 0 {
		t.Fatal("/api/auth must keep AuthMiddleware protection")
	}
	if router.middlewarePrefixes["/api/oidc/auth"] != 0 {
		t.Fatal("/api/oidc/auth must stay public so it can use the OIDC refresh_token cookie")
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
