# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **Twingate-style zero-trust identity and access control management system** with:
- A **Go controller** (gRPC, mTLS, SPIFFE IDs) acting as the control plane
- **Rust connector and tunneler** services for gateway and client roles
- A **Vite + React + TypeScript** frontend with an Express API server
- SQLite for both frontend local state and backend persistence

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
go test ./...

# Run (requires env vars)
sudo TRUST_DOMAIN="mycorp.internal" \
  INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
  INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
  ADMIN_AUTH_TOKEN="<token>" \
  INTERNAL_API_TOKEN="<token>" \
  ADMIN_HTTP_ADDR="0.0.0.0:8081" \
  go run .
```

### Connector / Tunneler (`cd services/connector` or `services/tunneler`)

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
make dev-tunneler       # cargo run tunneler
make dev-frontend       # npm run dev in apps/frontend
make test-all           # Test all components
make clean              # Remove build artifacts
```

## Architecture

### Services

- **Controller** (`services/controller/`): Go service. Internal CA + enrollment gRPC server on `:8443`, admin HTTP API on `:8081`. Manages SQLite DB, token store, ACLs, and policy distribution.
- **Connector** (`services/connector/`): Rust service. Gateway between tunnelers and resources, accepts inbound tunneler connections on `:9443`.
- **Tunneler** (`services/tunneler/`): Rust client service. Connects to connector with mTLS, provides local SOCKS5 proxy.

All services use SPIFFE IDs under trust domain `spiffe://mycorp.internal`:
- Connector: `spiffe://mycorp.internal/connector/<id>`
- Tunneler: `spiffe://mycorp.internal/tunneler/<id>`

Go module name for controller is `controller` (in `services/controller/go.mod`).

Admin HTTP API routes live in `services/controller/admin/` — `handlers.go`, `handlers_remote_networks.go`, `handlers_users.go` for core routes; `ui_handlers.go` for UI-specific endpoints; `ui_routes.go` for routing. gRPC implementations are in `services/controller/api/`.

Protobuf definitions are in `shared/proto/controller.proto`.

### Frontend (Vite + React + Express)

**Architecture:** Vite dev server (port 3000) proxies `/api/*` to an Express server (port 3001). In production, Express serves the Vite build statically.

- **`server/index.ts`** — Express app, mounts all API routers
- **`server/routes/`** — Per-resource Express routers (groups, users, resources, connectors, tunnelers, remote-networks, access-rules, tokens, subjects, service-accounts, policy)
- **`lib/proxy.ts`** — Proxies Express requests to the Go controller at `NEXT_PUBLIC_API_BASE_URL` (default `:8081`) with Bearer token auth
- **`lib/db.ts`** — SQLite schema, migrations, seeding (via `better-sqlite3`) for frontend-local state
- **`lib/types.ts`** — All shared TypeScript types
- **`lib/mock-api.ts`** — Frontend API client calling `/api/*`
- **`lib/sign-in-policy.ts`**, **`lib/resource-policies.ts`**, **`lib/device-profiles.ts`** — Policy management (client-side, persisted to localStorage)

**Pages** under `src/pages/` — groups, users, resources, connectors, tunnelers, remote-networks, and policy sub-routes. Components under `components/dashboard/`. Shared UI primitives are shadcn/ui components in `components/ui/`.

### Environment Variables

| Variable | Service | Description |
|---|---|---|
| `NEXT_PUBLIC_API_BASE_URL` | Frontend | Go controller URL (default: `http://localhost:8081`) |
| `ADMIN_AUTH_TOKEN` | Frontend + Controller | Bearer token for admin API |
| `INTERNAL_CA_CERT` | Controller/Connector/Tunneler | PEM CA certificate |
| `INTERNAL_CA_KEY` | Controller | PEM PKCS#8 CA private key |
| `CONTROLLER_ADDR` | Connector/Tunneler | `host:port` of controller gRPC |
| `ADMIN_HTTP_ADDR` | Controller | HTTP listen address (default `:8081`) |
| `DB_PATH` | Controller | SQLite database path |
| `TRUST_DOMAIN` | All | SPIFFE trust domain (default: `mycorp.internal`) |

### Key Design Notes

- **Schema migrations** in `lib/db.ts` handle live upgrades (e.g., adding columns to `access_rules`)
- **Policy state** (sign-in policy, resource policies, device profiles) is stored in localStorage, not in the database
- `make dev-controller` loads env from `services/controller/.env` automatically
