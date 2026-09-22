# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MLiev IAM is an enterprise identity and access management system built in Go, supporting SSO via OIDC/OAuth2. Module path: `cnb.cool/mliev/iam/mliev-iam`.

**Key tech**: Go 1.26, Gin, GORM, Redis, Zap, JWT, embed.FS for static assets. The HTTP stack, DI container, and process supervision come from the external framework module `cnb.cool/mliev/open/go-web` (v1.9.0); the guided installer comes from `cnb.cool/mliev/open/setup-kit` (v0.1.0).

Interactive architecture diagrams (system, OIDC flow, request lifecycle, boot assembly; bilingual): `docs/archify/`.

## Development Commands

### Build & Run
```bash
make all              # Build frontend + Go backend
make build-go         # Go backend only
make dev              # Dev mode: frontend watch + Go backend (debug mode)
go run . start        # Start the backend via the gomander process manager
```

The binary is a gomander (Cobra) process manager: `start | stop | restart | reload | status`. A bare `go run .` only prints help — the `start` subcommand is required. Note: the `dev-go`/`run` Makefile targets still invoke bare `go run .` and therefore only print help; use `mode=debug go run . start` for a debug backend.

### Frontend Build
Frontends are **two git submodules** (`admin-web/`, `frontend-oidc/`). Core packages must build first.
```bash
make build-frontend   # Build all (packages + OIDC login UI + admin panel)
make build-packages   # Build @vben-core packages only (admin-web monorepo)
make build-oidc       # Build OIDC login UI from frontend-oidc/ → static/auth/
make build-antd       # Build admin panel from admin-web/apps/web-antd → static/admin/
make install-frontend # pnpm install for frontend-oidc/ and admin-web/apps/web-antd
```
The retired `apps/web-oidc` and `apps/web-install` no longer exist; installation is handled by the embedded setup-kit wizard (UI inside web-antd).

### Other
```bash
go test ./...                              # Run unit tests
go test ./app/service/ -run TestOIDC       # Run a single test by name
go test -tags integration ./migrations/    # Migration tests via testcontainers (requires Docker)
go mod tidy                                # Clean up dependencies
make release                               # Cross-compile via goreleaser
make clean                                 # Clean all build artifacts
```

### OIDC Key Generation
```bash
openssl genrsa -out keys/oidc_private_key.pem 2048
openssl rsa -in keys/oidc_private_key.pem -pubout -out keys/oidc_public_key.pem
```

## Architecture

### Assembly-Based Dependency Injection

The project uses go-web's lightweight DI framework based on ordered assembly. No reflection. The framework source lives in the `cnb.cool/mliev/open/go-web` module (module cache), **not** in this repo.

**Boot sequence** (`main.go` → go-web `cmd.Start` → `config/app.go`):
1. `RegisterAssemblies(app.Assemblies())` — assemblies are topologically sorted by `AssemblyInterface.DependsOn()`
2. Env → Config (aggregates the `InitConfig()` providers from `config/autoload/*.go`: Base, Cache, Http, Cors, StaticFs, Database, DatabaseSafety, Redis, Push, Middleware, Router) → Logger → Database → Redis (nil client until installed) → Cache (memory before install, Redis after) → startupChecks (ensures the credential key, runs goose migrations, starts modules, sets `app.ready`) → SetupInstaller
3. Embedded static FS is injected via `config.Set("static.fs", ...)`
4. Server instances from `app.Servers()` are started — before install: setup installer + HTTP server; after install: also SyncWorker (directory sync) and LoginSessionCleanup background servers

Each component implements `interfaces.AssemblyInterface` (`{ Type() reflect.Type; DependsOn() []reflect.Type; Assembly() (any, error) }`).

**Helper singleton** (go-web `pkg/helper/`) is the service locator — provides `GetDatabase()`, `GetRedis()`, `GetLogger()`, `GetConfig()`, `GetEnv()`, `GetCache()`. Runtime settings use `SettingService` (`app/service/setting_service.go`): a DB-backed key-value store cached through `helper.GetCache()` with a 1-hour TTL (it replaces the old `internal/helper.GetSetting`, which no longer exists).

### Request Handling Pattern

All HTTP handlers have the signature:
```go
func(rc interfaces.RouterContextInterface)
```
Registering a route on `RouterInterface` auto-wraps every handler via `HttpDeps.WrapHandler`, which adapts `gin.Context` → RouterContext and injects a request-scoped logger carrying the trace ID. Business code reaches dependencies through the global `helper.Get*()` locator. Routes are defined in `config/autoload/router.go`; main groups: `/api/admin/*` (AdminAuth), `/api/user/*` (UserAuth), `/api/auth/*`, `/api/oidc/*` (auth-flow UI API), `/external-auth/:identifier/*` + `/external-events/*` (webhooks), `/oidc/:client_id/*` (protocol endpoints), and setup-kit's `/install/*`.

### Layered Architecture

**Controller → Service → DAO**, all under `app/`. Controllers parse requests into DTOs (`app/dto/`), services contain business logic, DAOs handle GORM queries.

### Static Assets & Dual-Mode Serving

Static files are embedded via `//go:embed` in `main.go`. The dual-mode driver (embedded FS in release, disk with hot reload in debug) lives in go-web's `pkg/server/http_server/impl/static_handler.go`.

Frontend apps map to:
- `frontend-oidc/` (submodule; Vue 3 + Tailwind v4 + reka-ui) → `static/auth/` — OIDC login/register/consent UI, served at `/auth`
- `admin-web/apps/web-antd` (submodule; Vben v5 + Ant Design Vue) → `static/admin/` — admin console at `/admin`, also hosts the guided installation flow
- `static/static/` holds shared vendor assets; stale `static/install/` is removed by `make build-frontend`

### Configuration System

Config sources live in `config/autoload/*.go` as `InitConfig()` methods returning `map[string]any`. These are aggregated into an in-memory map provider accessible via `helper.GetConfig()`. Runtime settings use the DB-backed `SettingService` with cache (see above).

External config: `config.yaml` (copy from `config.yaml.example`).

### OIDC Implementation

Full OAuth2/OIDC provider with authorization code flow, implicit flow, PKCE support.

- **Session management**: Redis-backed with TTL (`app/service/oidc_session_service.go`); key prefix `oidc:session:`, 10-minute expiry, rotating CSRF tokens, browser binding
- **Protocol abstraction**: `app/service/protocol/handler.go` defines `ProtocolHandler`; the factory supports OIDC / SAML / CAS / OAuth2, plus `directory/` (Feishu/WeCom/DingTalk sync), `event/`, `identity/`, and `social/` adapters
- **Endpoints**: Under `/oidc/:client_id/` — authorize, token, userinfo, introspect, revoke, jwks, discovery; login/consent UI APIs under `/api/oidc/*`
- **Auth flow UI**: Separate controller (`oidc_auth_flow_controller.go`) for login/register/consent pages

### Middleware Stack

Global middleware (applied by the go-web HTTP server, in order): traceId → access log → recovery → CORS → SetupInstaller (install gate). Auth middleware (`AuthMiddleware`, `AdminAuthMiddleware`, `UserAuthMiddleware`, `OptionalAuthMiddleware`; JWT validation plus DB session check) is applied per route group. RBAC/DataScope middleware exist in `app/middleware/` but are currently not attached to route groups — permission checks and data scoping apply through service-level helpers.

### RBAC & Data Permissions

Four-level data scope: All / Department / Department+Children / Self. Permission types: Menu, Button, API, Data. Middleware enforces both URL-level access and row-level data filtering.

### Graceful Restart

SIGHUP triggers a soft reload (stop servers → `ReloadAssemblies` → re-run in the same process); reload is also API-triggered. The implementation lives in go-web `pkg/server/reload` (`Restart()` re-executes the process via `syscall.Exec`, preserving the PID).

## Key Conventions

- All GORM models use soft delete (`DeletedAt` field)—queries auto-filter deleted records
- **Database migrations are goose versioned SQL files** in `migrations/` (embedded via `go:embed`, one set each for mysql/postgresql/sqlite), run automatically at startup by `migrations.Server` inside startupChecks. GORM AutoMigrate is a vestigial flag (`database.auto_migrate`, default off in release) — new DDL goes into goose migration files
- The `Application` model is protocol-agnostic with a `ProtocolType` field and JSON `ProtocolConfig`
- Handlers are auto-wrapped by the router — never register raw `gin.HandlerFunc` directly
- **Schema changes** add one goose SQL file per dialect under `migrations/{mysql,postgresql,sqlite}/` (embedded, run automatically at startup)
- **Frontend changes** are made inside the `frontend-oidc/` or `admin-web/` submodules: commit there first, then commit the updated submodule pointer in this repo
- Extension modules (`pkg/extension/`) can contribute routes, middleware, static dirs, capabilities, and startup hooks

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
