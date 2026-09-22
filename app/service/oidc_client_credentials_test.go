package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestClientCredentialsCanIssueRepeatedlyAcrossClients(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:client-credentials-repeat?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Application{}, &model.ApplicationOIDCKey{}, &model.OIDCToken{}))

	creator := &model.User{Username: "client-credentials-creator", Status: model.UserStatusActive, IsActivated: true}
	require.NoError(t, db.Create(creator).Error)
	createClient := func(name string) (string, string) {
		response, err := NewApplicationService(db).CreateApplication(&dto.CreateApplicationRequest{
			Name:         name,
			ProtocolType: string(model.ProtocolTypeOIDC),
			ProtocolConfig: map[string]interface{}{
				"redirectUris":     []interface{}{"https://client.example.test/callback"},
				"grantTypes":       []interface{}{"client_credentials"},
				"responseTypes":    []interface{}{"code"},
				"scopes":           []interface{}{"openid"},
				"clientType":       "confidential",
				"signingAlgorithm": "HS256",
			},
		}, creator.ID)
		require.NoError(t, err)
		return response.ProtocolConfig["clientId"].(string), response.ProtocolConfig["clientSecret"].(string)
	}
	clientA, secretA := createClient("machine A")
	clientB, secretB := createClient("machine B")
	svc := NewOIDCAuthorizationService(db)
	issue := func(clientID, secret string) *dto.TokenResponse {
		response, err := svc.ExchangeClientCredentials(&dto.TokenRequest{
			GrantType:    string(model.GrantTypeClientCredentials),
			ClientID:     clientID,
			ClientSecret: secret,
			Scope:        "openid",
		}, "https://issuer.example.test")
		require.NoError(t, err)
		require.NotEmpty(t, response.AccessToken)
		require.Empty(t, response.RefreshToken)
		encoded, err := json.Marshal(response)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "refresh_token")
		return response
	}

	first := issue(clientA, secretA)
	second := issue(clientA, secretA)
	issue(clientB, secretB)
	introspect := func() bool {
		response, err := svc.IntrospectToken(&dto.IntrospectionRequest{
			Token:        first.AccessToken,
			ClientID:     clientA,
			ClientSecret: secretA,
		})
		require.NoError(t, err)
		return response.Active
	}
	require.True(t, introspect())
	require.NoError(t, svc.RevokeToken(&dto.RevocationRequest{
		Token:        first.AccessToken,
		ClientID:     clientA,
		ClientSecret: secretA,
	}))
	require.False(t, introspect(), "撤销后的机器令牌应立即变为 inactive")

	tokenDAO := dao.NewOIDCTokenDAO(db)
	revokedToken, err := tokenDAO.GetByAccessToken(first.AccessToken)
	require.NoError(t, err)
	require.True(t, revokedToken.IsRevoked)
	stored, err := tokenDAO.GetByAccessToken(second.AccessToken)
	require.NoError(t, err)
	require.Nil(t, stored.RefreshToken)
	stored.IsRevoked = true
	require.NoError(t, tokenDAO.Update(stored))
	var nullCount int64
	require.NoError(t, db.Model(&model.OIDCToken{}).Where("refresh_token IS NULL").Count(&nullCount).Error)
	require.EqualValues(t, 3, nullCount)
	issue(clientA, secretA)

	refresh := "unique-refresh-token"
	for i := 0; i < 2; i++ {
		token := &model.OIDCToken{
			AccessToken:  fmt.Sprintf("user-token-%d", i),
			ClientID:     clientA,
			UserID:       creator.ID,
			RefreshToken: &refresh,
			ExpiresAt:    time.Now().Add(time.Hour),
		}
		err := db.Create(token).Error
		if i == 0 {
			require.NoError(t, err)
		} else {
			require.Error(t, err, "AutoMigrate must retain non-NULL refresh token uniqueness")
		}
	}
	require.ErrorIs(t, svc.RevokeToken(&dto.RevocationRequest{
		Token:        refresh,
		ClientID:     clientB,
		ClientSecret: secretB,
	}), constants.ErrInvalidGrant)
	var revoked bool
	require.NoError(t, db.Model(&model.OIDCToken{}).Select("is_revoked").Where("refresh_token = ?", refresh).Scan(&revoked).Error)
	require.False(t, revoked, "其他客户端不能撤销该刷新令牌")
	require.NoError(t, svc.RevokeToken(&dto.RevocationRequest{
		Token:        refresh,
		ClientID:     clientA,
		ClientSecret: secretA,
	}))
	require.NoError(t, db.Model(&model.OIDCToken{}).Select("is_revoked").Where("refresh_token = ?", refresh).Scan(&revoked).Error)
	require.True(t, revoked)
}
