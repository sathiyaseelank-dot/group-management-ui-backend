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
  JWT_SECRET="<secret>" \
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

# Run in TUN mode (default, requires root)
sudo CONTROLLER_URL=http://localhost:8081 \
ZTNA_TENANT=<workspace-slug> \
CONNECTOR_TUNNEL_ADDR=<connector-host>:9444 \
CA_CERT_PATH=ca/ca.crt \
cargo run

# Run in SOCKS5 mode (unprivileged fallback)
CONTROLLER_URL=http://localhost:8081 \
ZTNA_TENANT=<workspace-slug> \
ZTNA_MODE=socks5 \
SOCKS5_ADDR=127.0.0.1:1080 \
CONNECTOR_TUNNEL_ADDR=<connector-host>:9444 \
INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
cargo run

# CLI commands (interactive UI is default)
ztna-client login <tenant>
ztna-client status [tenant]
ztna-client resources [tenant]
ztna-client sync <tenant>
ztna-client disconnect <tenant>
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

### PostgreSQL (from repo root)

```bash
docker compose up -d              # Start PostgreSQL (postgres:16-alpine, port 5432)
docker exec ztna-postgres psql -U ztnaadmin -d ztna -c "<query>"  # Run queries
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
| **Connector** | Rust | `:9443` (agent gRPC), `:9444/tcp` (TLS tunnel), `:9444/udp` (QUIC tunnel) | Gateway between agents/devices and resources |
| **Agent** | Rust | outbound only | Server-side agent: connects to connector, enforces nftables firewall. Runs entirely in-memory. |
| **ztna-client** | Rust | N/A | Desktop CLI: device enrollment, OAuth PKCE, TUN/SOCKS5 split-tunnel |
| **ztna-client-mobile** | Rust (UniFFI lib) | N/A | Shared auth/networking core for Android (and future iOS) |
| **Frontend** | React 19 + Express | `:3000` (Vite), `:3001` (Express) | Admin dashboard + user portal |
| **Android** | Kotlin 2.0 | N/A | Mobile zero-trust client |

### Split-Tunnel Traffic Flow

**TUN mode (default, `ZTNA_MODE=tun`):**
```
App (no config) → kernel route → TUN device (ztna0) → smoltcp TCP / raw UDP → ACL check → Connector tunnel (:9444) → Agent → Protected resource
```

**SOCKS5 mode (`ZTNA_MODE=socks5`):**
```
App (must configure proxy) → SOCKS5 proxy (ztna-client:1080) → ACL check → Connector tunnel (:9444) → Agent → Protected resource
```

**TUN mode flow (TCP):**
1. App sends TCP to resource IP — kernel routes through TUN device `ztna0` (via `ip route add <ip>/32 dev ztna0`)
2. smoltcp userspace TCP stack handles SYN/ACK handshake transparently
3. Client checks ACL via `POST /api/device/check-access` on the controller
4. If allowed, tries QUIC first (if cached), falls back to TLS tunnel to connector `:9444`
5. Bidirectional relay: smoltcp socket <-> tunnel stream
6. Connector routes to the agent protecting the target resource
7. Agent forwards traffic to the resource

**TUN mode flow (UDP):**
1. App sends UDP to resource IP — kernel routes through TUN device `ztna0`
2. Raw UDP packet parsed with `etherparse` (bypasses smoltcp entirely)
3. DNS queries (port 53) for resource domains are intercepted and resolved locally
4. Non-DNS UDP: ACL check, then length-prefixed datagram relay over TLS/QUIC tunnel
5. Connector relays to agent, agent forwards via `UdpSocket`
6. Response datagrams are wrapped in raw IPv4+UDP packets and injected back into TUN

**SOCKS5 mode flow:**
1. Application connects through SOCKS5 proxy on `127.0.0.1:1080`
2. Client checks ACL via `POST /api/device/check-access` on the controller
3. If allowed, opens TLS tunnel to connector's device tunnel port (`:9444`)
4. Connector routes to the agent protecting the target resource
5. Agent forwards traffic to the resource

### SPIFFE Identity

All services use SPIFFE IDs under trust domain `spiffe://mycorp.internal`:
- Connector: `spiffe://mycorp.internal/connector/<id>`
- Agent: `spiffe://mycorp.internal/tunneler/<id>` (SPIFFE path kept as `tunneler` for wire compatibility)
- Device: `spiffe://mycorp.internal/device/<user_id>/<device_id>`

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

### Device Auth API

```
POST /api/device/authorize       # Start OAuth PKCE flow
GET  /api/device/callback        # OAuth provider redirect target
POST /api/device/token           # Exchange code for JWT + ACL snapshot
POST /api/device/refresh         # Rotate refresh token, issue new JWT
POST /api/device/revoke          # Revoke a session
POST /api/device/check-access    # Per-request ACL check (destination:port)
GET  /api/device/me              # Current user/workspace/resources view
POST /api/device/sync            # Refresh cached resources
POST /api/device/enroll-cert     # Issue device mTLS certificate
```

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
| `JWT_SECRET` | | HMAC secret for JWT signing. If unset, the controller derives one from `INTERNAL_CA_KEY`. |
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
| `IDP_ENCRYPTION_KEY` | | AES-256 key for workspace IdP secrets (fallback: effective JWT secret, including the derived `INTERNAL_CA_KEY` fallback) |
| `SECURE_COOKIES` | | `true` for HTTPS-only cookies |
| `ALLOWED_ORIGINS` | | Comma-separated CORS allowlist |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` | | Email config for invites |

### Frontend (`apps/frontend/.env`)

| Variable | Description |
|---|---|
| `NEXT_PUBLIC_API_BASE_URL` | Go controller URL (default: `http://localhost:8081`) |
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
- **Device OAuth flow** — `POST /api/device/authorize` builds the callback URI as `INVITE_BASE_URL + /api/device/callback`. Both `http://localhost:8080/oauth/google/callback` (web UI) and `http://localhost:8081/api/device/callback` (device flow) must be registered as authorized redirect URIs in Google Cloud Console for the same OAuth Client ID.
- **Access rule chain** — for a user to see resources via `ztna-client resources`, the full chain must exist: `user → user_group_members → user_groups → access_rule_groups → access_rules (enabled=1) → resources`. The `resourceCount` shown in the groups UI API counts all rules regardless of `enabled`, so a group can show `resourceCount > 0` while the agent sees 0 resources if the rule is disabled.
- **ZTNA Client caches resources** at login time. Run `ztna-client sync <tenant>` or re-login after changing group membership or access rules to refresh the local cache.
- **Agent is memory-only** — the agent does not persist any state to disk. It enrolls fresh on every start and receives firewall policy from the connector. On `initialize()` the nftables chain is flushed to prevent rule duplication across restarts.
- **Connector has three listeners** — `:9443` for agent gRPC (mTLS), `:9444/tcp` for TLS device tunnel, `:9444/udp` for QUIC device tunnel. QUIC is non-fatal — if it fails to bind, only TLS is used. The device tunnel port is set via `DEVICE_TUNNEL_ADDR`.
- **QUIC upgrade (Option C discovery)** — the TLS tunnel handshake response includes `quic_addr` when QUIC is available. The client caches this and tries QUIC first (3s timeout) on subsequent connections, falling back to TLS. The QUIC pool (`quic_tunnel.rs`) maintains one QUIC connection per connector with multiplexed streams.
- **Tunnel wire protocol** — the handshake JSON includes a `protocol` field (`"tcp"` or `"udp"`, defaults to `"tcp"` via `#[serde(default)]` for backward compat). UDP datagrams use length-prefixed framing (`[u32 BE length][payload]`) over the TLS/QUIC byte stream. The connector↔agent hop uses gRPC `ControlMessage` frames (JSON payloads) which are naturally message-delimited.
- **DNS interception** — `tun_dns_intercept.rs` intercepts UDP port 53 queries for known resource domains at the TUN level, resolving them locally to prevent DNS leaks. Non-resource domains pass through. The domain set is rebuilt from workspace resources every 60 seconds.
- **UDP in TUN bypasses smoltcp** — UDP packets are parsed with `etherparse` and relayed directly (no smoltcp UDP sockets). Response datagrams are constructed as raw IPv4+UDP packets with `etherparse` and injected into the TUN writer. UDP flows have a 30-second idle timeout.
- **PostgreSQL runs in Docker** — container name `ztna-postgres`, image `postgres:16-alpine`, port `5432`. Connect with: `docker exec ztna-postgres psql -U ztnaadmin -d ztna -c "<query>"`
- **Systemd services** — unit files in `systemd/` directory (`agent.service`, `connector.service`). Connector config at `/etc/connector/connector.conf`, agent config at `/etc/agent/agent.conf`. Both use `LoadCredential=CONTROLLER_CA:/etc/<service>/ca.crt`.
- **Commit messages** follow Conventional Commits with component scopes: `feat(controller): ...`, `fix(connector): ...`, `test(agent): ...`, `chore: ...`. PRs typically target `develop`.
- **Protobuf changes** (`shared/proto/controller.proto`) affect all services — regenerate and verify across Go, Rust, and frontend after modifications.
- **CI/CD** — GitHub Actions workflows in `.github/workflows/` build release binaries for `x86_64` and `aarch64` Linux on `v*` tag push. Binary names: `connector-linux-amd64`, `agent-linux-amd64`, `ztna-client-linux-amd64` (and `-arm64` variants).
- **Client security** — OAuth callback binds to `127.0.0.1` by default (not `0.0.0.0`). HTML responses in the callback handler are escaped via `html_escape()`. Tenant slugs are validated (`[a-zA-Z0-9][a-zA-Z0-9_-]*`, max 63 chars) to prevent path traversal. The callback endpoint is rate-limited (10 req/60s). Service token file uses `0640` with `ztna` group when available (falls back to `0644`).
- **`validate_tenant_slug()`** in `config.rs` is called from `persist_tenant()`, `require_tenant()`, `run_setup()`, and `begin_login()`. Any code accepting a tenant slug from user input should call this function.
