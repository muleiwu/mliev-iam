package service

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func loginVisibilityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupApplicationIconTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.IdentityConnection{}, &model.IdentityConnectionCredential{}, &model.IdentityConnectionHealthCheck{}))
	return db
}

func seedLoginConnection(t *testing.T, db *gorm.DB, name string, global bool) model.IdentityConnection {
	t.Helper()
	connection := model.IdentityConnection{Name: name, Identifier: name, Category: model.ConnectionCategorySocial,
		ProviderType: "wechat_miniprogram", Capabilities: model.StringList{"authentication"}, Status: model.ConnectionStatusEnabled, ShowGlobally: global}
	require.NoError(t, db.Create(&connection).Error)
	return connection
}

func TestLoginConnectionVisibility(t *testing.T) {
	db := loginVisibilityDB(t)
	global := seedLoginConnection(t, db, "global", true)
	first := seedLoginConnection(t, db, "mini-a", false)
	second := seedLoginConnection(t, db, "mini-b", false)
	enterprise := seedLoginConnection(t, db, "enterprise", true)
	require.NoError(t, db.Model(&enterprise).Updates(map[string]any{"category": "enterprise", "provider_type": "feishu"}).Error)
	connections := NewIdentityConnectionService(db)
	for _, test := range []struct {
		name     string
		settings *model.LoginConnectionConfig
		want     []string
	}{
		{"historical", nil, []string{"enterprise", "global"}},
		{"inherit", &model.LoginConnectionConfig{Mode: "inherit"}, []string{"enterprise", "global"}},
		{"first application", &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{first.ID}}, []string{"mini-a"}},
		{"second application", &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{second.ID, enterprise.ID}}, []string{"enterprise", "mini-b"}},
		{"empty", &model.LoginConnectionConfig{Mode: "custom"}, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			list, err := connections.PublicLoginList(&model.Application{LoginConnections: test.settings})
			require.NoError(t, err)
			got := []string{}
			for _, connection := range list {
				got = append(got, connection.Identifier)
			}
			require.Equal(t, test.want, got)
		})
	}
	all, err := connections.PublicList()
	require.NoError(t, err)
	require.Len(t, all, 4, "binding list includes globally hidden connections")
	require.NoError(t, db.Model(&first).Update("show_globally", true).Error)
	list, err := connections.PublicLoginList(nil)
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.NoError(t, db.Model(&global).Update("status", "disabled").Error)
	require.NoError(t, db.Model(&enterprise).Update("capabilities", model.StringList{"directory_sync"}).Error)
	require.NoError(t, db.Delete(&first).Error)
	list, err = connections.PublicLoginList(&model.Application{LoginConnections: &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{global.ID, enterprise.ID, first.ID, second.ID}}})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "mini-b", list[0].Identifier)
}

func TestLoginConnectionConfigurationPersistence(t *testing.T) {
	db := loginVisibilityDB(t)
	first := seedLoginConnection(t, db, "mini-a", false)
	second := seedLoginConnection(t, db, "mini-b", false)
	s := NewApplicationService(db)
	req := validOIDCApplicationRequest("")
	req.LoginConnections = &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{first.ID, first.ID}}
	app, err := s.CreateApplication(req, 1)
	require.NoError(t, err)
	require.Equal(t, []uint{first.ID}, app.LoginConnections.ConnectionIDs)
	app, err = s.GetApplication(app.ID)
	require.NoError(t, err)
	require.Equal(t, "custom", app.LoginConnections.Mode)
	name := "renamed"
	app, err = s.UpdateApplication(app.ID, &dto.UpdateApplicationRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, []uint{first.ID}, app.LoginConnections.ConnectionIDs)
	// Existing selections may be retained even if disabled or deleted; new ones may not.
	require.NoError(t, db.Delete(&first).Error)
	require.NoError(t, db.Model(&second).Update("status", "disabled").Error)
	_, err = s.UpdateApplication(app.ID, &dto.UpdateApplicationRequest{LoginConnections: &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{first.ID}}})
	require.NoError(t, err)
	for _, config := range []*model.LoginConnectionConfig{
		{Mode: "unknown"}, {Mode: "custom", ConnectionIDs: []uint{0}},
		{Mode: "custom", ConnectionIDs: []uint{first.ID, second.ID}},
		{Mode: "custom", ConnectionIDs: []uint{999999}},
	} {
		_, err = s.UpdateApplication(app.ID, &dto.UpdateApplicationRequest{LoginConnections: config})
		require.Error(t, err)
	}
	app, err = s.UpdateApplication(app.ID, &dto.UpdateApplicationRequest{LoginConnections: &model.LoginConnectionConfig{Mode: "custom"}})
	require.NoError(t, err)
	require.Empty(t, app.LoginConnections.ConnectionIDs)
	require.NotNil(t, app.LoginConnections.ConnectionIDs)
	app, err = s.UpdateApplication(app.ID, &dto.UpdateApplicationRequest{LoginConnections: &model.LoginConnectionConfig{Mode: "inherit"}})
	require.NoError(t, err)
	require.Equal(t, "inherit", app.LoginConnections.Mode)
}

func TestLoginConnectionNewDefaultsAndUpdate(t *testing.T) {
	db := loginVisibilityDB(t)
	s := NewIdentityConnectionService(db)
	connection, err := s.Create(&dto.CreateIdentityConnectionRequest{Name: "new", Identifier: "new", Category: "social", ProviderType: "github"})
	require.NoError(t, err)
	require.False(t, connection.ShowGlobally)
	for _, show := range []bool{true, false} {
		connection, err = s.Update(connection.ID, &dto.UpdateIdentityConnectionRequest{ShowGlobally: &show})
		require.NoError(t, err)
		require.Equal(t, show, connection.ShowGlobally)
		name := "renamed"
		connection, err = s.Update(connection.ID, &dto.UpdateIdentityConnectionRequest{Name: &name})
		require.NoError(t, err)
		require.Equal(t, show, connection.ShowGlobally)
	}
}

func TestLoginConnectionBuiltinSettingsSurviveRepair(t *testing.T) {
	for _, id := range []string{AdminConsoleApplicationUUID, AccountCenterApplicationUUID} {
		t.Run(id, func(t *testing.T) {
			db := loginVisibilityDB(t)
			connection := seedLoginConnection(t, db, "chosen", false)
			app, err := EnsureFirstPartyApplication(db, id, nil)
			require.NoError(t, err)
			settings := &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{connection.ID}}
			_, err = NewApplicationService(db).UpdateApplication(app.ID, &dto.UpdateApplicationRequest{LoginConnections: settings})
			require.NoError(t, err)
			app, err = EnsureFirstPartyApplication(db, id, nil)
			require.NoError(t, err)
			require.Equal(t, *settings, app.LoginConnectionSettings())
		})
	}
}
