package controller

import (
	"errors"
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
)

func TestNormalizeAndValidateOIDCPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prompt  string
		want    string
		wantErr string
	}{
		{name: "empty prompt", prompt: "", want: ""},
		{name: "login", prompt: "login", want: "login"},
		{name: "create", prompt: "create", want: "create"},
		{name: "normalizes spaces and duplicates", prompt: "  login   consent login ", want: "login consent"},
		{name: "none conflict", prompt: "none login", wantErr: "prompt=none"},
		{name: "create conflict", prompt: "create login", wantErr: "prompt=create"},
		{name: "unsupported value", prompt: "signup", wantErr: "不支持的 prompt 值"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeAndValidateOIDCPrompt(tt.prompt)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("prompt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptBypassesSSOForLoginAndCreate(t *testing.T) {
	for _, prompt := range []string{"login", "create"} {
		if !promptBypassesSSO(prompt) {
			t.Fatalf("prompt=%q must bypass SSO", prompt)
		}
	}
	for _, prompt := range []string{"", "consent"} {
		if promptBypassesSSO(prompt) {
			t.Fatalf("prompt=%q must retain the normal SSO behavior", prompt)
		}
	}
}

func TestRegistrationUnavailableFailurePreservesStateAndRedirectQuery(t *testing.T) {
	session := oidcRegistrationTestSession()
	adminApp := &model.Application{
		UUID:        service.AdminConsoleApplicationUUID,
		IsProtected: true,
	}
	failure, err := oidcRegistrationUnavailableFailure(adminApp, session)
	if err != nil {
		t.Fatalf("new registration failure: %v", err)
	}
	if failure == nil {
		t.Fatal("expected the disabled application to reject registration")
	}
	if failure.Code != "invalid_request" || failure.Description != oidcRegistrationUnavailableDescription {
		t.Fatalf("unexpected failure: %+v", failure)
	}
	if !strings.Contains(failure.RedirectURL, "tenant=alpha") ||
		!strings.Contains(failure.RedirectURL, "state=state-1") ||
		!strings.Contains(failure.RedirectURL, "error=invalid_request") {
		t.Fatalf("redirect URL did not preserve the authorization context: %s", failure.RedirectURL)
	}

	regularApp := &model.Application{UUID: "client-a"}
	failure, err = oidcRegistrationUnavailableFailure(regularApp, session)
	if err != nil || failure != nil {
		t.Fatalf("regular applications should retain registration: failure=%+v err=%v", failure, err)
	}
}

func oidcRegistrationTestSession() *service.OIDCSession {
	return &service.OIDCSession{
		RedirectURI: "https://client.example/callback?tenant=alpha",
		State:       "state-1",
		Prompt:      "create",
	}
}

func TestValidateOIDCRegistrationChoiceHonorsPersistedDecision(t *testing.T) {
	userID := uint(42)
	confirmed := &service.OIDCSession{
		Prompt:                      "create",
		RegistrationCandidateUserID: &userID,
		RegistrationDecision:        service.OIDCRegistrationChoiceCreate,
	}
	if err := validateOIDCRegistrationChoice(nil, confirmed); err != nil {
		t.Fatalf("confirmed registration should be allowed: %v", err)
	}

	resolved := &service.OIDCSession{
		Prompt:                      "create",
		RegistrationCandidateUserID: &userID,
		RegistrationDecision:        service.OIDCRegistrationChoiceCurrent,
	}
	if err := validateOIDCRegistrationChoice(nil, resolved); !errors.Is(err, service.ErrOIDCRegistrationChoiceResolved) {
		t.Fatalf("current-account decision should block registration, got %v", err)
	}
}
