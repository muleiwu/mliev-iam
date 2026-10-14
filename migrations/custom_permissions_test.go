package migrations

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func assertCustomPermissionSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, code := range []string{"Test:read", "test:read"} {
		row := model.CustomPermission{Code: code, Name: "Test", Enabled: true}
		require.NoError(t, db.Create(&row).Error, "scope codes must be case-sensitive on every database")
	}
	require.Error(t, db.Create(&model.CustomPermission{Code: "Test:read", Name: "Duplicate"}).Error)
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	for _, bounds := range [][2]*time.Time{{&now, nil}, {nil, &now}, {&now, &earlier}} {
		err := db.Create(&model.CustomPermissionGrant{PermissionID: 1, SubjectType: "everyone", StartsAt: bounds[0], EndsAt: bounds[1]}).Error
		require.Error(t, err, "database must reject partial and reversed ranges")
	}
	require.NoError(t, db.Create(&model.CustomPermissionGrant{PermissionID: 1, SubjectType: "everyone"}).Error)
	require.NoError(t, db.Create(&model.CustomPermissionGrant{PermissionID: 1, SubjectType: "everyone", StartsAt: &earlier, EndsAt: &now}).Error)
}
