package admin

import "testing"

func TestSafeAdminReturnToRejectsExternalAndFragmentTargets(t *testing.T) {
	for _, value := range []string{
		"https://evil.example/steal",
		"//evil.example/steal",
		"/dashboard#https://evil.example",
		"/dashboard\\evil",
	} {
		if got := safeAdminReturnTo(value); got != "/dashboard" {
			t.Fatalf("safeAdminReturnTo(%q)=%q, want default", value, got)
		}
	}
	if got := safeAdminReturnTo("/application-management/application-list?page=2"); got != "/application-management/application-list?page=2" {
		t.Fatalf("valid internal target changed to %q", got)
	}
}
