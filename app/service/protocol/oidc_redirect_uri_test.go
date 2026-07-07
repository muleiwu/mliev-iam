package protocol

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

func testAppWithRedirectURIs(uris ...interface{}) *model.Application {
	return &model.Application{
		ProtocolType: string(model.ProtocolTypeOIDC),
		IsActive:     true,
		ProtocolConfig: map[string]interface{}{
			"redirectUris": uris,
		},
	}
}

func TestValidateRedirectURIExactMatch(t *testing.T) {
	handler := NewOIDCProtocolHandler(nil)
	app := testAppWithRedirectURIs("https://app.example.com/callback")

	if err := handler.ValidateRedirectURI(app, "https://app.example.com/callback"); err != nil {
		t.Fatalf("exact match should pass: %v", err)
	}
	if err := handler.ValidateRedirectURI(app, "https://app.example.com/other"); err == nil {
		t.Fatalf("different path should be rejected")
	}
	if err := handler.ValidateRedirectURI(app, "https://app.example.com:8443/callback"); err == nil {
		t.Fatalf("non-loopback host must match port exactly")
	}
	if err := handler.ValidateRedirectURI(app, "https://app.example.com/callback#frag"); err == nil {
		t.Fatalf("fragment should be rejected")
	}
}

func TestValidateRedirectURILoopbackIgnoresPort(t *testing.T) {
	handler := NewOIDCProtocolHandler(nil)
	app := testAppWithRedirectURIs("http://127.0.0.1/callback")

	for _, uri := range []string{
		"http://127.0.0.1/callback",
		"http://127.0.0.1:53821/callback",
		"http://127.0.0.1:80/callback",
	} {
		if err := handler.ValidateRedirectURI(app, uri); err != nil {
			t.Fatalf("loopback %s should ignore port: %v", uri, err)
		}
	}

	for name, uri := range map[string]string{
		"path mismatch":            "http://127.0.0.1:53821/other",
		"query mismatch":           "http://127.0.0.1:53821/callback?extra=1",
		"https not relaxed":        "https://127.0.0.1:53821/callback",
		"localhost not relaxed":    "http://localhost:53821/callback",
		"ipv6 not cross-matched":   "http://[::1]:53821/callback",
		"non-loopback ip rejected": "http://127.0.0.2:53821/callback",
	} {
		if err := handler.ValidateRedirectURI(app, uri); err == nil {
			t.Fatalf("%s: %s should be rejected", name, uri)
		}
	}
}

func TestValidateRedirectURILoopbackIPv6(t *testing.T) {
	handler := NewOIDCProtocolHandler(nil)
	app := testAppWithRedirectURIs("http://[::1]/callback")

	if err := handler.ValidateRedirectURI(app, "http://[::1]:53821/callback"); err != nil {
		t.Fatalf("ipv6 loopback should ignore port: %v", err)
	}
	if err := handler.ValidateRedirectURI(app, "http://127.0.0.1:53821/callback"); err == nil {
		t.Fatalf("ipv4 should not match ipv6 registration")
	}
}

func TestValidateRedirectURIRegisteredWithPortStillIgnoresPort(t *testing.T) {
	// RFC 8252 §7.3：即使注册时带了端口，比较时也忽略端口
	handler := NewOIDCProtocolHandler(nil)
	app := testAppWithRedirectURIs("http://127.0.0.1:8080/callback")

	if err := handler.ValidateRedirectURI(app, "http://127.0.0.1:53821/callback"); err != nil {
		t.Fatalf("registered loopback with port should still ignore port: %v", err)
	}
}

func TestValidateRedirectURILocalhostStaysExact(t *testing.T) {
	handler := NewOIDCProtocolHandler(nil)
	app := testAppWithRedirectURIs("http://localhost:3000/callback")

	if err := handler.ValidateRedirectURI(app, "http://localhost:3000/callback"); err != nil {
		t.Fatalf("localhost exact match should pass: %v", err)
	}
	if err := handler.ValidateRedirectURI(app, "http://localhost:53821/callback"); err == nil {
		t.Fatalf("localhost must not get port relaxation")
	}
}
