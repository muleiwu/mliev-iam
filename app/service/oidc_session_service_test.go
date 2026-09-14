package service

import (
	"errors"
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

func TestApplyOIDCRegistrationChoice(t *testing.T) {
	userID := uint(42)
	baseSession := func() *OIDCSession {
		return &OIDCSession{
			Prompt:                      "create",
			CsrfToken:                   "csrf-1",
			RegistrationCandidateUserID: &userID,
		}
	}

	t.Run("continue registration rotates csrf", func(t *testing.T) {
		session := baseSession()
		deleteSession, err := applyOIDCRegistrationChoice(
			session, "csrf-1", OIDCRegistrationChoiceCreate, nil,
		)
		if err != nil || deleteSession {
			t.Fatalf("apply create choice: delete=%v err=%v", deleteSession, err)
		}
		if session.RegistrationDecision != OIDCRegistrationChoiceCreate || session.CsrfToken == "csrf-1" {
			t.Fatalf("choice was not persisted or csrf was not rotated: %+v", session)
		}
		if _, err := applyOIDCRegistrationChoice(session, session.CsrfToken, OIDCRegistrationChoiceCreate, nil); !errors.Is(err, ErrOIDCRegistrationChoiceResolved) {
			t.Fatalf("replayed choice error=%v, want resolved", err)
		}
	})

	t.Run("current account must match candidate", func(t *testing.T) {
		session := baseSession()
		otherUserID := uint(7)
		if _, err := applyOIDCRegistrationChoice(session, "csrf-1", OIDCRegistrationChoiceCurrent, &otherUserID); !errors.Is(err, ErrOIDCRegistrationUserChanged) {
			t.Fatalf("mismatched user error=%v", err)
		}
	})

	t.Run("current account choice rotates csrf", func(t *testing.T) {
		session := baseSession()
		deleteSession, err := applyOIDCRegistrationChoice(
			session, "csrf-1", OIDCRegistrationChoiceCurrent, &userID,
		)
		if err != nil || deleteSession {
			t.Fatalf("apply current choice: delete=%v err=%v", deleteSession, err)
		}
		if session.RegistrationDecision != OIDCRegistrationChoiceCurrent || session.CsrfToken == "csrf-1" {
			t.Fatalf("current choice was not persisted or csrf was not rotated: %+v", session)
		}
	})

	t.Run("cancel consumes the session", func(t *testing.T) {
		session := baseSession()
		deleteSession, err := applyOIDCRegistrationChoice(
			session, "csrf-1", OIDCRegistrationChoiceCancel, nil,
		)
		if err != nil || !deleteSession {
			t.Fatalf("apply cancel choice: delete=%v err=%v", deleteSession, err)
		}
	})

	t.Run("wrong csrf is rejected", func(t *testing.T) {
		session := baseSession()
		if _, err := applyOIDCRegistrationChoice(session, "wrong", OIDCRegistrationChoiceCreate, nil); !errors.Is(err, ErrOIDCRegistrationChoiceCSRF) {
			t.Fatalf("csrf error=%v", err)
		}
	})
}
