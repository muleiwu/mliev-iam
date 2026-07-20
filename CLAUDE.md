# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MLiev IAM is an enterprise identity and access management system built in Go, supporting SSO via OIDC/OAuth2. Module path: `cnb.cool/mliev/iam/mliev-iam`.

**Key tech**: Go 1.25, Gin, GORM, Redis, Zap, JWT, embed.FS for static assets.

## Development Commands

### Build & Run
```bash
make all              # Build frontend + Go backend
make build-go         # Go backend only
make dev              # Dev mode: frontend watch + Go backend (debug mode)
make dev-go           # Go backend only in debug mode (mode=debug go run .)
make run              # Build frontend then run Go backend
go run .              # Run Go backend directly
```

### Frontend Build
Frontend is a pnpm monorepo under `admin-web/`. Core packages must build first.
```bash
make build-frontend   # Build all (packages + oidc + antd)
make build-packages   # Build @vben-core packages only
make build-oidc       # Build OIDC login UI → static/auth/
make build-antd       # Build admin panel → static/admin/
make install-frontend # pnpm install for frontend apps
```

### Other
```bash
go test ./...         # Run tests (no tests exist yet)
go mod tidy           # Clean up dependencies
make release          # Cross-compile via goreleaser
make clean            # Clean all build artifacts
```

### OIDC Key Generation
```bash
openssl genrsa -out keys/oidc_private_key.pem 2048
openssl rsa -in keys/oidc_private_key.pem -pubout -out keys/oidc_public_key.pem
```

## Architecture

### Assembly-Based Dependency Injection

The project uses a custom lightweight DI framework based on ordered assembly. No reflection.

**Boot sequence** (`cmd/run.go` → `config/assembly.go`):
1. Env → Config → Logger → Database → Redis → Cache (strict order)
2. Each component implements `interfaces.AssemblyInterface.Assembly()`
3. Static FS injected into config after assembly
4. Server instances created and started

**Helper singleton** (`internal/helper/`) is the service locator—provides `GetDatabase()`, `GetRedis()`, `GetLogger()`, `GetConfig()`, `GetEnv()`, `GetCache()`. Also manages a settings cache (DB-backed key-value with TTL).

### Request Handling Pattern

All HTTP handlers have the signature:
```go
func(c *gin.Context, helper interfaces.HelperInterface)
```

Handlers are registered via `deps.WrapHandler()` which injects a per-request Helper with trace ID propagation. Routes are defined in `config/autoload/router.go`.

### Layered Architecture

**Controller → Service → DAO**, all under `app/`. Controllers parse requests into DTOs (`app/dto/`), services contain business logic, DAOs handle GORM queries.

### Static Assets & Dual-Mode Serving

Static files are embedded via `//go:embed` in `main.go`. In debug mode, files are served from disk (hot reload). In release mode, served from embedded FS. The driver is selected in `internal/pkg/http_server/impl/static_handler.go`.

Frontend apps map to:
- `admin-web/apps/web-oidc` → `static/auth/` (OIDC login/register UI)
- `admin-web/apps/web-antd` → `static/admin/` (admin panel)
- `admin-web/apps/web-install` → `static/install/` (installation wizard)

### Configuration System

Config sources live in `config/autoload/*.go` as `InitConfig()` methods returning `map[string]any`. These are aggregated into an in-memory map provider accessible via `helper.GetConfig()`. Runtime settings use a DB-backed store with cache via `helper.GetSetting(key)`.

External config: `config.yaml` (copy from `config.yaml.example`).

### OIDC Implementation

Full OAuth2/OIDC provider with authorization code flow, implicit flow, PKCE support.

- **Session management**: Redis-backed with TTL (`app/service/oidc_session_service.go`)
- **Protocol abstraction**: `app/service/protocol/handler.go` defines `ProtocolHandler` interface supporting OIDC/SAML/CAS/OAuth2
- **Endpoints**: Under `/oidc/:client_id/` — authorize, token, userinfo, introspect, revoke, JWKS, discovery
- **Auth flow UI**: Separate controller (`oidc_auth_flow_controller.go`) for login/register/consent pages

### Middleware Stack

Global middleware (applied in order): CORS → InstallCheck. Auth middleware (`AuthMiddleware`, `AdminAuthMiddleware`, `OptionalAuthMiddleware`) and RBAC/DataScope middleware are applied per route group.

### RBAC & Data Permissions

Four-level data scope: All / Department / Department+Children / Self. Permission types: Menu, Button, API, Data. Middleware enforces both URL-level access and row-level data filtering.

### Graceful Restart

The app supports SIGHUP for graceful restart (re-assembly without process exit) and API-triggered reload via `internal/pkg/reload`.

## Key Conventions

- All GORM models use soft delete (`DeletedAt` field)—queries auto-filter deleted records
- Database migration is automatic via GORM AutoMigrate at startup (configured in `config/autoload/migration.go`)
- The `Application` model is protocol-agnostic with a `ProtocolType` field and JSON `ProtocolConfig`
- Handler functions are always wrapped via `WrapHandler`—never register raw `gin.HandlerFunc` directly

## Design Context

### Users
IAM administrators, security and operations teams, and developers integrating OIDC/OAuth2 applications. They work in an enterprise admin console where identity configuration must be fast to scan, precise, and safe to change.

### Brand Personality
Modern, trustworthy, clear. The interface may feel light and energetic while preserving the professionalism and predictability expected from an enterprise identity security product.

### Aesthetic Direction
Extend the existing Vben and Ant Design system. Use MLiev Identity Blue as a restrained accent with subtly blue-tinted neutrals. Support light/dark themes, narrow screens, and keyboard use. Avoid decorative gradients, glassmorphism, showy motion, and deeply nested cards.

### Design Principles
- Establish the application's identity, state, and current task before protocol details.
- Use explicit action labels, visible focus states, and actionable error recovery.
- Reuse existing design tokens and Ant Design interactions instead of creating isolated visual patterns.
- Keep optional configuration lightweight and provide dependable defaults when absent.
- Design for dark mode, keyboard access, narrow screens, long names, and reduced motion from the start.
