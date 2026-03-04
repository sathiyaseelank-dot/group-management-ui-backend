# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Twingate-style zero-trust network access (ZTNA) system with:
- A **Go backend** (gRPC, mTLS, SPIFFE IDs) acting as the control plane
- A **React + Vite + Express** frontend for administrative UI
- SQLite for both frontend local state and backend persistence

## Commands

### Frontend (`cd frontend`)

```bash
npm run dev       # Start Vite dev server (port 3000) + Express BFF (port 3001)
npm run build     # Production build
npm run lint      # ESLint
```

### Backend (`cd backend`)

Each component is a separate Go module — `go build ./...` must be run from within `controller/`, `connector/`, or `tunneler/`.

```bash
# Build
cd backend/controller && go build ./...
cd backend/connector && go build ./...
cd backend/tunneler && go build ./...

# Test (only connector has tests)
cd backend/connector && go test ./...

# Run a single test
cd backend/connector && go test ./run/... -run TestPolicyCacheDNSAllow

# Run controller
sudo TRUST_DOMAIN="mycorp.internal" \
  INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
  INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
  ADMIN_AUTH_TOKEN="7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" \
  INTERNAL_API_TOKEN="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
  CONTROLLER_ADDR="<host>:8443" \
  ADMIN_HTTP_ADDR="0.0.0.0:8081" \
  ./controller

# Enroll and run connector
./connector enroll   # One-time enrollment with controller
./connector run      # Long-running daemon
```

## Architecture

### Backend (Go)

Three services in `backend/` — each is its own Go module. The root `backend/go.mod` has module name `grpc`.

- **Controller** (`backend/controller/`): Internal CA + enrollment gRPC server on `:8443`, admin HTTP API on `:8081`. Manages SQLite DB, token store, ACLs, and policy distribution.
- **Connector** (`backend/connector/`): Enrolls with the controller, maintains a persistent bi-directional gRPC stream to receive policy updates, and listens on `:9443` for inbound tunneler connections.
- **Tunneler** (`backend/tunneler/`): Client-side service that enrolls and connects outbound to a connector.

All services use SPIFFE IDs under the trust domain:
- Connector: `spiffe://mycorp.internal/connector/<id>`
- Tunneler: `spiffe://mycorp.internal/tunneler/<id>`

**Controller internals:**
- `admin/handlers.go` — Core admin HTTP endpoints (tokens, connectors, resources, audit)
- `admin/ui_handlers.go` — UI-specific endpoints with CORS and formatted responses
- `admin/ui_routes.go` — Route registration
- `api/control_plane.go` — gRPC ControlPlane service (persistent bi-directional stream)
- `api/enroll.go` — gRPC EnrollmentService (EnrollConnector, EnrollTunneler, Renew)
- `state/` — In-memory + SQLite state (ACLs, connector registry, user/group management, token store)
- `ca/` — Certificate authority: `ca.go` for CA init, `issue.go` for cert issuance
- `gen/controllerpb/` — Generated gRPC code (from `proto/controller.proto`)

**gRPC services** (`backend/proto/controller.proto`):
- `EnrollmentService` — `EnrollConnector`, `EnrollTunneler`, `Renew`
- `ControlPlane` — `Connect` (bi-directional stream carrying heartbeats, policy snapshots, ACL decisions)

### Frontend (React + Express BFF)

**Tech stack**: React 19 + Vite (port 3000), Express BFF (port 3001), React Router v6, shadcn/ui, Tailwind CSS v4, better-sqlite3.

**Data flow:**
```
Browser (React/Vite :3000)
  → Express BFF (:3001) at /api/*
    → Local SQLite (frontend/ztna.db)   ← for UI-managed entities
    → Go backend HTTP API (:8081)       ← for controller-authoritative data
```

**Key lib files:**
- `lib/types.ts` — All shared TypeScript interfaces
- `lib/db.ts` — SQLite schema, auto-migrations, seeding (via `better-sqlite3`)
- `lib/proxy.ts` — Proxies requests to Go backend with Bearer token auth
- `lib/mock-api.ts` — Frontend API client that calls `/api/*` routes
- `lib/sign-in-policy.ts`, `lib/resource-policies.ts`, `lib/device-profiles.ts` — Policy state persisted to localStorage

**Express BFF routes** (`server/routes/`): `users`, `groups`, `resources`, `access-rules`, `connectors`, `remote-networks`, `tunnelers`, `subjects`, `tokens`, `service-accounts`, `policy`.

**Pages** under `src/pages/`: groups, users, resources, connectors, remote-networks, tunnelers, and policy sub-routes (resource-policies, sign-in, device-profiles).

### Environment Variables

| Variable | Service | Description |
|---|---|---|
| `VITE_API_BASE_URL` | Frontend | (empty by default — Vite proxy handles `/api`) |
| `BACKEND_URL` | Frontend BFF | Go backend URL (default: `http://localhost:8081`) |
| `ADMIN_AUTH_TOKEN` | Frontend + Controller | Bearer token for admin API |
| `PORT` | Frontend BFF | Express server port (default: 3001) |
| `INTERNAL_CA_CERT` | Controller/Connector/Tunneler | PEM CA certificate |
| `INTERNAL_CA_KEY` | Controller | PEM PKCS#8 CA private key |
| `CONTROLLER_ADDR` | Connector/Tunneler | `host:port` of controller gRPC |
| `ADMIN_HTTP_ADDR` | Controller | HTTP listen address (default `:8081`) |
| `DB_PATH` | Controller | SQLite database path (default: `ztna.db`) |
| `TRUST_DOMAIN` | All | SPIFFE trust domain (default: `mycorp.internal`) |
| `INTERNAL_API_TOKEN` | Controller/Connector | Internal gRPC auth token |
| `POLICY_SIGNING_KEY` | Controller | HMAC key for policy snapshots (defaults to `INTERNAL_API_TOKEN`) |
| `POLICY_SNAPSHOT_TTL_SECONDS` | Controller | Policy validity TTL (default: 600s) |

### Key Design Notes

- **Policy distribution**: Controller pushes signed policy snapshots over the persistent gRPC stream. Connectors cache these locally; connectors authorize tunneler connections against the cache. HMAC signatures (keyed by `POLICY_SIGNING_KEY`) prevent tampering.
- **Certificate lifecycle**: Workload certs are short-lived (~5 min); connectors and tunnelers auto-renew by re-calling `Renew` before expiry.
- **UI-specific HTTP endpoints** in `admin/ui_handlers.go` have CORS enabled and no auth requirement — they serve the frontend BFF. Admin endpoints under `/api/admin/*` require Bearer token auth.
- **Frontend schema migrations** in `lib/db.ts` run automatically at startup (ALTER TABLE patterns for live upgrades).
- **Policy state** (sign-in policy, resource policies, device profiles) is stored in localStorage, not the database.
- **Audit logging**: All access decisions logged to the `audit_logs` table in the controller's SQLite DB.
- **Systemd units** in `systemd/` harden connectors/tunnelers with `DynamicUser`, `ProtectSystem=full`, and restricted syscalls. Installation scripts are in `scripts/`.
