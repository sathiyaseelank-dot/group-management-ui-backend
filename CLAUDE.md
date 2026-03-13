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

### Root Makefile (from repo root)

```bash
make build-all          # Build all components
make dev-controller     # Run controller in dev mode (loads .env from services/controller/.env)
make dev-connector      # cargo run connector
make dev-agent          # cargo run agent
make dev-frontend       # npm run dev in apps/frontend
make test-all           # Test all components
make clean              # Remove build artifacts
```

## Architecture

### Services

- **Controller** (`services/controller/`): Go service. Internal CA + enrollment gRPC server on `:8443`, admin HTTP API on `:8081`. Manages SQLite DB, token store, ACLs, and policy distribution.
- **Connector** (`services/connector/`): Rust service. Gateway between agents and resources, accepts inbound agent connections on `:9443`.
- **Agent** (`services/agent/`): Rust client service. Connects to connector with mTLS, provides local SOCKS5 proxy and nftables firewall enforcement.

All services use SPIFFE IDs under trust domain `spiffe://mycorp.internal`:
- Connector: `spiffe://mycorp.internal/connector/<id>`
- Agent: `spiffe://mycorp.internal/tunneler/<id>` (SPIFFE path kept as `tunneler` for wire compatibility)

Go module name for controller is `controller` (in `services/controller/go.mod`).

Admin HTTP API routes live in `services/controller/admin/` — `handlers_remote_networks.go`, `handlers_users.go`, `handlers_discovery.go`, `oauth_invite_handlers.go` for core routes; UI-specific endpoints split across `ui_access_rules.go`, `ui_connectors.go`, `ui_groups.go`, `ui_resources.go`, `ui_tunnelers.go`, `ui_users.go`, `ui_remote_networks.go`; `ui_routes.go` for routing; `session_helpers.go` for session utilities. gRPC implementations are in `services/controller/api/`.

Protobuf definitions are in `shared/proto/controller.proto`.

### Frontend (Vite + React + Express)

**Architecture:** Vite dev server (port 3000) proxies `/api/*` to an Express server (port 3001). In production, Express serves the Vite build statically.

- **`server/index.ts`** — Express app, mounts all API routers
- **`server/routes/`** — Per-resource Express routers (groups, users, resources, connectors, agents, remote-networks, access-rules, tokens, subjects, service-accounts, policy, audit-logs, discovery)
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
| `CONTROLLER_ADDR` | Connector/Agent | `host:port` of controller gRPC |
| `ADMIN_HTTP_ADDR` | Controller | HTTP listen address (default `:8081`) |
| `DATABASE_URL` | Controller | PostgreSQL connection string (required) |
| `DB_PATH` | Controller | Legacy SQLite path (ignored — PostgreSQL only) |
| `TRUST_DOMAIN` | All | SPIFFE trust domain (default: `mycorp.internal`) |

### Key Design Notes

- **Controller DB is PostgreSQL-only** — `state.Open()` requires `DATABASE_URL`; `OpenSQLite()` is a no-op stub. Use `state.Rebind()` when writing raw SQL to convert `?` placeholders to `$1, $2, …` for PostgreSQL.
- **Multi-tenant workspaces** — resources, connectors, tunnelers, access rules, etc. are all scoped by `workspace_id`. The `withWorkspaceContext` middleware extracts workspace claims from a JWT cookie/Bearer token and populates request context.
- **Schema migrations** — `initSchemaDialect()` in `state/db.go` runs `CREATE TABLE IF NOT EXISTS` for all tables on startup. New columns are added with `ALTER TABLE … ADD COLUMN IF NOT EXISTS` (PostgreSQL) in the same function.
- **Frontend schema migrations** in `lib/db.ts` handle the frontend SQLite schema.
- **Policy state** (sign-in policy, resource policies, device profiles) is stored in localStorage, not in the database.
- `make dev-controller` loads env from `services/controller/.env` automatically.
