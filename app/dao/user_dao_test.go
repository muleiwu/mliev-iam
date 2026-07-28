package dao

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserDaoListTest(t *testing.T) *UserDao {
	t.Helper()

	db, err := gorm.Open(
		sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	users := []model.User{
		{
			UUID:     "0198a73f-4ab1-7000-8000-000000000001",
			Username: "alice",
			Phone:    stringPointer("+8613800138000"),
			Status:   model.UserStatusActive,
		},
		{
			UUID:     "0198a73f-4ab1-7000-8000-000000000002",
			Username: "bob",
			Phone:    stringPointer("+8613912345678"),
			Status:   model.UserStatusDisabled,
		},
		{
			UUID:     "0198a73f-4ab1-7000-8000-000000000003",
			Username: "carol",
			Phone:    stringPointer("+8613712345678"),
			Status:   model.UserStatusActive,
		},
	}
	require.NoError(t, db.Create(&users).Error)

	return NewUserDao(db)
}

func stringPointer(value string) *string {
	return &value
}

func TestUserDaoListFiltersByPhoneFragment(t *testing.T) {
	userDao := setupUserDaoListTest(t)

	for _, query := range []string{"+8613800138000", "8000"} {
		users, total, err := userDao.List(&dto.UserQueryRequest{Phone: query})

		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Len(t, users, 1)
		require.Equal(t, "alice", users[0].Username)
	}
}

func TestUserDaoListFiltersByExactUUID(t *testing.T) {
	userDao := setupUserDaoListTest(t)

	users, total, err := userDao.List(&dto.UserQueryRequest{
		UUID: "0198a73f-4ab1-7000-8000-000000000002",
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	require.Equal(t, "bob", users[0].Username)

	users, total, err = userDao.List(&dto.UserQueryRequest{
		UUID: "0198a73f-4ab1",
	})

	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, users)
}

func TestUserDaoListCombinesFiltersAndKeepsFilteredTotal(t *testing.T) {
	userDao := setupUserDaoListTest(t)
	status := int(model.UserStatusActive)

	users, total, err := userDao.List(&dto.UserQueryRequest{
		Page:     1,
		PageSize: 1,
		Phone:    "12345678",
		Status:   &status,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	require.Equal(t, "carol", users[0].Username)
}
