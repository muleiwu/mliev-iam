# Edition extensions

Modules may contribute routes, middleware, startup checks, capabilities, static directories and compiled first-party OIDC client descriptors. Community builds register no optional modules.

Register descriptors through `Module.FirstPartyClients` during composition. `service.EnsureFirstPartyApplication` reconciles only registered clients; a database `is_protected` flag alone never enables trusted callback handling. Their callback path is fixed and must share the trusted issuer origin. Built-in clients do not allow public registration.

`service.NewFirstPartyAuthFlowService` isolates state/PKCE/browser binding transactions by namespace. The administrator constructor preserves its existing Redis keys. `controller.EstablishExistingUserSession` signs the existing validated SID without creating or revoking sessions.

`service.UserMatchesAccessScope` preserves application audience semantics (empty means all). Extensions that support an explicitly restricted audience must reject restricted-empty scopes before calling it. `pkg/credentials` seals extension credentials using the existing installed keyring and authenticates a caller-supplied stable purpose. `AttachmentService.UploadBrandImage` reuses raster validation and configured object storage.
