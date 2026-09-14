package service

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOIDCPublicClientTokenDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.LoginSession{},
		&model.User{},
		&model.Application{},
		&model.ApplicationOIDCKey{},
		&model.AuthorizationCode{},
		&model.OIDCToken{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestPublicClientAuthorizationCodePKCEExchangeWithoutClientSecret(t *testing.T) {
	db := setupOIDCPublicClientTokenDB(t)

	user := &model.User{
		Username:    "oidc-public-user",
		RealName:    "OIDC Public User",
		Status:      model.UserStatusActive,
		IsActivated: true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	appResp, err := NewApplicationService(db).CreateApplication(&dto.CreateApplicationRequest{
		Name:         "public pkce app",
		ProtocolType: string(model.ProtocolTypeOIDC),
		ProtocolConfig: map[string]interface{}{
			"redirectUris":     []interface{}{"https://spa.example.com/callback"},
			"grantTypes":       []interface{}{"authorization_code", "refresh_token"},
			"responseTypes":    []interface{}{"code"},
			"scopes":           []interface{}{"openid", "profile"},
			"clientType":       "public",
			"requirePkce":      false,
			"tokenLifetime":    3600,
			"signingAlgorithm": "HS256",
		},
	}, user.ID)
	if err != nil {
		t.Fatalf("create public app: %v", err)
	}
	if _, ok := appResp.ProtocolConfig["clientSecret"]; ok {
		t.Fatalf("public app creation response should not include clientSecret")
	}

	clientID, ok := appResp.ProtocolConfig["clientId"].(string)
	if !ok || clientID == "" {
		t.Fatalf("created app should include clientId, got %#v", appResp.ProtocolConfig["clientId"])
	}

	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNO123456"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	login, err := NewLoginSessionService(db).Create(user.ID, "password", "192.0.2.1", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	authCode := &model.AuthorizationCode{
		LoginSessionID:      login.ID,
		Code:                "public-code-with-pkce",
		ClientID:            clientID,
		UserID:              user.ID,
		RedirectURI:         "https://spa.example.com/callback",
		Scopes:              []string{"profile"},
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}
	if err := db.Create(authCode).Error; err != nil {
		t.Fatalf("create authorization code: %v", err)
	}

	resp, err := NewOIDCAuthorizationService(db).ExchangeCodeForToken(&dto.TokenRequest{
		GrantType:    string(model.GrantTypeAuthorizationCode),
		Code:         authCode.Code,
		RedirectURI:  authCode.RedirectURI,
		ClientID:     clientID,
		ClientSecret: "",
		CodeVerifier: verifier,
	}, "https://issuer.example.com/oidc/"+clientID)
	if err != nil {
		t.Fatalf("exchange code without client_secret: %v", err)
	}
	if resp.AccessToken == "" {
		t.Fatalf("expected access token")
	}
	if resp.RefreshToken == "" {
		t.Fatalf("expected refresh token")
	}
	if resp.IDToken != "" {
		t.Fatalf("scope without openid should not generate id token in this test")
	}
}
