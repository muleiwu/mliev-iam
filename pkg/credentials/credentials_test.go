package credentials

import (
	"encoding/base64"
	"testing"
)

func TestEnvelopePurposeAndKeyIsolation(t *testing.T) {
	t.Setenv("IDENTITY_CREDENTIALS_KEYS", "")
	t.Setenv("IDENTITY_CREDENTIALS_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	sealed, err := Seal("feature.weather", []byte("test-credential"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Open("feature.weather", sealed)
	if err != nil || string(raw) != "test-credential" {
		t.Fatal("credential did not round trip")
	}
	if _, err := Open("feature.other", sealed); err == nil {
		t.Fatal("accepted incorrect purpose")
	}
	other := make([]byte, 32)
	other[0] = 1
	t.Setenv("IDENTITY_CREDENTIALS_KEY", base64.StdEncoding.EncodeToString(other))
	if _, err := Open("feature.weather", sealed); err == nil {
		t.Fatal("accepted incorrect key")
	}
}
