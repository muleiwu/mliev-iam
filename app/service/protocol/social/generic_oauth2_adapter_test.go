package social

import "testing"

func TestMapCommonUserFieldsWithAttributes(t *testing.T) {
	raw := map[string]any{
		"uid":          12345,
		"display_name": "Ada",
		"photo":        "https://example.com/avatar.png",
		"mail":         "ada@example.com",
	}
	attrs := map[string]any{
		"openIdField":   "uid",
		"nicknameField": "display_name",
		"avatarField":   "photo",
		"emailField":    "mail",
	}

	user := MapCommonUserFieldsWithAttributes(raw, attrs)
	if user.OpenID != "12345" {
		t.Fatalf("OpenID = %q", user.OpenID)
	}
	if user.Nickname != "Ada" {
		t.Fatalf("Nickname = %q", user.Nickname)
	}
	if user.Avatar != "https://example.com/avatar.png" {
		t.Fatalf("Avatar = %q", user.Avatar)
	}
	if user.Email != "ada@example.com" {
		t.Fatalf("Email = %q", user.Email)
	}
}
