// Package extension defines the small composition surface shared by editions.
package extension

import (
	"cnb.cool/mliev/iam/mliev-iam/pkg/firstparty"
	"context"

	http "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"gorm.io/gorm"
)

// ResponseCodeKey exposes response metadata without exposing response bodies.
const ResponseCodeKey = "iam.response.code"

type Capabilities struct {
	Edition  string          `json:"edition"`
	Version  string          `json:"version"`
	Features map[string]bool `json:"features"`
}

// Module is composed at build time. Startup runs after core migrations and
// before workers or HTTP can accept requests. Module functions must be safe
// for concurrent HTTP requests after Startup completes.
type Module struct {
	StaticDirs        []string
	FirstPartyClients []firstparty.Client
	Name              string
	Routes            func(http.RouterInterface)
	Middleware        []http.HandlerFunc
	Startup           func(context.Context, *gorm.DB, string) error
	Capabilities      func(context.Context) map[string]bool
}
