package service

import (
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSocialProviderTestService(t *testing.T) *SocialProviderService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.SocialProvider{}, &model.UserSocialAccount{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return NewSocialProviderService(db, nil)
}

func TestSocialProviderDisabledTemplateAllowsMissingConfig(t *testing.T) {
	svc := newSocialProviderTestService(t)

	resp, err := svc.CreateProvider(&dto.CreateSocialProviderRequest{
		Name:       "QQ 互联",
		Identifier: "qq",
		Type:       model.ProviderTypeQQ,
		Status:     model.ProviderStatusDisabled,
	})
	if err != nil {
		t.Fatalf("disabled template should be created without credentials: %v", err)
	}
	if resp.Status != model.ProviderStatusDisabled {
		t.Fatalf("expected disabled status, got %d", resp.Status)
	}
}

func TestSocialProviderEnableRequiresCompleteConfig(t *testing.T) {
	svc := newSocialProviderTestService(t)

	resp, err := svc.CreateProvider(&dto.CreateSocialProviderRequest{
		Name:       "QQ 互联",
		Identifier: "qq",
		Type:       model.ProviderTypeQQ,
		Status:     model.ProviderStatusDisabled,
	})
	if err != nil {
		t.Fatalf("create disabled provider: %v", err)
	}

	enabled := model.ProviderStatusEnabled
	_, err = svc.UpdateProvider(resp.ID, &dto.UpdateSocialProviderRequest{Status: &enabled})
	if err == nil {
		t.Fatal("expected enabling incomplete QQ provider to fail")
	}
	if !strings.Contains(err.Error(), "QQ AppID") {
		t.Fatalf("expected QQ AppID validation error, got %v", err)
	}

	if err := svc.BatchOperation(&dto.SocialProviderBatchRequest{
		IDs:       []uint{resp.ID},
		Operation: "enable",
	}); err == nil {
		t.Fatal("expected batch enable to validate incomplete provider")
	}
}

func TestSocialProviderEnabledConfigsValidate(t *testing.T) {
	svc := newSocialProviderTestService(t)

	cases := []struct {
		name     string
		provider *model.SocialProvider
	}{
		{
			name: "github",
			provider: &model.SocialProvider{
				Type:         model.ProviderTypeGithub,
				ClientID:     "github-client",
				ClientSecret: "github-secret",
				RedirectURL:  "https://iam.example.com/social/callback/github",
				Status:       model.ProviderStatusEnabled,
			},
		},
		{
			name: "google",
			provider: &model.SocialProvider{
				Type:         model.ProviderTypeGoogle,
				ClientID:     "google-client",
				ClientSecret: "google-secret",
				RedirectURL:  "https://iam.example.com/social/callback/google",
				Status:       model.ProviderStatusEnabled,
			},
		},
		{
			name: "qq",
			provider: &model.SocialProvider{
				Type:         model.ProviderTypeQQ,
				ClientID:     "qq-app-id",
				ClientSecret: "qq-app-key",
				RedirectURL:  "https://iam.example.com/social/callback/qq",
				Status:       model.ProviderStatusEnabled,
			},
		},
		{
			name: "wechat",
			provider: &model.SocialProvider{
				Type:         model.ProviderTypeWeChat,
				ClientID:     "wechat-app-id",
				ClientSecret: "wechat-app-secret",
				RedirectURL:  "https://iam.example.com/social/callback/wechat",
				Status:       model.ProviderStatusEnabled,
			},
		},
		{
			name: "wechat_miniprogram",
			provider: &model.SocialProvider{
				Type:         model.ProviderTypeWeChatMiniProgram,
				ClientID:     "mini-app-id",
				ClientSecret: "mini-app-secret",
				Status:       model.ProviderStatusEnabled,
			},
		},
		{
			name: "oidc",
			provider: &model.SocialProvider{
				Type:         model.ProviderTypeOIDC,
				ClientID:     "oidc-client",
				ClientSecret: "oidc-secret",
				AuthURL:      "https://accounts.example.com/oauth2/authorize",
				TokenURL:     "https://accounts.example.com/oauth2/token",
				UserInfoURL:  "https://accounts.example.com/oauth2/userinfo",
				RedirectURL:  "https://iam.example.com/social/callback/oidc",
				Status:       model.ProviderStatusEnabled,
				Attributes:   model.JSONObject{"issuer": "https://accounts.example.com"},
			},
		},
		{
			name: "generic",
			provider: &model.SocialProvider{
				Type:         model.ProviderTypeGeneric,
				ClientID:     "oauth-client",
				ClientSecret: "oauth-secret",
				AuthURL:      "https://oauth.example.com/authorize",
				TokenURL:     "https://oauth.example.com/token",
				UserInfoURL:  "https://oauth.example.com/me",
				RedirectURL:  "https://iam.example.com/social/callback/generic-oauth2",
				Status:       model.ProviderStatusEnabled,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.validateProviderConfig(tt.provider); err != nil {
				t.Fatalf("expected valid config: %v", err)
			}
		})
	}
}
