package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/service"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

func oidcBrowserCookieName(sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return "oidc_browser_" + hex.EncodeToString(digest[:8])
}

func setOIDCBrowserBindingCookie(ctx httpInterfaces.RouterContextInterface, sessionID, token string) {
	isSecure := ctx.Request().TLS != nil
	if isSecure {
		ctx.SetSameSite(http.SameSiteNoneMode)
	} else {
		ctx.SetSameSite(http.SameSiteLaxMode)
	}
	ctx.SetCookie(oidcBrowserCookieName(sessionID), token, 10*60, "/", "", isSecure, true)
}

func getBoundOIDCSession(ctx httpInterfaces.RouterContextInterface, sessions *service.OIDCSessionService, sessionID string) (*service.OIDCSession, error) {
	session, err := sessions.GetSession(ctx.Request().Context(), sessionID)
	if err != nil {
		return nil, err
	}
	token, err := ctx.Cookie(oidcBrowserCookieName(sessionID))
	if err != nil {
		return nil, fmt.Errorf("OIDC 会话未绑定当前浏览器")
	}
	if err := sessions.ValidateBrowserBinding(session, token); err != nil {
		return nil, err
	}
	return session, nil
}

func oidcLoginURL(ticket, sessionID string) string {
	return "/api/oidc/session-bind?ticket=" + url.QueryEscape(ticket) +
		"&return_to=" + url.QueryEscape("/auth/#/oidc/authorize?session_id="+sessionID)
}

func safeOIDCReturnTo(value, sessionID string) string {
	if strings.HasPrefix(value, "/auth/") && !strings.HasPrefix(value, "//") {
		return value
	}
	return "/auth/#/oidc/authorize?session_id=" + url.QueryEscape(sessionID)
}
