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
	for _, tc := range []struct{ preset, color, theme, radius string }{
		{"minimal", "#334155", "light", "6px"}, {"brand", "#1d4ed8", "auto", "8px"}, {"midnight", "#0f766e", "dark", "8px"},
		{"paper", "#9a4c20", "light", "4px"}, {"grove", "#3f6212", "auto", "12px"}, {"graphite", "#3f4c5a", "auto", "0px"},
	} {
		config.Custom = service.DefaultAppearanceSettings().Custom
		config.Custom.Preset, config.Custom.DesktopAlignment = tc.preset, "right"
		if err := settings.UpdateAppearanceSettings(config); err != nil {
			t.Fatal(err)
		}
		ctx = newTestRouterContext(map[string]string{"client_id": app.UUID})
		OIDCAuthFlowController{}.GetClientConfig(ctx)
		resp = assertControllerResponse[dto.Response](t, ctx.payload)
		client = resp.Data.(ClientConfigResponse)
		if client.Appearance.PrimaryColor != tc.color || client.Appearance.DarkMode != tc.theme || client.Appearance.BorderRadius != tc.radius {
			t.Fatalf("%s preset overridden by legacy defaults: %+v", tc.preset, client.Appearance)
		}
		active, err := settings.GetPublicAppearance()
		if err != nil || active.Appearance.Preset != tc.preset || active.Appearance.PrimaryColor != "" || active.Appearance.ControlRadius != nil {
			t.Fatal("public settings must retain explicit overrides separately from preset defaults")
		}
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
