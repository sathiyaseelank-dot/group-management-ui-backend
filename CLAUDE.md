# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **Twingate-style zero-trust identity and access control management system** with:
- A **Go controller** (gRPC, mTLS, SPIFFE IDs) acting as the control plane
- **Rust connector and agent** services for gateway and client roles
- A **Vite + React + TypeScript** frontend with an Express API server
- **PostgreSQL** for controller backend persistence; **SQLite** (`better-sqlite3`) for frontend-local state only

## Commands

### Frontend (`cd apps/frontend`)

```bash
npm run dev     # Start Vite (port 3000) + Express server (port 3001) concurrently
npm run build   # Vite production build
npm run start   # Run Express server only (production)
npm run lint    # ESLint
```

### Controller (`cd services/controller`)

```bash
go build ./...
DATABASE_URL="postgres://..." go test ./...  # tests skip if DATABASE_URL is unset

# Run (requires env vars)
sudo TRUST_DOMAIN="mycorp.internal" \
  INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
  INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
  ADMIN_AUTH_TOKEN="<token>" \
  INTERNAL_API_TOKEN="<token>" \
  ADMIN_HTTP_ADDR="0.0.0.0:8081" \
  go run .
```

### Connector / Agent (`cd services/connector` or `services/agent`)

```bash
cargo build --release
cargo test
cargo run
```

### ZTNA Client (`cd services/ztna-client`)

```bash
cargo build --release
cargo run

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

### Root Makefile (from repo root)

```bash
make build-all          # Build all components
make dev-controller     # Run controller in dev mode (loads .env from services/controller/.env)
make dev-connector      # cargo run connector
make dev-agent          # cargo run agent
make dev-ztna-client    # cargo run ztna-client
make dev-frontend       # npm run dev in apps/frontend
make test-all           # Test all components
make clean              # Remove build artifacts
```

## Architecture

### Services

- **Controller** (`services/controller/`): Go service. Internal CA + enrollment gRPC server on `:8443`, admin HTTP API on `:8081`. Manages PostgreSQL DB, token store, ACLs, and policy distribution.
- **Connector** (`services/connector/`): Rust service. Gateway between agents and resources. Accepts agent gRPC connections on `:9443` and device tunnel connections on `:9444` (configurable via `DEVICE_TUNNEL_ADDR`).
- **Agent** (`services/agent/`): Rust server-side agent deployed on resource hosts. Connects to connector with mTLS, manages nftables firewall rules to protect resources. Runs entirely in-memory — no state is persisted to disk.
- **ZTNA Client** (`services/ztna-client/`): Rust end-user client. Authenticates via device OAuth PKCE flow. Supports two transport modes: **TUN** (default, transparent split-tunnel via kernel routing, requires root) and **SOCKS5** (proxy fallback, unprivileged). Tunnels traffic through the connector over mTLS.

### Split-Tunnel Traffic Flow

**TUN mode (default, `ZTNA_MODE=tun`):**
```
App (no config) → kernel route → TUN device (ztna0) → smoltcp TCP stack → ACL check → Connector tunnel (:9444) → Agent → Protected resource
```

**SOCKS5 mode (`ZTNA_MODE=socks5`):**
```
App (must configure proxy) → SOCKS5 proxy (ztna-client:1080) → ACL check → Connector tunnel (:9444) → Agent → Protected resource
```

**TUN mode flow:**
1. App sends TCP to resource IP — kernel routes through TUN device `ztna0` (via `ip route add <ip>/32 dev ztna0`)
2. smoltcp userspace TCP stack handles SYN/ACK handshake transparently
3. Client checks ACL via `POST /api/device/check-access` on the controller
4. If allowed, opens TLS tunnel to connector's device tunnel port (`:9444`)
5. Bidirectional relay: smoltcp socket <-> tunnel stream
6. Connector routes to the agent protecting the target resource
7. Agent forwards traffic to the resource

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

### Controller Internals

Admin HTTP API routes live in `services/controller/admin/`:
- Core handlers: `handlers_remote_networks.go`, `handlers_users.go`, `handlers_discovery.go`, `oauth_invite_handlers.go`, `handlers_device_auth.go`, `handlers_identity_providers.go`, `handlers_sessions.go`
- UI-specific endpoints: `ui_access_rules.go`, `ui_connectors.go`, `ui_groups.go`, `ui_resources.go`, `ui_tunnelers.go`, `ui_users.go`, `ui_remote_networks.go`, `ui_diagnostics.go`, `ui_policy.go`
- Device client: `device_routes.go`, `device_client_handlers.go`
- Routing: `ui_routes.go`; session utilities: `session_helpers.go`

gRPC implementations are in `services/controller/api/` — `enroll.go`, `control_plane.go`, `policy_snapshot.go`, `interceptor.go`.

Protobuf definitions are in `shared/proto/controller.proto`. Services: `EnrollmentService` (EnrollConnector, EnrollTunneler, Renew), `ControlPlane` (bidirectional streaming Connect).

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

**Architecture:** Vite dev server (port 3000) proxies `/api/*` to an Express server (port 3001). In production, Express serves the Vite build statically.

- **`server/index.ts`** — Express app, mounts all API routers
- **`server/routes/`** — Per-resource Express routers (groups, users, resources, connectors, agents, remote-networks, access-rules, tokens, subjects, service-accounts, policy, audit-logs, discovery, workspaces, diagnostics)
- **`lib/proxy.ts`** — Proxies Express requests to the Go controller at `NEXT_PUBLIC_API_BASE_URL` (default `:8081`) with Bearer token auth
- **`lib/db.ts`** — SQLite schema, migrations, seeding (via `better-sqlite3`) for frontend-local state
- **`lib/types.ts`** — All shared TypeScript types
- **`lib/mock-api.ts`** — Frontend API client calling `/api/*`
- **`lib/sign-in-policy.ts`**, **`lib/resource-policies.ts`**, **`lib/device-profiles.ts`** — Policy management (client-side, persisted to localStorage)

**Pages** under `src/pages/` — groups, users, resources, connectors, agents, remote-networks, and policy sub-routes. Components under `components/dashboard/`. Shared UI primitives are shadcn/ui components in `components/ui/`.

### Environment Variables

| Variable | Service | Description |
|---|---|---|
| `NEXT_PUBLIC_API_BASE_URL` | Frontend | Go controller URL (default: `http://localhost:8081`) |
| `ADMIN_AUTH_TOKEN` | Frontend + Controller | Bearer token for admin API |
| `INTERNAL_CA_CERT` | Controller/Connector/Agent | PEM CA certificate |
| `INTERNAL_CA_KEY` | Controller | PEM PKCS#8 CA private key |
| `CONTROLLER_ADDR` | Connector/Agent | `host:port` of controller gRPC (`:8443`) |
| `ADMIN_HTTP_ADDR` | Controller | HTTP listen address (default `:8081`) |
| `DATABASE_URL` | Controller | PostgreSQL connection string (required) |
| `TRUST_DOMAIN` | All | SPIFFE trust domain (default: `mycorp.internal`) |
| `OAUTH_REDIRECT_URL` | Controller | Google OAuth redirect URI for web UI login (e.g. `http://localhost:8080/oauth/google/callback`) |
| `OAUTH_CALLBACK_ADDR` | Controller | Extra HTTP listener for OAuth callbacks (e.g. `:8080`); serves same mux as admin API |
| `INVITE_BASE_URL` | Controller | Base URL used for device auth callback URI (e.g. `http://localhost:8081`) |
| `CONNECTOR_ID` | Connector | Connector identifier (required) |
| `AGENT_ID` | Agent | Agent identifier (required; fallback: `TUNNELER_ID`) |
| `ENROLLMENT_TOKEN` | Connector/Agent | One-time enrollment token (required) |
| `CONNECTOR_LISTEN_ADDR` | Connector | Listen address for agent gRPC (default inferred) |
| `DEVICE_TUNNEL_ADDR` | Connector | Device tunnel listen address (e.g. `0.0.0.0:9444`) |
| `CONTROLLER_HTTP_URL` | Connector | Controller HTTP URL for device tunnel auth (e.g. `http://host:8081`) |
| `CONNECTOR_ADDR` | Agent | Connector gRPC address for agent connection (e.g. `host:9443`) |
| `TUN_NAME` | Agent | TUN interface name (default: `tun0`) |
| `ZTNA_MODE` | ZTNA Client | Transport mode: `tun` (default, requires root) or `socks5` |
| `TUN_NAME` | ZTNA Client | TUN device name (default: `ztna0`) |
| `TUN_ADDR` | ZTNA Client | TUN device address in CIDR (default: `10.200.0.1/24`) |
| `TUN_MTU` | ZTNA Client | TUN device MTU (default: `1500`) |
| `SOCKS5_ADDR` | ZTNA Client | Local SOCKS5 proxy address for socks5 mode (default: `127.0.0.1:1080`) |
| `CONNECTOR_TUNNEL_ADDR` | ZTNA Client | Connector device tunnel address for split-tunnel (e.g. `host:9444`) |
| `ZTNA_TENANT` | ZTNA Client | Default workspace slug for split-tunnel |
| `CONTROLLER_URL` | ZTNA Client | Controller HTTP URL (default: `http://localhost:8081`) |
| `CA_CERT_PATH` | ZTNA Client | Path to connector CA PEM file |

### Key Design Notes

- **Controller DB is PostgreSQL-only** — `state.Open()` requires `DATABASE_URL`; `OpenSQLite()` is a no-op stub. Use `state.Rebind()` when writing raw SQL to convert `?` placeholders to `$1, $2, …` for PostgreSQL.
- **Multi-tenant workspaces** — resources, connectors, tunnelers, access rules, etc. are all scoped by `workspace_id`. The `withWorkspaceContext` middleware extracts workspace claims from a JWT cookie/Bearer token and populates request context.
- **Schema migrations** — `initSchemaDialect()` in `state/db.go` runs `CREATE TABLE IF NOT EXISTS` for all tables on startup. New columns are added with `ALTER TABLE … ADD COLUMN IF NOT EXISTS` (PostgreSQL) in the same function.
- **Frontend schema migrations** in `lib/db.ts` handle the frontend SQLite schema.
- **Policy state** (sign-in policy, resource policies, device profiles) is stored in localStorage, not in the database.
- `make dev-controller` loads env from `services/controller/.env` automatically.
- **Device OAuth flow** — `POST /api/device/authorize` builds the callback URI as `INVITE_BASE_URL + /api/device/callback`. Both `http://localhost:8080/oauth/google/callback` (web UI) and `http://localhost:8081/api/device/callback` (device flow) must be registered as authorized redirect URIs in Google Cloud Console for the same OAuth Client ID.
- **Access rule chain** — for a user to see resources via `ztna-client resources`, the full chain must exist: `user → user_group_members → user_groups → access_rule_groups → access_rules (enabled=1) → resources`. The `resourceCount` shown in the groups UI API counts all rules regardless of `enabled`, so a group can show `resourceCount > 0` while the agent sees 0 resources if the rule is disabled.
- **ZTNA Client caches resources** at login time. Run `ztna-client sync <tenant>` or re-login after changing group membership or access rules to refresh the local cache.
- **Agent is memory-only** — the agent does not persist any state to disk. It enrolls fresh on every start and receives firewall policy from the connector. On `initialize()` the nftables chain is flushed to prevent rule duplication across restarts.
- **Connector has two listener ports** — `:9443` for agent gRPC (mTLS) and `:9444` for device tunnel (TLS with JWT auth). The device tunnel port is set via `DEVICE_TUNNEL_ADDR`.
- **PostgreSQL runs in Docker** — container name `ztna-postgres`, image `postgres:16-alpine`, port `5432`. Connect with: `docker exec ztna-postgres psql -U ztnaadmin -d ztna -c "<query>"`
- **Systemd services** — unit files in `systemd/` directory (`agent.service`, `connector.service`). Connector config at `/etc/connector/connector.conf`, agent config at `/etc/agent/agent.conf`. Both use `LoadCredential=CONTROLLER_CA:/etc/<service>/ca.crt`.
