package controller

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/service"
)

func TestBuildLoginVerificationSessionIDIsSceneScoped(t *testing.T) {
	phoneSessionA := buildLoginVerificationSessionID("session-a", service.VerificationTypePhone, "13800138000")
	phoneSessionB := buildLoginVerificationSessionID("session-b", service.VerificationTypePhone, "13800138000")
	emailSessionA := buildLoginVerificationSessionID("session-a", service.VerificationTypeEmail, "user@example.com")

	if phoneSessionA == phoneSessionB {
		t.Fatal("different OIDC sessions must not share login verification code keys")
	}
	if phoneSessionA == emailSessionA {
		t.Fatal("phone and email login verification code keys must be isolated")
	}
	if phoneSessionA == "13800138000" {
		t.Fatal("login verification code key must include login scene and session context")
	}
}
