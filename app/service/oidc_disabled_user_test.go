package service

import (
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDisabledUserOIDCTest(t *testing.T) (*gorm.DB, *model.User, *model.Application, string, *model.OIDCToken) {
	t.Helper()

	db, err := gorm.Open(
		sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Application{},
		&model.ApplicationOIDCKey{},
		&model.OIDCToken{},
	))

	user := &model.User{
		Username:    "disabled-oidc-user",
		Status:      model.UserStatusActive,
		IsActivated: true,
	}
	require.NoError(t, db.Create(user).Error)

	applicationResponse, err := NewApplicationService(db).CreateApplication(&dto.CreateApplicationRequest{
		Name:         "disabled user oidc test",
		ProtocolType: string(model.ProtocolTypeOIDC),
		ProtocolConfig: map[string]interface{}{
			"redirectUris":     []interface{}{"https://client.example.com/callback"},
			"grantTypes":       []interface{}{"authorization_code", "refresh_token"},
			"responseTypes":    []interface{}{"code"},
			"scopes":           []interface{}{"openid", "profile"},
			"clientType":       "confidential",
			"requirePkce":      false,
			"tokenLifetime":    3600,
			"signingAlgorithm": "HS256",
		},
	}, user.ID)
	require.NoError(t, err)
	clientID, ok := applicationResponse.ProtocolConfig["clientId"].(string)
	require.True(t, ok)
	clientSecret, ok := applicationResponse.ProtocolConfig["clientSecret"].(string)
	require.True(t, ok)

	application, err := NewApplicationService(db).GetApplicationByClientID(clientID)
	require.NoError(t, err)
	token := &model.OIDCToken{
		AccessToken:  "disabled-user-access-token",
		RefreshToken: "disabled-user-refresh-token",
		ClientID:     clientID,
		UserID:       user.ID,
		Scopes:       []string{"openid", "profile"},
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(token).Error)

	require.NoError(t, db.Model(user).Updates(map[string]interface{}{
		"status":       model.UserStatusDisabled,
		"is_activated": false,
	}).Error)
	return db, user, application, clientSecret, token
}

func TestDisabledUserCannotUseHistoricalOIDCTokens(t *testing.T) {
	db, user, application, clientSecret, token := setupDisabledUserOIDCTest(t)
	authorizationService := NewOIDCAuthorizationService(db)

	_, err := authorizationService.RefreshToken(&dto.TokenRequest{
		GrantType:    string(model.GrantTypeRefreshToken),
		RefreshToken: token.RefreshToken,
		ClientID:     token.ClientID,
		ClientSecret: clientSecret,
	}, "https://issuer.example.com")
	require.ErrorIs(t, err, constants.ErrInvalidGrant)

	_, err = authorizationService.GenerateTokens(
		application,
		user.ID,
		token.Scopes,
		"",
		"https://issuer.example.com",
	)
	require.ErrorIs(t, err, constants.ErrInvalidGrant)

	_, err = NewOIDCUserInfoService(db).GetUserInfo(token.AccessToken)
	require.EqualError(t, err, "invalid_token")

	introspection, err := authorizationService.IntrospectToken(&dto.IntrospectionRequest{
		Token:        token.AccessToken,
		ClientID:     token.ClientID,
		ClientSecret: clientSecret,
	})
	require.NoError(t, err)
	require.False(t, introspection.Active)
}
