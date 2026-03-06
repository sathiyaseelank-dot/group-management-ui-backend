# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **Twingate-style zero-trust identity and access control management system** with:
- A **Go backend** (gRPC, mTLS, SPIFFE IDs) acting as the control plane
- A **Vite + React 19 frontend** with an **Express BFF** (backend-for-frontend) server
- SQLite for both frontend local state and backend persistence

## Commands

### Frontend (`cd frontend`)

```bash
npm run dev       # Start Vite dev server (:3000) + Express BFF (:3001) concurrently
npm run build     # Vite production build
npm run start     # Run production build (Express serves built Vite app)
npm run lint      # ESLint
```

### Backend (`cd backend`)

Each component is a separate Go module. Build from within each service directory:

```bash
cd backend/controller && go build ./...
cd backend/connector  && go build ./...
cd backend/tunneler   && go build ./...
```

Run the controller (requires env vars):
```bash
sudo TRUST_DOMAIN="mycorp.internal" \
  INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
  INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
  ADMIN_AUTH_TOKEN="7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" \
  INTERNAL_API_TOKEN="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
  ADMIN_HTTP_ADDR="0.0.0.0:8081" \
  ./controller
```

Backend tests:
```bash
cd backend/connector && go test ./run/...   # policy cache tests
```

## Architecture

### Backend (Go)

Three services under `backend/` (plus a Rust connector `backend/connector-rs/`):

- **Controller** (`backend/controller/`): Internal CA + enrollment gRPC server on `:8443`, admin HTTP API on `:8081`. Manages SQLite DB, token store, ACLs, user/group/resource state, and policy distribution.
- **Connector** (`backend/connector/`): Maintains persistent outbound stream to controller gRPC. Accepts inbound tunneler connections on `:9443`. Caches and enforces distributed policies.
- **Tunneler** (`backend/tunneler/`): Client-side service that connects to a connector with mTLS.

All services use SPIFFE IDs under trust domain `spiffe://mycorp.internal`:
- Connector: `spiffe://mycorp.internal/connector/<id>`
- Tunneler: `spiffe://mycorp.internal/tunneler/<id>`

**Controller internal layout:**
- `admin/handlers.go` — HTTP server struct, auth middleware (`adminAuth`, `internalAuth`), route registration
- `admin/ui_routes.go` — UI-specific route registration (separate from `/api/admin/*` admin routes)
- `admin/ui_handlers.go` — handler implementations for UI endpoints (~48 KB)
- `admin/handlers_users.go`, `handlers_remote_networks.go` — split-out handler files
- `api/control_plane.go` — gRPC ControlPlane implementation (connector streams)
- `api/enroll.go` — enrollment gRPC service (issues signed certs)
- `api/interceptor.go` — gRPC auth interceptor (extracts SPIFFE ID, injects role)
- `api/policy_snapshot.go` — compiles and signs ACL policy snapshots
- `state/sqlite.go` — SQLite schema + migrations
- `state/users.go`, `acl.go`, `registry.go`, `token_store.go`, `remote_networks.go`, `tunneler_*.go` — domain stores

The Go module name for all backend packages is `grpc` (set in `backend/go.mod`).

### Frontend (Vite + React + Express BFF)

**Three-tier data flow:**
```
Browser (React, port 3000)
  → Vite dev proxy  →  Express BFF (port 3001)
                          ├── server/routes/*.ts  (field mapping, data assembly)
                          └── lib/proxy.ts  →  Go backend (port 8081)
```

In production, Express serves the built Vite app and handles all `/api/*` routes.

**Key lib files:**
- `lib/types.ts` — all shared TypeScript types (`User`, `Group`, `Resource`, `Connector`, `RemoteNetwork`, `Tunneler`, `AccessRule`, etc.)
- `lib/db.ts` — SQLite schema, migrations, and seeding via `better-sqlite3`; database file is `ztna.db`
- `lib/proxy.ts` — fetch wrapper used by Express BFF routes; reads `BACKEND_URL` and `ADMIN_AUTH_TOKEN` env vars
- `lib/mock-api.ts` — browser-side API client that calls `/api/*` routes (relative URLs by default, override with `VITE_API_BASE_URL`)
- `lib/data.ts` — data mappers from SQLite rows to TypeScript types
- `lib/sign-in-policy.ts`, `lib/resource-policies.ts`, `lib/device-profiles.ts` — policy management persisted to localStorage

**Express BFF routes** (`server/routes/`): Each route file handles a domain (users, groups, resources, connectors, remote-networks, access-rules, tunnelers, service-accounts, subjects, tokens, policy). Routes transform backend snake_case JSON → camelCase frontend types and assemble related data (e.g., user routes also fetch group memberships).

**Pages** are under `src/` with React Router DOM v6 for client-side routing. **Components** under `components/dashboard/` mirror the domain structure. Shared UI primitives are shadcn/ui (Radix UI) components in `components/ui/`.

### Environment Variables

| Variable | Service | Default | Description |
|---|---|---|---|
| `BACKEND_URL` | Express BFF | `http://localhost:8081` | Go backend URL |
| `ADMIN_AUTH_TOKEN` | Express BFF + Controller | `7f8e...a4` | Bearer token for admin API |
| `VITE_API_BASE_URL` | Browser | `` (relative) | Override API base in browser |
| `INTERNAL_CA_CERT` | Controller/Connector/Tunneler | — | PEM CA certificate |
| `INTERNAL_CA_KEY` | Controller | — | PEM PKCS#8 CA private key |
| `CONTROLLER_ADDR` | Connector/Tunneler | — | `host:port` of controller gRPC |
| `ADMIN_HTTP_ADDR` | Controller | `:8081` | HTTP listen address |
| `DB_PATH` | Controller | `ztna.db` | SQLite database path |
| `TRUST_DOMAIN` | All | `mycorp.internal` | SPIFFE trust domain |
| `INTERNAL_API_TOKEN` | Controller | — | Internal service-to-service token |
| `CONNECTOR_ID` / `TUNNELER_ID` | Connector/Tunneler | — | Unique service identifier |
| `BOOTSTRAP_CERT` / `BOOTSTRAP_KEY` | Connector/Tunneler | — | Bootstrap identity certificates |

### Key Design Notes

- **Admin vs UI routes**: The Go backend has two route families — `/api/admin/*` (Bearer token gated, CRUD) and `/api/*` (UI-focused, also CORS-allowed). The Express BFF primarily hits `/api/admin/*`.
- **Schema migrations**: Both `lib/db.ts` (frontend) and `state/sqlite.go` (backend) run ALTER TABLE migrations at startup to handle live schema upgrades.
- **Policy state** (sign-in policy, resource policies, device profiles): stored in localStorage on the client, not in any database.
- **gRPC auth**: mTLS certificates carry SPIFFE IDs; `api/interceptor.go` validates identity and injects a `role` ("connector" or "tunneler") into the gRPC context.
- **Internal token auth**: Calls to `/api/internal/*` (e.g., token consumption) use an `X-Internal-Token` header, separate from the admin Bearer token.
