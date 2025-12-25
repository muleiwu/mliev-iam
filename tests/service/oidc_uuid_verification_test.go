package service

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	jwtLib "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UUID regex pattern for validation
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TestIDTokenSubClaimUsesUUID verifies that ID Token sub claim uses UUID
func TestIDTokenSubClaimUsesUUID(t *testing.T) {
	// Create a test user with UUID
	testUUID := "01936f8a-1234-7abc-8def-0123456789ab"
	user := &model.User{
		ID:       1,
		UUID:     testUUID,
		Username: "testuser",
		RealName: "Test User",
		Nickname: "tester",
	}

	// Create OIDC ID Token service with HMAC for testing
	hmacSecret := []byte("test-secret-key-for-testing-only")
	svc := service.NewOIDCIDTokenServiceWithHMAC(hmacSecret, "test-key-id", "https://test-issuer.com", 3600)

	// Generate ID Token
	authTime := time.Now()
	idToken, err := svc.GenerateIDToken(user, "test-client-id", []string{"openid", "profile"}, "test-nonce", &authTime)
	require.NoError(t, err)
	assert.NotEmpty(t, idToken)

	// Verify the token contains the UUID as sub claim
	// Parse the token to verify claims
	claims, err := parseJWTClaims(idToken, hmacSecret)
	require.NoError(t, err)

	// Verify sub claim equals user UUID
	sub, ok := claims["sub"].(string)
	require.True(t, ok, "sub claim should be a string")
	assert.Equal(t, testUUID, sub, "ID Token sub claim should be user UUID")

	// Verify sub is a valid UUID format
	assert.True(t, uuidRegex.MatchString(sub), "sub claim should be a valid UUID format")
}

// TestAccessTokenSubClaimUsesUUID verifies that Access Token sub claim uses UUID
func TestAccessTokenSubClaimUsesUUID(t *testing.T) {
	// Create a test user with UUID
	testUUID := "01936f8a-5678-7abc-8def-0123456789ab"
	email := "test@example.com"
	user := &model.User{
		ID:       2,
		UUID:     testUUID,
		Username: "testuser2",
		RealName: "Test User 2",
		Email:    &email,
	}

	// Create OIDC ID Token service with HMAC for testing
	hmacSecret := []byte("test-secret-key-for-testing-only")
	svc := service.NewOIDCIDTokenServiceWithHMAC(hmacSecret, "test-key-id", "https://test-issuer.com", 3600)

	// Generate Access Token
	accessToken, err := svc.GenerateAccessToken(user, "test-client-id", []string{"openid", "profile", "email"})
	require.NoError(t, err)
	assert.NotEmpty(t, accessToken)

	// Parse the token to verify claims
	claims, err := parseJWTClaims(accessToken, hmacSecret)
	require.NoError(t, err)

	// Verify sub claim equals user UUID
	sub, ok := claims["sub"].(string)
	require.True(t, ok, "sub claim should be a string")
	assert.Equal(t, testUUID, sub, "Access Token sub claim should be user UUID")

	// Verify sub is a valid UUID format
	assert.True(t, uuidRegex.MatchString(sub), "sub claim should be a valid UUID format")
}

// TestUserInfoResponseUsesUUID verifies that UserInfo response uses UUID for sub and id fields
func TestUserInfoResponseUsesUUID(t *testing.T) {
	// Create a test user with UUID
	testUUID := "01936f8a-9abc-7def-8012-3456789abcde"
	email := "userinfo@example.com"
	user := &model.User{
		ID:        3,
		UUID:      testUUID,
		Username:  "userinfotest",
		RealName:  "UserInfo Test",
		Nickname:  "uitest",
		Email:     &email,
		UpdatedAt: time.Now(),
	}

	// Create OIDCUserInfoService and test BuildUserInfoResponse
	svc := &service.OIDCUserInfoService{}

	// Build UserInfo response
	scopes := []string{"openid", "profile", "email"}
	response := svc.BuildUserInfoResponse(user, scopes)

	// Verify sub field equals user UUID
	assert.Equal(t, testUUID, response.Sub, "UserInfo sub field should be user UUID")

	// Verify id field equals user UUID
	assert.Equal(t, testUUID, response.Id, "UserInfo id field should be user UUID")

	// Verify sub and id are the same
	assert.Equal(t, response.Sub, response.Id, "UserInfo sub and id should be equal")

	// Verify sub is a valid UUID format
	assert.True(t, uuidRegex.MatchString(response.Sub), "sub field should be a valid UUID format")
}

// parseJWTClaims parses JWT token and returns claims map
func parseJWTClaims(tokenString string, secret []byte) (map[string]any, error) {
	token, err := jwtLib.Parse(tokenString, func(token *jwtLib.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwtLib.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	return claims, nil
}
