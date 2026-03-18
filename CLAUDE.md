# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **Twingate-style zero-trust identity and access control management system** with:
- A **Go controller** (gRPC, mTLS, SPIFFE IDs) acting as the control plane
- **Rust connector and agent** services for gateway and client roles
- A **Vite + React + TypeScript** frontend with an Express API server
- An **Android mobile client** (Kotlin + Jetpack Compose) backed by a **Rust UniFFI networking library**
- A **Rust CLI client** (`ztna-client`) for desktop device enrollment
- **PostgreSQL** for controller backend persistence; **SQLite** (`better-sqlite3`) for frontend-local state only

---

## Commands

### Frontend (`cd apps/frontend`)

```bash
npm run dev     # Start Vite (port 3000) + Express server (port 3001) concurrently
npm run build   # Vite production build
npm run start   # Run Express server only (production)
npm run lint    # ESLint
npm run test    # Jest
```

### Controller (`cd services/controller`)

```bash
go build ./...
DATABASE_URL="postgres://..." go test ./...  # tests skip if DATABASE_URL is unset

# Run (requires env vars — or use make dev-controller which loads .env)
sudo TRUST_DOMAIN="mycorp.internal" \
  INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
  INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
  ADMIN_AUTH_TOKEN="<token>" \
  INTERNAL_API_TOKEN="<token>" \
  ADMIN_HTTP_ADDR="0.0.0.0:8081" \
  DATABASE_URL="postgres://..." \
  go run .
```

### Connector / Agent (`cd services/connector` or `services/agent`)

```bash
cargo build --release
cargo test
cargo run
```

### ZTNA Client CLI (`cd services/ztna-client`)

```bash
cargo build --release
cargo run                        # default: connects to http://localhost:8081
```

### ZTNA Mobile Library (`cd services/ztna-client-mobile`)

```bash
cargo build                      # host build (for testing)
# Android cross-compile (requires cargo-ndk + NDK installed):
cargo ndk -t arm64-v8a build --release
cargo ndk -t x86_64 build --release
```

### Android App (`cd mobile/android`)

```bash
./gradlew buildRustAll           # cross-compile both ABIs
./gradlew generateUniffiBindings # regenerate Kotlin bindings from ztna.udl
./gradlew assembleDebug          # build APK
./gradlew installDebug           # install on connected device
```

### Root Makefile (from repo root)

```bash
make build-all          # Build all components
make build-controller   # Go binary only
make build-ztna-client  # ztna-client CLI
make dev-controller     # Run controller in dev mode (loads .env from services/controller/.env)
make dev-connector      # cargo run connector
make dev-agent          # cargo run agent
make dev-ztna-client    # Run ztna-client (requires controller on localhost:8081)
make dev-frontend       # npm run dev in apps/frontend
make test-all           # Test all components
make clean              # Remove build artifacts
```

---

## Architecture

### Services

| Service | Language | Port | Purpose |
|---|---|---|---|
| **Controller** | Go 1.24 | `:8081` (HTTP admin), `:8443` (gRPC enrollment), `:8080` (OAuth callbacks) | Control plane: CA, enrollment, ACLs, OAuth, device auth |
| **Connector** | Rust | `:9443` (inbound) | Gateway between tunnelers and resources |
| **Agent** | Rust | outbound only | Client tunneler: connects to connector, exposes SOCKS5 proxy, enforces nftables firewall |
| **ztna-client** | Rust | N/A | Desktop CLI for device enrollment and OAuth |
| **ztna-client-mobile** | Rust (UniFFI lib) | N/A | Shared auth/networking core for Android (and future iOS) |
| **Frontend** | React 19 + Express | `:3000` (Vite), `:3001` (Express) | Admin dashboard + user portal |
| **Android** | Kotlin 2.0 | N/A | Mobile zero-trust client |

All services use SPIFFE IDs under trust domain `spiffe://mycorp.internal`:
- Connector: `spiffe://mycorp.internal/connector/<id>`
- Agent: `spiffe://mycorp.internal/tunneler/<id>` (SPIFFE path kept as `tunneler` for wire compatibility)

Go module name for controller is `controller` (in `services/controller/go.mod`).

---

### Controller

**File layout** (`services/controller/`):

- **`main.go`** — Entry point; reads env vars, builds `admin.Server{}`, registers routes, starts HTTP + gRPC listeners
- **`admin/`** — All HTTP handlers:
  - `server.go` — `Server` struct (OAuthConfig, ClientOAuthConfig, JWTSecret, Sessions, IdPs, Users, Workspaces, ACLs, ...)
  - `ui_routes.go` — Route registration
  - `oauth_invite_handlers.go` — Admin OAuth login, invite accept, GitHub/Google callbacks, CSRF state store
  - `oauth_providers.go` — `BuildGoogleOAuthConfig`, `BuildClientGoogleOAuthConfig`, `BuildGitHubOAuthConfig`
  - `handlers_device_auth.go` — Device PKCE flow: `/api/device/authorize`, `/api/device/callback`, `/api/device/token`, `/api/device/refresh`, `/api/device/revoke`
  - `device_client_handlers.go` — `/api/device/me`, `/api/device/sync`, `/api/device/enroll-cert`, `/api/device/posture`
  - `device_routes.go` — Device route registration
  - `session_helpers.go` — JWT signing/parsing (`signAdminJWT`, `signDeviceJWT`, `parseAllClaims`)
  - `ui_users.go`, `ui_groups.go`, `ui_resources.go`, `ui_connectors.go`, `ui_tunnelers.go`, `ui_remote_networks.go`, `ui_access_rules.go` — Per-resource CRUD handlers
  - `handlers_users.go`, `handlers_remote_networks.go`, `handlers_discovery.go` — Core resource handlers
  - `identity_provider_handlers.go` — Workspace IdP CRUD
  - `session_handlers.go` — Session list/revoke
- **`api/`** — gRPC implementations
- **`state/`** — Database layer:
  - `db.go` — `initSchemaDialect()` runs all `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` migrations on startup
  - `identity_provider.go` — `IdentityProviderStore` (AES-256-GCM encrypted client secrets)
  - `session.go` — `SessionStore`
  - `user.go`, `workspace.go` — User and workspace stores

**gRPC** definitions in `shared/proto/controller.proto`.

---

### OAuth / Authentication Flows

The controller runs two HTTP listeners:
- `:8081` — admin API (all `/api/*` routes)
- `:8080` — OAuth callbacks only (`OAUTH_CALLBACK_ADDR=:8080`)

**Three distinct flows:**

| Flow | Google App | PKCE | State Prefix | Ends at |
|---|---|---|---|---|
| Admin login | Admin app (`GOOGLE_CLIENT_ID`) | No | (none / `"invite:"`) | Dashboard cookie + redirect |
| Device PKCE (mobile) | Client app (`CLIENT_GOOGLE_CLIENT_ID`) | Yes (S256) | `"device:"` | `ztna://callback?code=...` |
| Invite PKCE (browser) | Client app (`CLIENT_GOOGLE_CLIENT_ID`) | Yes (S256) | `"invitepkce:"` | Frontend `/app/welcome?token=...` |

- `handleOAuthCallback` (at `/oauth/google/callback`) routes by state prefix: `"device:"` → `handleDeviceCallback`, else → web login handler.
- `handleInviteCallback` (at `/api/invite/callback`) handles the browser PKCE invite flow.
- JWT `aud` claim: `"admin"` for dashboard sessions, `"device"` for mobile sessions.
- `adminAuth` middleware rejects device tokens; `deviceAuth` middleware rejects admin tokens.

---

### Android Mobile App

**Location**: `mobile/android/`
**Package**: `com.zerotrust.ztna` | **Min SDK**: 26 | **Target SDK**: 34

- **Screens**: `LoginScreen`, `ResourcesScreen`, `SettingsScreen`
- **ViewModel**: `ZtnaViewModel` — calls UniFFI functions (`beginLogin`, `completeLogin`, `sync`, `disconnect`, `listWorkspaces`)
- **Deep link**: `ztna://callback?code=...&state=...` handled in `MainActivity` → calls `viewModel.completeLogin(code, state)`
- **Rust integration**: Prebuilt `.so` in `app/src/main/jniLibs/`; UniFFI-generated bindings in `app/uniffi/ztna/`

---

### Rust Mobile Library (UniFFI)

**Location**: `services/ztna-client-mobile/`
**Exposed via** `ztna.udl`:

```
begin_login(controller_url, tenant_slug, redirect_uri, data_dir) → String  // returns Google auth URL
complete_login(controller_url, code, state, data_dir) → WorkspaceState
load_state(tenant_slug, data_dir) → WorkspaceState?
list_workspaces(data_dir) → [WorkspaceState]
sync(tenant_slug, controller_url, data_dir) → WorkspaceState
disconnect(tenant_slug, controller_url, data_dir) → void
```

`WorkspaceState` contains: workspace info, user info, access_token, refresh_token, cert, resources (`[ResourceItem]`).

State is persisted as JSON files in `data_dir` (Android: `filesDir`). PKCE `code_verifier` is stored transiently during the auth flow.

---

### Frontend (Vite + React + Express)

**Architecture:** Vite dev server (port 3000) proxies `/api/*` to Express (port 3001). In production, Express serves the Vite build statically.

**Key files:**
- **`server/index.ts`** — Express app, mounts all API routers
- **`server/routes/`** — Per-resource routers proxying to Go controller
- **`lib/proxy.ts`** — Proxies Express requests to controller at `NEXT_PUBLIC_API_BASE_URL` (default `:8081`) with Bearer token
- **`lib/db.ts`** — SQLite schema for frontend-local state (`better-sqlite3`)
- **`lib/types.ts`** — All shared TypeScript types
- **`lib/mock-api.ts`** — Frontend API client calling `/api/*`
- **`src/App.tsx`** — `TokenCapture` component: reads `?token=` from URL after OAuth redirect, stores in `localStorage.authToken`

**Pages** (`src/pages/`):
- `/login` — Login (admin/user entry point, Google OAuth redirect)
- `/signup/*` — Workspace signup flow
- `/workspaces`, `/workspaces/new` — Workspace selector/creator
- `/dashboard/*` — Admin dashboard (groups, users, resources, connectors, agents, networks, policy, settings, diagnostics)
  - `/policy/device-profiles` — Device trusted profiles
  - `/settings/identity-providers` — Workspace OAuth/OIDC config
  - `/settings/sessions` — Active session management
  - `/devices` — Device posture overview
- `/app/*` — User portal (home, welcome, install guide)

**Token handling**: After OAuth, the controller redirects to `DASHBOARD_URL?token=<jwt>`. `TokenCapture` in `App.tsx` reads the token, stores it in `localStorage`, and routes based on `aud` and `wrole` JWT claims.

---

### Database Schema (PostgreSQL)

All tables created via `initSchemaDialect()` in `state/db.go` on startup. New columns added with `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`.

**Core tables**: `connectors`, `tunnelers`, `resources`, `authorizations`, `tokens`, `audit_logs`, `users`, `user_groups`, `user_group_members`, `remote_networks`, `remote_network_connectors`, `connector_policy_versions`, `access_rules`, `access_rule_groups`, `service_accounts`, `connector_logs`, `tunneler_logs`

**Auth / identity tables**: `invite_tokens`, `workspace_invites`, `workspaces`, `workspace_members`, `identity_providers` (client secrets AES-256-GCM encrypted), `sessions` (refresh_token_hash, session_type: admin|device), `device_auth_requests` (PKCE state: code_challenge, redirect_uri, expires_at)

**Device posture tables**: `device_posture` (OS, firewall, disk encryption, screen lock, client version, device model/make/serial), `device_trusted_profiles` (compliance policy per user group)

**Admin audit**: `admin_audit_logs`

---

## Environment Variables

### Controller (`services/controller/.env`)

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | ✅ | PostgreSQL connection string |
| `INTERNAL_CA_CERT` | ✅ | PEM CA certificate |
| `INTERNAL_CA_KEY` | ✅ | PEM PKCS#8 CA private key |
| `ADMIN_AUTH_TOKEN` | ✅ | Bearer token for admin API |
| `INTERNAL_API_TOKEN` | ✅ | Token for internal service auth |
| `JWT_SECRET` | ✅ | HMAC secret for JWT signing |
| `TRUST_DOMAIN` | | SPIFFE trust domain (default: `mycorp.internal`) |
| `ADMIN_HTTP_ADDR` | | HTTP listen address (default: `:8081`) |
| `OAUTH_CALLBACK_ADDR` | | Secondary listener for OAuth callbacks (default: none, uses main addr) |
| `GOOGLE_CLIENT_ID` | | **Admin** Google OAuth app client ID |
| `GOOGLE_CLIENT_SECRET` | | **Admin** Google OAuth app secret |
| `OAUTH_REDIRECT_URL` | | Admin OAuth redirect URI (e.g. `http://localhost:8080/oauth/google/callback`) |
| `CLIENT_GOOGLE_CLIENT_ID` | | **Client** Google OAuth app client ID (PKCE flows) |
| `CLIENT_GOOGLE_CLIENT_SECRET` | | **Client** Google OAuth app secret |
| `CLIENT_OAUTH_REDIRECT_URL` | | Client OAuth redirect URI (e.g. `http://localhost:8080/api/invite/callback`) |
| `GITHUB_CLIENT_ID` | | GitHub OAuth client ID |
| `GITHUB_CLIENT_SECRET` | | GitHub OAuth secret |
| `GITHUB_OAUTH_REDIRECT_URL` | | GitHub OAuth redirect URI |
| `ADMIN_LOGIN_EMAILS` | | CSV of emails allowed admin login (empty = DB role check) |
| `DASHBOARD_URL` | | Frontend URL for post-OAuth redirect (default: `http://localhost:3000`) |
| `INVITE_BASE_URL` | | Base URL for invite links (use frontend URL: `http://localhost:3000`) |
| `IDP_ENCRYPTION_KEY` | | AES-256 key for workspace IdP secrets (fallback: `JWT_SECRET`) |
| `SECURE_COOKIES` | | `true` for HTTPS-only cookies |
| `ALLOWED_ORIGINS` | | Comma-separated CORS allowlist |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` | | Email config for invites |
| `POLICY_SIGNING_KEY` | | (default: `INTERNAL_API_TOKEN`) |

### Frontend (`apps/frontend/.env`)

| Variable | Description |
|---|---|
| `NEXT_PUBLIC_API_BASE_URL` | Go controller URL (default: `http://localhost:8081`) |
| `ADMIN_AUTH_TOKEN` | Bearer token for proxying to controller |
| `VITE_CONTROLLER_URL` | Controller base URL for browser-side redirects (default: `http://localhost:8081`) |

### Connector / Agent

| Variable | Description |
|---|---|
| `CONTROLLER_ADDR` | `host:port` of controller gRPC (`:8443`) |
| `INTERNAL_CA_CERT` | PEM CA certificate |
| `TRUST_DOMAIN` | SPIFFE trust domain |

---

## Key Design Notes

- **Controller DB is PostgreSQL-only** — `state.Open()` requires `DATABASE_URL`. Use `state.Rebind()` when writing raw SQL to convert `?` placeholders to `$1, $2, …` for PostgreSQL.
- **Multi-tenant workspaces** — resources, connectors, tunnelers, access rules, etc. are all scoped by `workspace_id`. The `withWorkspaceContext` middleware extracts workspace claims from a JWT cookie/Bearer token.
- **Schema migrations** — `initSchemaDialect()` in `state/db.go` runs all `CREATE TABLE IF NOT EXISTS` on startup. New columns use `ALTER TABLE … ADD COLUMN IF NOT EXISTS` in the same function.
- **Two Google OAuth apps** — Admin app (`GOOGLE_CLIENT_ID`) for the dashboard login (no PKCE). Client app (`CLIENT_GOOGLE_CLIENT_ID`) for mobile device auth and browser invite PKCE flow. If `CLIENT_GOOGLE_CLIENT_ID` is unset, both flows fall back to `OAuthConfig`.
- **JWT audiences** — `aud:"admin"` for dashboard sessions (24hr); `aud:"device"` for mobile sessions (15min + refresh token). Middleware strictly enforces this separation.
- **Device PKCE flow** — `POST /api/device/authorize` → Google → `GET /api/device/callback` → one-time code → `ztna://callback` → `POST /api/device/token` (PKCE S256 verified).
- **Invite PKCE flow** — Frontend generates PKCE pair → `POST /api/invite/authorize` → Google → `GET /api/invite/callback` (validates ID token via JWKS) → frontend `POST /api/invite/token` (PKCE verified) → user created.
- **UniFFI mobile library** — `services/ztna-client-mobile` builds as `libztna.so`. Functions defined in `ztna.udl`; Kotlin bindings auto-generated. State stored as JSON files in `data_dir`.
- **Android deep link** — `ztna://callback?code=...&state=...` is registered in `AndroidManifest.xml`. `MainActivity` handles it and calls `viewModel.completeLogin()`.
- **Policy state** (sign-in policy, resource policies, device profiles) is stored in localStorage on the frontend, not in the database.
- `make dev-controller` loads env from `services/controller/.env` automatically.
