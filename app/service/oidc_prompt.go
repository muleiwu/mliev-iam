package service

var supportedOIDCPromptValues = []string{
	"none",
	"login",
	"consent",
	"select_account",
	"create",
}

// SupportedOIDCPromptValues returns the values accepted by the authorization
// endpoint and advertised through OIDC Discovery.
func SupportedOIDCPromptValues() []string {
	return append([]string(nil), supportedOIDCPromptValues...)
}

func SupportsOIDCPromptValue(value string) bool {
	for _, supported := range supportedOIDCPromptValues {
		if value == supported {
			return true
		}
	}
	return false
}
