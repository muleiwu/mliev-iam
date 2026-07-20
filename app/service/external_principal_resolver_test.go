package service

import (
	"errors"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestExternalPrincipalResolverSyncAndLoginOrderConverge(t *testing.T) {
	tests := []struct {
		name           string
		firstDirectory bool
		first          []PrincipalIdentifierInput
		later          []PrincipalIdentifierInput
	}{
		{
			name:           "directory before login",
			firstDirectory: true,
			first: []PrincipalIdentifierInput{
				{Namespace: model.IdentifierNamespaceDirectoryUserID, Value: "tenant:user-1", Primary: true},
				{Namespace: model.IdentifierNamespaceUnionID, Value: "union-1"},
			},
			later: []PrincipalIdentifierInput{
				{Namespace: model.IdentifierNamespaceLoginSubject, Value: "login-1", Primary: true},
				{Namespace: model.IdentifierNamespaceUnionID, Value: "union-1"},
			},
		},
		{
			name: "login before directory",
			first: []PrincipalIdentifierInput{
				{Namespace: model.IdentifierNamespaceOpenID, Value: "open-2", Primary: true},
				{Namespace: model.IdentifierNamespaceUnionID, Value: "union-2"},
			},
			later: []PrincipalIdentifierInput{
				{Namespace: model.IdentifierNamespaceDirectoryUserID, Value: "tenant:user-2", Primary: true},
				{Namespace: model.IdentifierNamespaceUnionID, Value: "union-2"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, connection := openPrincipalResolverTestDB(t)
			resolver := NewExternalPrincipalResolver(db)
			var first *model.ExternalPrincipal
			var err error
			if test.firstDirectory {
				first, err = resolver.ResolveForDirectory(connection.ID, test.first, true)
			} else {
				first, err = resolver.Resolve(connection.ID, test.first, true)
			}
			require.NoError(t, err)
			var later *model.ExternalPrincipal
			if test.firstDirectory {
				later, err = resolver.Resolve(connection.ID, test.later, true)
			} else {
				later, err = resolver.ResolveForDirectory(connection.ID, test.later, true)
			}
			require.NoError(t, err)
			require.Equal(t, first.ID, later.ID)

			var principals, identifiers int64
			require.NoError(t, db.Model(&model.ExternalPrincipal{}).Count(&principals).Error)
			require.NoError(t, db.Model(&model.ExternalIdentifier{}).Count(&identifiers).Error)
			require.EqualValues(t, 1, principals)
			require.EqualValues(t, 3, identifiers, "共享别名应让登录和目录标识收敛到同一 principal")
		})
	}
}

func TestExternalPrincipalResolverRejectsBlockedAndCreatesConflictForMultipleMatches(t *testing.T) {
	db, connection := openPrincipalResolverTestDB(t)
	resolver := NewExternalPrincipalResolver(db)
	blocked, err := resolver.ResolveForDirectory(connection.ID, []PrincipalIdentifierInput{{Namespace: model.IdentifierNamespaceDirectoryUserID, Value: "tenant:blocked"}}, true)
	require.NoError(t, err)
	require.NoError(t, db.Model(blocked).Update("status", model.PrincipalStatusBlocked).Error)
	_, err = resolver.Resolve(connection.ID, []PrincipalIdentifierInput{{Namespace: model.IdentifierNamespaceDirectoryUserID, Value: "tenant:blocked"}}, false)
	require.ErrorContains(t, err, "已被停用")

	first, err := resolver.Resolve(connection.ID, []PrincipalIdentifierInput{{Namespace: model.IdentifierNamespaceLoginSubject, Value: "subject-a"}}, true)
	require.NoError(t, err)
	second, err := resolver.ResolveForDirectory(connection.ID, []PrincipalIdentifierInput{{Namespace: model.IdentifierNamespaceDirectoryUserID, Value: "tenant:user-b"}}, true)
	require.NoError(t, err)
	_, err = resolver.Resolve(connection.ID, []PrincipalIdentifierInput{
		{Namespace: model.IdentifierNamespaceLoginSubject, Value: "subject-a"},
		{Namespace: model.IdentifierNamespaceDirectoryUserID, Value: "tenant:user-b"},
	}, false)
	var conflictErr *PrincipalConflictError
	require.True(t, errors.As(err, &conflictErr))
	require.NotZero(t, conflictErr.ConflictID)
	require.NotEqual(t, first.ID, second.ID)
	var conflict model.ExternalPrincipalConflict
	require.NoError(t, db.First(&conflict, conflictErr.ConflictID).Error)
	require.Equal(t, model.PrincipalConflictPending, conflict.Status)
}

func openPrincipalResolverTestDB(t *testing.T) (*gorm.DB, *model.IdentityConnection) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.IdentityConnection{}, &model.ExternalPrincipal{},
		&model.ExternalIdentifier{}, &model.ExternalPrincipalConflict{},
	))
	connection := &model.IdentityConnection{
		Name: "Resolver", Identifier: "resolver-" + t.Name(),
		Category: model.ConnectionCategoryEnterprise, ProviderType: "feishu",
		Capabilities: model.StringList{"authentication", "directory_sync"}, Status: model.ConnectionStatusEnabled,
	}
	require.NoError(t, db.Create(connection).Error)
	return db, connection
}
