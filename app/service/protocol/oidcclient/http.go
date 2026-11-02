// Package oidcclient defines the transport policy shared by external OIDC discovery,
// token, JWKS and UserInfo requests.
package oidcclient

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ValidateURL permits HTTP only for local development. Private HTTPS issuers
// remain supported for enterprise identity providers.
func ValidateURL(raw, field string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.Contains(raw, "#") {
		return fmt.Errorf("OIDC %s URL 无效", field)
	}
	if field == "issuer" && (u.RawQuery != "" || u.ForceQuery) {
		return fmt.Errorf("OIDC issuer 不能包含查询参数")
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && (strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && ip.IsLoopback())) {
		return nil
	}
	return fmt.Errorf("OIDC %s 必须使用 HTTPS（仅本机回环地址允许 HTTP）", field)
}
