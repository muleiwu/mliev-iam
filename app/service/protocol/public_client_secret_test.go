package protocol

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

func testOIDCConfig(clientType string) map[string]interface{} {
	return map[string]interface{}{
		"redirectUris":  []interface{}{"https://app.example.com/callback"},
		"grantTypes":    []interface{}{"authorization_code", "refresh_token"},
		"responseTypes": []interface{}{"code"},
		"scopes":        []interface{}{"openid", "profile"},
		"clientType":    clientType,
		"requirePkce":   false,
	}
}

func testOAuth2Config(clientType string) map[string]interface{} {
	return map[string]interface{}{
		"redirectUris": []interface{}{"https://app.example.com/callback"},
		"grantTypes":   []interface{}{"authorization_code", "refresh_token"},
		"clientType":   clientType,
	}
}

func TestOIDCPublicClientDoesNotCreateOrRequireSecret(t *testing.T) {
	handler := NewOIDCProtocolHandler(nil)
	config := testOIDCConfig("public")
	config["clientSecret"] = "incoming-secret"

	prepared, err := handler.PrepareConfig("client-public", config)
	if err != nil {
		t.Fatalf("prepare config: %v", err)
	}
	if _, ok := prepared["clientSecret"]; ok {
		t.Fatalf("public client should not expose clientSecret in prepared config")
	}
	if requirePkce, ok := prepared["requirePkce"].(bool); !ok || !requirePkce {
		t.Fatalf("public client should force requirePkce=true, got %#v", prepared["requirePkce"])
	}

	if err := handler.EncryptSensitiveFields(prepared); err != nil {
		t.Fatalf("encrypt sensitive fields: %v", err)
	}
	if _, ok := prepared["clientSecret"]; ok {
		t.Fatalf("public client should not persist clientSecret")
	}

	app := &model.Application{
		ProtocolType:   string(model.ProtocolTypeOIDC),
		IsActive:       true,
		ProtocolConfig: prepared,
	}
	if err := handler.ValidateClient(app, ""); err != nil {
		t.Fatalf("public client should validate without secret: %v", err)
	}
	if err := handler.ValidateClient(app, "leaked-secret"); err == nil {
		t.Fatalf("public client should reject provided secret")
	}
}

func TestOIDCConfidentialClientCreatesAndValidatesSecret(t *testing.T) {
	handler := NewOIDCProtocolHandler(nil)

	prepared, err := handler.PrepareConfig("client-confidential", testOIDCConfig("confidential"))
	if err != nil {
		t.Fatalf("prepare config: %v", err)
	}
	plainSecret, ok := prepared["clientSecret"].(string)
	if !ok || plainSecret == "" {
		t.Fatalf("confidential client should expose generated clientSecret once")
	}

	if err := handler.EncryptSensitiveFields(prepared); err != nil {
		t.Fatalf("encrypt sensitive fields: %v", err)
	}
	if prepared["clientSecret"] == plainSecret {
		t.Fatalf("confidential clientSecret should be hashed before persisting")
	}

	app := &model.Application{
		ProtocolType:   string(model.ProtocolTypeOIDC),
		IsActive:       true,
		ProtocolConfig: prepared,
	}
	if err := handler.ValidateClient(app, plainSecret); err != nil {
		t.Fatalf("confidential client should validate generated secret: %v", err)
	}
	if err := handler.ValidateClient(app, ""); err == nil {
		t.Fatalf("confidential client should reject missing secret")
	}
}

func TestOIDCUpdateToPublicRemovesExistingSecret(t *testing.T) {
	handler := NewOIDCProtocolHandler(nil)
	app := &model.Application{
		ProtocolType: string(model.ProtocolTypeOIDC),
		IsActive:     true,
		ProtocolConfig: map[string]interface{}{
			"clientId":      "client-id",
			"clientSecret":  "hashed-secret",
			"redirectUris":  []interface{}{"https://app.example.com/callback"},
			"grantTypes":    []interface{}{"authorization_code", "refresh_token"},
			"responseTypes": []interface{}{"code"},
			"scopes":        []interface{}{"openid", "profile"},
			"clientType":    "confidential",
			"requirePkce":   false,
			"tokenLifetime": 3600,
		},
	}

	if err := handler.UpdateApplication(app, map[string]interface{}{
		"protocolConfig": map[string]interface{}{"clientType": "public"},
	}); err != nil {
		t.Fatalf("update application: %v", err)
	}

	if _, ok := app.ProtocolConfig["clientSecret"]; ok {
		t.Fatalf("public client update should remove existing clientSecret")
	}
	if requirePkce, ok := app.ProtocolConfig["requirePkce"].(bool); !ok || !requirePkce {
		t.Fatalf("public client update should force requirePkce=true, got %#v", app.ProtocolConfig["requirePkce"])
	}
}

func TestOAuth2PublicClientDoesNotCreateOrPersistSecret(t *testing.T) {
	handler := NewOAuth2ProtocolHandler(nil)
	config := testOAuth2Config("public")
	config["clientSecret"] = "incoming-secret"

	prepared, err := handler.PrepareConfig("client-public", config)
	if err != nil {
		t.Fatalf("prepare config: %v", err)
	}
	if _, ok := prepared["clientSecret"]; ok {
		t.Fatalf("public OAuth2 client should not expose clientSecret in prepared config")
	}
	if err := handler.EncryptSensitiveFields(prepared); err != nil {
		t.Fatalf("encrypt sensitive fields: %v", err)
	}
	if _, ok := prepared["clientSecret"]; ok {
		t.Fatalf("public OAuth2 client should not persist clientSecret")
	}

	app := &model.Application{
		ProtocolType:   string(model.ProtocolTypeOAuth2),
		IsActive:       true,
		ProtocolConfig: prepared,
	}
	if err := handler.ValidateClient(app, ""); err != nil {
		t.Fatalf("public OAuth2 client should validate without secret: %v", err)
	}
	if err := handler.ValidateClient(app, "leaked-secret"); err == nil {
		t.Fatalf("public OAuth2 client should reject provided secret")
	}
}
