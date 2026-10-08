package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/container"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"golang.org/x/net/idna"
	"gorm.io/gorm"
)

var oidcCORSHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-Trace-Id"}

// oidcCORSEndpoint matches only the registered protocol endpoints. In particular,
// OPTIONS has no matched route/Params yet, so classification uses the URL path.
func oidcCORSEndpoint(request *http.Request) (clientID string, methods []string, public bool) {
	parts := strings.Split(request.URL.Path, "/")
	if len(parts) < 4 || parts[0] != "" || parts[1] != "oidc" || parts[2] == "" || parts[2] == "." || parts[2] == ".." {
		return "", nil, false
	}
	if len(parts) == 5 && parts[3] == ".well-known" && parts[4] == "openid-configuration" {
		return parts[2], []string{http.MethodGet}, true
	}
	if len(parts) != 4 {
		return "", nil, false
	}
	switch parts[3] {
	case "jwks":
		return parts[2], []string{http.MethodGet}, true
	case "token", "revoke", "introspect":
		return parts[2], []string{http.MethodPost}, false
	case "userinfo":
		return parts[2], []string{http.MethodGet, http.MethodPost}, false
	default:
		return "", nil, false
	}
}

// serveOIDCCORS owns CORS for a recognized OIDC endpoint, including errors and
// unknown/disabled clients. It never falls back to the management API whitelist.
func serveOIDCCORS(c httpInterfaces.RouterContextInterface) bool {
	clientID, methods, public := oidcCORSEndpoint(c.Request())
	if methods == nil {
		return false
	}
	header := c.ResponseWriter().Header()
	for _, name := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Expose-Headers", "Access-Control-Max-Age"} {
		header.Del(name)
	}
	origin := c.GetHeader("Origin")
	allowed := ""
	// Match the same decoded path as Gin, but reject encoded separators rather
	// than falling through to the global (possibly credentialed) whitelist.
	ambiguousPath := strings.Contains(strings.ToLower(c.Request().URL.RawPath), "%2f")
	if public && !ambiguousPath {
		allowed = "*"
	} else {
		appendCORSVary(header, "Origin")
		if !ambiguousPath && origin != "" && oidcOriginAllowed(c.Request(), clientID, origin) {
			allowed = origin
		}
	}
	if c.Method() == http.MethodOptions {
		appendCORSVary(header, "Access-Control-Request-Method", "Access-Control-Request-Headers")
		if origin == "" || allowed == "" || !corsContains(methods, c.GetHeader("Access-Control-Request-Method"), false) || !oidcRequestedHeadersAllowed(c.GetHeader("Access-Control-Request-Headers")) {
			c.AbortWithStatus(http.StatusForbidden)
			return true
		}
		c.SetHeader("Access-Control-Allow-Origin", allowed)
		c.SetHeader("Access-Control-Allow-Methods", strings.Join(append(methods, http.MethodOptions), ", "))
		c.SetHeader("Access-Control-Allow-Headers", strings.Join(oidcCORSHeaders, ", "))
		c.SetHeader("Access-Control-Max-Age", "300")
		c.AbortWithStatus(http.StatusNoContent)
		return true
	}
	if allowed != "" {
		c.SetHeader("Access-Control-Allow-Origin", allowed)
		c.SetHeader("Access-Control-Expose-Headers", "Content-Length, Content-Type, Trace-Id, X-Trace-Id, WWW-Authenticate")
	}
	c.Next()
	return true
}

func oidcOriginAllowed(request *http.Request, clientID, origin string) bool {
	incoming, ok := oidcURLOrigin(origin, true)
	if !ok {
		return false
	}
	db, err := container.Get[*gorm.DB]()
	if err != nil || db == nil {
		return false
	}
	var app model.Application
	if err := db.WithContext(request.Context()).Where("uuid = ? AND protocol_type = ? AND is_active = ?", clientID, "oidc", true).First(&app).Error; err != nil {
		return false
	}
	// serializer:json decodes redirectUris as []interface{}; malformed entries
	// are ignored rather than widening access to the global/default origins.
	redirects, ok := app.ProtocolConfig["redirectUris"].([]interface{})
	if !ok {
		return false
	}
	for _, entry := range redirects {
		redirect, ok := entry.(string)
		if !ok {
			continue
		}
		registered, ok := oidcURLOrigin(redirect, false)
		if ok && registered == incoming {
			return true
		}
	}
	return false
}

// oidcURLOrigin serializes HTTP(S) origins. Native redirect URI port relaxation
// does not apply to CORS. Origin request headers cannot contain a path/query.
func oidcURLOrigin(raw string, originOnly bool) (string, bool) {
	if raw == "" || raw == "null" || raw != strings.TrimSpace(raw) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Host == "" || u.User != nil || u.Fragment != "" || strings.Contains(raw, "#") {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if originOnly && (u.Path != "" || u.RawQuery != "" || u.ForceQuery) {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.ContainsAny(host, "%*\\") {
		return "", false
	}
	if address, err := netip.ParseAddr(host); err == nil {
		host = address.String()
	} else {
		if strings.HasPrefix(u.Host, "[") {
			return "", false
		}
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || host == "" || strings.Contains(host, ":") {
			return "", false
		}
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 {
			return "", false
		}
		port = strconv.Itoa(n)
		if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
			port = ""
		}
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host, true
}

func oidcRequestedHeadersAllowed(raw string) bool {
	if raw == "" {
		return true
	}
	for _, name := range strings.Split(raw, ",") {
		if !corsContains(oidcCORSHeaders, strings.TrimSpace(name), true) {
			return false
		}
	}
	return true
}

func corsContains(values []string, target string, fold bool) bool {
	for _, value := range values {
		if value == target || (fold && strings.EqualFold(value, target)) {
			return true
		}
	}
	return false
}

func appendCORSVary(header http.Header, fields ...string) {
	values := []string{}
	for _, line := range header.Values("Vary") {
		for _, value := range strings.Split(line, ",") {
			value = strings.TrimSpace(value)
			if value == "*" {
				return
			}
			if value != "" && !corsContains(values, value, true) {
				values = append(values, value)
			}
		}
	}
	for _, field := range fields {
		if !corsContains(values, field, true) {
			values = append(values, field)
		}
	}
	header.Set("Vary", strings.Join(values, ", "))
}
