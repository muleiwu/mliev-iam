package controller

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
)

func TestPublicAppearanceAndClientConfigShareActiveStyle(t *testing.T) {
	db := registerControllerTestDatabase(t)
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	config := service.DefaultAppearanceSettings()
	config.Mode = "custom"
	config.Custom.PrimaryColor = "#127456"
	config.Custom.DarkMode = "dark"
	config.Custom.BrandName = "全局品牌"
	radius := 16
	config.Custom.ControlRadius = &radius
	settings := service.NewSettingService(db)
	if err := settings.UpdateAppearanceSettings(config); err != nil {
		t.Fatal(err)
	}
	ctx := newTestRouterContext(nil)
	AppearanceController{}.Get(ctx)
	resp := assertControllerResponse[dto.Response](t, ctx.payload)
	active := resp.Data.(dto.PublicAppearanceResponse)
	if resp.Code != 0 || active.Appearance.PrimaryColor != "#127456" {
		t.Fatalf("unexpected public config: %+v", resp)
	}
	app := &model.Application{UUID: "appearance-client", Name: "应用品牌", Icon: "/attachments/app-icon", ProtocolType: "oidc", IsActive: true}
	if err := db.Create(app).Error; err != nil {
		t.Fatal(err)
	}
	ctx = newTestRouterContext(map[string]string{"client_id": app.UUID})
	OIDCAuthFlowController{}.GetClientConfig(ctx)
	resp = assertControllerResponse[dto.Response](t, ctx.payload)
	client := resp.Data.(ClientConfigResponse)
	if client.Appearance.PrimaryColor != "#127456" || client.Appearance.BorderRadius != "16px" || client.Appearance.DarkMode != "dark" {
		t.Fatalf("client style mismatch: %+v", client)
	}
	if client.AppName != app.Name || client.Logo != app.Icon {
		t.Fatal("application identity must be preserved")
	}
	config.Mode = "default"
	if err := settings.UpdateAppearanceSettings(config); err != nil {
		t.Fatal(err)
	}
	ctx = newTestRouterContext(nil)
	AppearanceController{}.Get(ctx)
	resp = assertControllerResponse[dto.Response](t, ctx.payload)
	if resp.Data.(dto.PublicAppearanceResponse).Appearance.BrandName != "" {
		t.Fatal("inactive draft leaked")
	}
}
