package service

import (
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
)

func TestRecordSuccessfulLoginUpdatesUserStats(t *testing.T) {
	userService, db := setupCodeLoginUserService(t)
	user := &model.User{
		Username:    "login-stats-user",
		LoginCount:  4,
		LastLoginIP: "192.0.2.1",
	}
	createCodeLoginUser(t, db, user)
	startedAt := time.Now()

	require.NoError(t, userService.RecordSuccessfulLogin(user.ID, "203.0.113.10"))
	require.NoError(t, userService.RecordSuccessfulLogin(user.ID, "203.0.113.11"))

	reloaded, err := dao.NewUserDao(db).GetByID(user.ID)
	require.NoError(t, err)
	require.Equal(t, int64(6), reloaded.LoginCount)
	require.NotNil(t, reloaded.LastLoginAt)
	require.False(t, reloaded.LastLoginAt.Before(startedAt))
	require.Equal(t, "203.0.113.11", reloaded.LastLoginIP)
}
