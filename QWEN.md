# ZTNA (Zero Trust Network Access) Project — Context Guide

This document provides comprehensive context for working with the ZTNA codebase — a production-ready Zero Trust Network Access system with mTLS authentication, SPIFFE IDs, and policy-based access control.

---

## 📋 Project Overview

**Purpose:** Zero Trust Network Access (ZTNA) system providing secure, policy-based remote access to protected resources without traditional VPN.

**Architecture:**
```
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│   Agent    │────────▶│  Connector  │────────▶│ Controller  │
│  (Client)   │  mTLS   │  (Gateway)  │  mTLS   │    (CA)     │
└─────────────┘         └─────────────┘         └─────────────┘
                                                        │
                                                        ▼
                                                  ┌──────────┐
                                                  │ Frontend │
                                                  │   (UI)   │
                                                  └──────────┘
```

**Trust Domain:** `spiffe://mycorp.internal`

---

## 📁 Project Structure

```
.
├── services/
│   ├── controller/          # Go — Certificate Authority & Control Plane (gRPC + HTTP API)
│   ├── connector/           # Rust — Gateway Service (TLS/QUIC tunnels)
│   ├── agent/               # Rust — Client Service (nftables firewall enforcer)
│   ├── ztna-client/         # Rust — Desktop CLI client (TUN/SOCKS5 mode)
│   └── ztna-client-mobile/  # Rust — UniFFI library for Android/iOS
├── apps/
│   ├── frontend/            # React + TypeScript — Management Dashboard
│   └── mobile/android/      # Kotlin + Jetpack Compose — Mobile client
├── shared/
│   ├── proto/               # Protobuf definitions (controller.proto)
│   └── configs/             # Shared configuration examples
├── systemd/                 # Systemd service unit files
├── scripts/                 # Deployment & setup scripts
├── docs/                    # Documentation
└── dist/                    # Build artifacts
```

---

## 🛠️ Tech Stack

| Component | Language | Key Technologies |
|-----------|----------|------------------|
| **Controller** | Go 1.25 | gRPC, SQLite/PostgreSQL, JWT, OAuth |
| **Connector** | Rust | Tokio, Tonic, TLS/QUIC, Quinn |
| **Agent** | Rust | Tokio, Tonic, nftables, SPIFFE |
| **ztna-client** | Rust | TUN (smoltcp), SOCKS5, Quinn, Keyring |
| **Frontend** | React 19 + TS | Vite, Express, Radix UI, TailwindCSS |
| **Android** | Kotlin | Jetpack Compose, UniFFI |

---

## 🚀 Build & Run Commands

### Root Makefile (from repo root)

```bash
make help              # Show all available commands
make build-all         # Build all components
make build-controller  # Build controller (Go)
make build-connector   # Build connector (Rust)
make build-agent       # Build agent (Rust)
make build-ztna-client # Build ztna-client CLI
make build-frontend    # Build frontend (React)

make dev-controller    # Run controller in dev mode (loads .env)
make dev-connector     # Run connector in dev mode
make dev-agent         # Run agent in dev mode
make dev-ztna-client   # Run ztna-client CLI
make dev-frontend      # Run frontend in dev mode

make test-all          # Test all components
make test-controller   # Test controller
make test-connector    # Test connector
make test-agent        # Test agent
make test-frontend     # Test frontend

make clean             # Clean build artifacts
make clean-all         # Clean everything including deps
```

### Component-Specific Commands

**Controller (`services/controller/`):**
```bash
go build ./...
go test ./...
# Run (requires env vars)
sudo TRUST_DOMAIN="mycorp.internal" \
  INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
  INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
  JWT_SECRET="<secret>" \
  DATABASE_URL="postgres://..." \
  go run .
```

**Connector (`services/connector/`):**
```bash
cargo build --release
cargo test
cargo run
```

**Agent (`services/agent/`):**
```bash
cargo build --release
cargo test
cargo run
```

**ztna-client (`services/ztna-client/`):**
```bash
cargo build --release
cargo run                        # default: connects to http://localhost:8081

# TUN mode (default, requires root)
sudo CONTROLLER_URL=http://localhost:8081 \
ZTNA_TENANT=<workspace-slug> \
cargo run

# SOCKS5 mode (unprivileged fallback)
CONTROLLER_URL=http://localhost:8081 \
ZTNA_TENANT=<workspace-slug> \
ZTNA_MODE=socks5 \
SOCKS5_ADDR=127.0.0.1:1080 \
cargo run
```

**Frontend (`apps/frontend/`):**
```bash
npm run dev     # Start Vite (port 3000) + Express server (port 3001)
npm run build   # Vite production build
npm run start   # Run Express server only (production)
npm run lint    # ESLint
npm run test    # Jest
```

**Android App (`mobile/android/`):**
```bash
./gradlew buildRustAll           # cross-compile Rust libraries
./gradlew generateUniffiBindings # regenerate Kotlin bindings
./gradlew assembleDebug          # build APK
./gradlew installDebug           # install on connected device
```

### PostgreSQL (Docker)

```bash
docker compose up -d              # Start PostgreSQL (postgres:16-alpine, port 5432)
docker exec ztna-postgres psql -U ztnaadmin -d ztna -c "<query>"
```

---

## 🔧 Environment Variables

### Controller (`services/controller/.env`)

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | ✅ | PostgreSQL connection string |
| `INTERNAL_CA_CERT` | ✅ | PEM CA certificate |
| `INTERNAL_CA_KEY` | ✅ | PEM PKCS#8 CA private key |
| `JWT_SECRET` | | HMAC secret (derived from `INTERNAL_CA_KEY` if unset) |
| `TRUST_DOMAIN` | | SPIFFE trust domain (default: `mycorp.internal`) |
| `ADMIN_HTTP_ADDR` | | HTTP listen address (default: `:8081`) |
| `OAUTH_CALLBACK_ADDR` | | Secondary listener for OAuth callbacks (default: `:8080`) |
| `GOOGLE_CLIENT_ID` | | Admin Google OAuth app client ID |
| `GOOGLE_CLIENT_SECRET` | | Admin Google OAuth app secret |
| `CLIENT_GOOGLE_CLIENT_ID` | | Client Google OAuth app client ID (PKCE flows) |
| `CLIENT_GOOGLE_CLIENT_SECRET` | | Client Google OAuth app secret |
| `DASHBOARD_URL` | | Frontend URL for post-OAuth redirect |
| `INVITE_BASE_URL` | | Base URL for invite links |
| `IDP_ENCRYPTION_KEY` | | AES-256 key for workspace IdP secrets |

### Frontend (`apps/frontend/.env`)

| Variable | Description |
|----------|-------------|
| `NEXT_PUBLIC_API_BASE_URL` | Go controller URL (default: `http://localhost:8081`) |
| `VITE_CONTROLLER_URL` | Controller base URL for browser redirects |

### Connector / Agent

| Variable | Description |
|----------|-------------|
| `CONTROLLER_ADDR` | `host:port` of controller gRPC (`:8443`) |
| `INTERNAL_CA_CERT` | PEM CA certificate |
| `TRUST_DOMAIN` | SPIFFE trust domain |

---

## 🏗️ Architecture Details

### Services Overview

| Service | Port | Purpose |
|---------|------|---------|
| **Controller** | `:8081` (HTTP), `:8443` (gRPC), `:8080` (OAuth) | CA, enrollment, ACLs, OAuth, device auth |
| **Connector** | `:9443` (agent gRPC), `:9444/tcp` (TLS), `:9444/udp` (QUIC) | Gateway between agents/devices and resources |
| **Agent** | outbound only | Server-side agent: connects to connector, enforces nftables |
| **ztna-client** | N/A | Desktop CLI: device enrollment, TUN/SOCKS5 split-tunnel |
| **Frontend** | `:3000` (Vite), `:3001` (Express) | Admin dashboard + user portal |

### SPIFFE Identity

All services use SPIFFE IDs under trust domain `spiffe://mycorp.internal`:
- **Connector:** `spiffe://mycorp.internal/connector/<id>`
- **Agent:** `spiffe://mycorp.internal/tunneler/<id>`
- **Device:** `spiffe://mycorp.internal/device/<user_id>/<device_id>`

### Split-Tunnel Traffic Flow

**TUN mode (default):**
1. App sends traffic to resource IP → kernel routes through TUN device `ztna0`
2. smoltcp (TCP) or raw UDP parsing (etherparse) handles packets
3. ACL check via `POST /api/device/check-access` on controller
4. QUIC preferred (3s timeout), falls back to TLS tunnel to connector `:9444`
5. Connector routes to agent protecting target resource
6. Agent forwards traffic to resource

**SOCKS5 mode (unprivileged fallback):**
1. Application connects through SOCKS5 proxy on `127.0.0.1:1080`
2. ACL check via controller
3. TLS tunnel to connector `:9444`
4. Connector routes to agent

### OAuth / Authentication Flows

| Flow | Google App | PKCE | State Prefix | Ends at |
|------|------------|------|--------------|---------|
| Admin login | Admin app (`GOOGLE_CLIENT_ID`) | No | (none) | Dashboard cookie + redirect |
| Device PKCE (mobile) | Client app (`CLIENT_GOOGLE_CLIENT_ID`) | Yes (S256) | `"device:"` | `ztna://callback?code=...` |
| Invite PKCE (browser) | Client app (`CLIENT_GOOGLE_CLIENT_ID`) | Yes (S256) | `"invitepkce:"` | Frontend `/app/welcome?token=...` |

**JWT `aud` claim:** `"admin"` for dashboard sessions, `"device"` for mobile sessions.

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

---

## 🗄️ Database Schema (PostgreSQL)

All tables created via `initSchemaDialect()` in `state/db.go` on startup.

**Core tables:** `connectors`, `tunnelers`, `resources`, `authorizations`, `tokens`, `audit_logs`, `users`, `user_groups`, `user_group_members`, `remote_networks`, `access_rules`, `access_rule_groups`, `service_accounts`

**Auth/identity tables:** `invite_tokens`, `workspace_invites`, `workspaces`, `workspace_members`, `identity_providers`, `sessions`, `device_auth_requests`

**Device posture tables:** `device_posture`, `device_trusted_profiles`

---

## 📝 Development Conventions

### Git Workflow

**Branch Strategy:**
- `main` — Production-ready code
- `develop` — Integration branch
- `feature/<component>-<description>` — Feature branches

**Commit Messages (Conventional Commits with scopes):**
```bash
feat(controller): add user enrollment endpoint
fix(connector): resolve connection timeout issue
test(agent): add integration tests
chore(frontend): update dependencies
docs(api): update authentication flow
```

### Protobuf Changes

When updating `shared/proto/controller.proto`:
1. Modify proto file
2. Regenerate code for all services (Go, Rust)
3. Update affected components
4. Coordinate with team

### Testing Practices

- **Go:** `cd services/controller && go test ./...`
- **Rust:** `cd services/connector && cargo test`
- **Frontend:** `cd apps/frontend && npm test`

Prefer `make test-<component>` before pushing.

### Code Style

- **Go:** `gofmt` formatting
- **Rust:** `rustfmt` formatting
- **Frontend:** ESLint via `npm run lint`

---

## 🔐 Security Notes

- mTLS authentication for all connections
- SPIFFE IDs for identity management
- Policy-based access control
- Certificate rotation
- Secure enrollment process with PKCE
- OAuth callback binds to `127.0.0.1` by default
- Tenant slugs validated to prevent path traversal
- Callback endpoints rate-limited (10 req/60s)
- Service token files use `0640` permissions

**Never commit secrets.** Use `shared/configs/.env.example` for new variables.

---

## 📚 Key Documentation

- `README.md` — Project overview
- `CLAUDE.md` — Detailed architecture and development notes
- `QUICKSTART.md` — Team onboarding guide
- `docs/architecture.md` — System architecture
- `docs/development.md` — Development guide
- `services/controller/RUN.md` — Controller setup
- `services/connector/run.md` — Connector setup
- `services/ztna-client/client-run.md` — ztna-client usage

---

## 🐛 Common Troubleshooting

### Build Fails?
```bash
make clean
make build-<component>
```

### Dependencies Issue?
```bash
# Go
cd services/controller && go mod tidy

# Rust
cd services/connector && cargo update

# Node
cd apps/frontend && npm install
```

### Can't Find Command?
```bash
make help    # Shows all available commands
```

### PostgreSQL Connection?
```bash
docker compose up -d
# Check: docker ps | grep ztna-postgres
```

---

## 🎯 Component Quick Reference

### Controller (Go)
- **Entry:** `services/controller/main.go`
- **Handlers:** `services/controller/admin/`
- **State/DB:** `services/controller/state/`
- **Proto gen:** `services/controller/gen/`

### Connector (Rust)
- **Entry:** `services/connector/src/main.rs`
- **Build:** `cargo build --release`
- **Output:** `dist/connector`

### Agent (Rust)
- **Entry:** `services/agent/src/main.rs`
- **Build:** `cargo build --release`
- **Output:** `dist/agent`

### ztna-client (Rust)
- **Entry:** `services/ztna-client/src/main.rs`
- **Build:** `cargo build --release`
- **Output:** `dist/ztna-client`

### Frontend (React)
- **Entry:** `apps/frontend/src/App.tsx`
- **Server:** `apps/frontend/server/index.ts`
- **Routes:** `apps/frontend/src/pages/`

---

**Built with ❤️ by the ZTNA Team**
