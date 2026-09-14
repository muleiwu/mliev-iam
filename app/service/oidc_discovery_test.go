package service

import "testing"

func TestSupportedOIDCPromptsIncludeLoginAndRegistration(t *testing.T) {
	wanted := map[string]bool{"login": false, "create": false}
	values := SupportedOIDCPromptValues()
	for _, value := range values {
		if _, ok := wanted[value]; ok {
			wanted[value] = true
		}
	}
	for value, found := range wanted {
		if !found {
			t.Fatalf("supported prompts do not contain %q: %#v", value, values)
		}
	}
}
