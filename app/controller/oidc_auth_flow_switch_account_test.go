package controller

import (
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/service"
)

func TestValidSwitchAccountCSRF(t *testing.T) {
	session := &service.OIDCSession{CsrfToken: "csrf-token"}

	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{name: "matching token", token: "csrf-token", want: true},
		{name: "wrong token", token: "wrong-token", want: false},
		{name: "missing token", token: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validSwitchAccountCSRF(session, tt.token); got != tt.want {
				t.Fatalf("validSwitchAccountCSRF() = %v, want %v", got, tt.want)
			}
		})
	}

	if validSwitchAccountCSRF(nil, "csrf-token") {
		t.Fatal("nil session must fail CSRF validation")
	}
}
