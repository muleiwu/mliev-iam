package controller

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	"github.com/stretchr/testify/require"
)

func TestLoginConnectionsUseBoundApplication(t *testing.T) {
	f := newAccountAuthFixture(t)
	require.NoError(t, f.db.AutoMigrate(&model.IdentityConnection{}))
	router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
	router.GET("/api/identity-connections", ExternalAuthController{}.GetConnections)
	global := model.IdentityConnection{Name: "global", Identifier: "global", Category: "social", ProviderType: "github", Capabilities: model.StringList{"authentication"}, Status: "enabled", ShowGlobally: true}
	mini := model.IdentityConnection{Name: "mini", Identifier: "mini", Category: "social", ProviderType: "wechat_miniprogram", Capabilities: model.StringList{"authentication"}, Status: "enabled"}
	require.NoError(t, f.db.Create(&global).Error)
	require.NoError(t, f.db.Create(&mini).Error)
	app := newConsentTestApplication(t, f)
	_, err := service.NewApplicationService(f.db).UpdateApplication(app.ID, &dto.UpdateApplicationRequest{LoginConnections: &model.LoginConnectionConfig{Mode: "custom", ConnectionIDs: []uint{mini.ID}}})
	require.NoError(t, err)
	jar := accountTestJar(t)
	session := newConsentTestSession(t, f, jar, app.UUID, "openid", "")
	path := "/api/identity-connections?purpose=login&session_id=" + session.SessionID
	for _, test := range []struct {
		path string
		want []string
	}{
		{"/api/identity-connections", []string{"global", "mini"}},
		{"/api/identity-connections?purpose=login", []string{"global"}},
		{path + "&client_id=untrusted", []string{"mini"}},
	} {
		rec := f.request(t, jar, "GET", test.path, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var response struct {
			Data []dto.PublicIdentityConnectionInfo `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		got := []string{}
		for _, item := range response.Data {
			got = append(got, item.Identifier)
		}
		require.Equal(t, test.want, got)
	}
	otherJar := accountTestJar(t)
	require.Equal(t, http.StatusUnauthorized, f.request(t, otherJar, "GET", path, nil).Code)
	require.Equal(t, http.StatusUnauthorized, f.request(t, jar, "GET", "/api/identity-connections?purpose=login&session_id=missing", nil).Code)
	require.Equal(t, http.StatusBadRequest, f.request(t, jar, "GET", "/api/identity-connections?purpose=unknown", nil).Code)
	require.NoError(t, f.db.Model(&model.Application{}).Where("id = ?", app.ID).Update("is_active", false).Error)
	require.Equal(t, http.StatusForbidden, f.request(t, jar, "GET", path, nil).Code)
	f.clock.FastForward(24 * time.Hour)
	require.Equal(t, http.StatusUnauthorized, f.request(t, jar, "GET", path, nil).Code)
}
