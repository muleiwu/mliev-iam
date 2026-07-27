package service

import (
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
)

func TestResetAuthenticationStatePreservesAuthorizationRequest(t *testing.T) {
	userID := uint(42)
	authTime := time.Now().Add(-time.Minute)
	session := &OIDCSession{
		SessionID:           "session-1",
		ClientID:            "client-1",
		RedirectURI:         "https://client.example/callback",
		Scope:               "openid profile",
		State:               "state-1",
		Nonce:               "nonce-1",
		ResponseType:        "code",
		CodeChallenge:       "challenge-1",
		CodeChallengeMethod: "S256",
		UserID:              &userID,
		IsAuthenticated:     true,
		IsConsentGranted:    true,
		AuthTime:            authTime,
		Error: &dto.AuthorizationFailure{
			Code:        "access_denied",
			Description: "denied",
			RedirectURL: "https://client.example/callback?error=access_denied",
		},
		CsrfToken: "old-csrf-token",
	}

	if err := resetAuthenticationState(session); err != nil {
		t.Fatalf("reset authentication state: %v", err)
	}

	if session.UserID != nil || session.IsAuthenticated || session.IsConsentGranted {
		t.Fatalf("authentication state was not cleared: %+v", session)
	}
	if !session.AuthTime.IsZero() {
		t.Fatalf("auth time = %v, want zero", session.AuthTime)
	}
	if session.Error != nil {
		t.Fatalf("terminal error was not cleared: %+v", session.Error)
	}
	if session.CsrfToken == "" || session.CsrfToken == "old-csrf-token" {
		t.Fatalf("csrf token was not rotated: %q", session.CsrfToken)
	}
	if session.ClientID != "client-1" || session.RedirectURI != "https://client.example/callback" ||
		session.Scope != "openid profile" || session.State != "state-1" || session.Nonce != "nonce-1" ||
		session.ResponseType != "code" || session.CodeChallenge != "challenge-1" ||
		session.CodeChallengeMethod != "S256" {
		t.Fatalf("authorization request was changed: %+v", session)
	}
}
