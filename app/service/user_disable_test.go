package service

import (
	"fmt"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserDisableTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(
		sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.LoginSession{}, &model.AuthorizationCode{}, &model.User{}, &model.OIDCToken{}))
	return db
}

func createUserWithOIDCToken(t *testing.T, db *gorm.DB, suffix string) (*model.User, *model.OIDCToken) {
	t.Helper()

	user := &model.User{
		Username:    "disable-user-" + suffix,
		Status:      model.UserStatusActive,
		IsActivated: true,
	}
	require.NoError(t, db.Create(user).Error)

	token := &model.OIDCToken{
		AccessToken:  "access-" + suffix,
		RefreshToken: "refresh-" + suffix,
		ClientID:     "client-" + suffix,
		UserID:       user.ID,
		Scopes:       []string{"openid"},
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(token).Error)
	return user, token
}

func requireUserAccessRevoked(t *testing.T, db *gorm.DB, userID, tokenID uint) *model.User {
	t.Helper()

	user, err := dao.NewUserDao(db).GetByID(userID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusDisabled, user.Status)
	require.False(t, user.IsActivated)
	require.NotNil(t, user.SessionRevokedAt)

	var token model.OIDCToken
	require.NoError(t, db.First(&token, tokenID).Error)
	require.True(t, token.IsRevoked)
	return user
}

func TestUpdateUserStatusRevokesAllUserAccess(t *testing.T) {
	db := setupUserDisableTestDB(t)
	user, token := createUserWithOIDCToken(t, db, "status")
	userService := NewUserService(db)

	require.NoError(t, userService.UpdateUserStatus(user.ID, &dto.UserStatusUpdateRequest{
		Status: model.UserStatusDisabled,
	}))
	disabled := requireUserAccessRevoked(t, db, user.ID, token.ID)
	revokedAt := *disabled.SessionRevokedAt

	require.NoError(t, userService.UpdateUserStatus(user.ID, &dto.UserStatusUpdateRequest{
		Status: model.UserStatusActive,
	}))
	reactivated, err := dao.NewUserDao(db).GetByID(user.ID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusActive, reactivated.Status)
	require.True(t, reactivated.IsActivated)
	require.NotNil(t, reactivated.SessionRevokedAt)
	require.True(t, reactivated.SessionRevokedAt.Equal(revokedAt))
}

func TestUpdateUserEditCannotBypassSessionRevocation(t *testing.T) {
	db := setupUserDisableTestDB(t)
	user, token := createUserWithOIDCToken(t, db, "edit")
	status := model.UserStatusDisabled
	activated := true

	_, err := NewUserService(db).UpdateUser(user.ID, &dto.UpdateUserRequest{
		Status:      &status,
		IsActivated: &activated,
	})

	require.NoError(t, err)
	requireUserAccessRevoked(t, db, user.ID, token.ID)
}

func TestBatchDisableRevokesEveryUserAccess(t *testing.T) {
	db := setupUserDisableTestDB(t)
	userService := NewUserService(db)
	var userIDs []uint
	var tokenIDs []uint
	for index := 1; index <= 2; index++ {
		user, token := createUserWithOIDCToken(t, db, fmt.Sprintf("batch-%d", index))
		userIDs = append(userIDs, user.ID)
		tokenIDs = append(tokenIDs, token.ID)
	}

	require.NoError(t, userService.BatchOperation(&dto.BatchUserRequest{
		UserIDs: userIDs,
		Action:  "disable",
	}))

	for index, userID := range userIDs {
		requireUserAccessRevoked(t, db, userID, tokenIDs[index])
	}
}
