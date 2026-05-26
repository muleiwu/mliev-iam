package identity

import (
	"fmt"
	"strconv"
	"strings"
)

func int64Number(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	default:
		return 0
	}
}

func joinScopes(scopes []string, sep string) string {
	parts := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		for _, part := range strings.FieldsFunc(scope, func(r rune) bool {
			return r == ',' || r == ' '
		}) {
			if part != "" {
				parts = append(parts, part)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s", strings.Join(parts, sep))
}
