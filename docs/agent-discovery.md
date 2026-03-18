# Agent Discovery System

Complete documentation of the agent-side service discovery, controller persistence, auto-prune lifecycle, workspace scoping, and frontend presentation.

---

## Table of Contents

1. [High-Level Flow](#1-high-level-flow)
2. [Agent-Side Scanning](#2-agent-side-scanning)
3. [Fingerprint Hashing & Change Detection](#3-fingerprint-hashing--change-detection)
4. [gRPC Message Types](#4-grpc-message-types)
5. [Controller gRPC Handler](#5-controller-grpc-handler)
6. [State Layer (Database)](#6-state-layer-database)
7. [Status Lifecycle](#7-status-lifecycle)
8. [Auto-Prune Goroutine](#8-auto-prune-goroutine)
9. [Workspace Scoping (Multi-Tenancy)](#9-workspace-scoping-multi-tenancy)
10. [Admin HTTP API](#10-admin-http-api)
11. [Frontend BFF Proxy](#11-frontend-bff-proxy)
12. [Frontend UI](#12-frontend-ui)
13. [Timing Reference](#13-timing-reference)
14. [Data Flow Diagram](#14-data-flow-diagram)

---

## 1. High-Level Flow

```
Agent (Rust)                    Controller (Go)                  Frontend (React)
─────────────                   ───────────────                  ────────────────
Scan /proc/net/tcp every 30s
Compute fingerprint hash
  │
  ├─ hash changed ──────────►  agent_discovery_report
  │                             Upsert rows (status=active)
  │                             Set workspace_id from tunnelers
  │
  ├─ ports disappeared ─────►  agent_discovery_gone
  │                             Mark rows status=gone
  │
  ├─ hash unchanged 5min ───►  agent_discovery_heartbeat
  │                             Bump last_seen on active rows
  │
  │                             Prune goroutine (hourly)
  │                             Delete gone rows past retention
  │                                                              Poll every 15s
  │                                                              GET /results
  │                                                              GET /summary
  │                                                              PATCH dismiss/undismiss
  │                                                              DELETE purge
```

---

## 2. Agent-Side Scanning

**File:** `services/agent/src/discovery.rs`

### What Gets Scanned

The agent reads the kernel's TCP socket table to find all listening services.

**Linux (primary path):**
- Parses `/proc/net/tcp` (IPv4) and `/proc/net/tcp6` (IPv6) directly
- Filters for LISTEN state (state code `0A` in the hex table)
- Extracts port number and bound IP address from hex-encoded fields
- No `CAP_SYS_PTRACE` or elevated privileges required — `/proc/net/tcp` is world-readable

**Non-Linux fallback:**
- Uses the `listeners` crate to enumerate listening sockets

### What Gets Filtered Out

A service is only reported if `is_externally_listening()` returns true:

| Bound IP | Reported? | Reason |
|----------|-----------|--------|
| `0.0.0.0` | Yes | Listening on all interfaces |
| `::` | Yes | Listening on all interfaces (IPv6) |
| `192.168.x.x` etc. | Yes | Bound to a specific non-loopback interface |
| `127.0.0.1` | No | Loopback only — not network-accessible |
| `::1` | No | Loopback only (IPv6) |

Additionally, two more filters apply before reporting:

1. **Protected ports** — Ports that the agent's firewall enforcer is currently protecting are excluded. These are already managed resources.
2. **Already-sent ports** — Ports reported in a previous scan cycle are not re-reported until they go away and come back. This is tracked in the `sent_services: HashSet<(u16, String)>` set.

### Output

Each discovered service is a struct:

```rust
pub struct DiscoveredService {
    pub protocol: &'static str,  // Always "tcp" currently
    pub port: u16,
    pub bound_ip: String,        // "0.0.0.0", "::", or a specific IP
}
```

---

## 3. Fingerprint Hashing & Change Detection

**File:** `services/agent/src/discovery.rs` — `compute_fingerprint()`

### How the Hash Works

The agent computes a **u64 fingerprint** of the current port set to detect changes cheaply without comparing full lists.

```rust
pub fn compute_fingerprint(ports: &HashSet<(u16, String)>) -> u64 {
    let mut sorted: Vec<_> = ports.iter().collect();
    sorted.sort();
    let mut hasher = DefaultHasher::new();
    for item in sorted {
        item.hash(&mut hasher);
    }
    hasher.finish()
}
```

**Steps:**
1. Collect all `(port, protocol)` tuples into a sorted vec (deterministic ordering)
2. Feed each tuple into Rust's `DefaultHasher` (SipHash-based)
3. Return the final u64 digest

### How Change Detection Uses It

Each 30-second scan cycle in `run_discovery_scan()`:

```
current_ports = scan() - protected_ports - already_sent_ports
new_fingerprint = compute_fingerprint(current_ports)

if new_fingerprint == last_fingerprint:
    return early (no change)
```

This short-circuit avoids:
- Redundant JSON serialization
- Redundant gRPC messages
- Redundant database upserts

The fingerprint only covers the *unprotected, unsent* port set. When a port goes away and then returns, it gets removed from `sent_services`, so it re-enters the fingerprint and will be re-reported.

### State Maintained Across Scans

| Variable | Type | Purpose |
|----------|------|---------|
| `sent_services` | `HashSet<(u16, String)>` | Ports already reported — prevents re-reporting |
| `previous_ports` | `HashSet<(u16, String)>` | Ports from last scan — used to detect gone services |
| `last_fingerprint` | `u64` | Hash of last port set — early exit if unchanged |
| `last_report_time` | `Instant` | When last report/heartbeat was sent — triggers heartbeat after 5min |

All four are initialized at connection time and reset if the agent reconnects.

---

## 4. gRPC Message Types

All messages use the `ControlMessage` protobuf envelope:

```protobuf
message ControlMessage {
    string type = 1;       // Message type discriminator
    bytes payload = 2;     // JSON-serialized body
    string connector_id = 3;
    string private_ip = 4;
    string status = 5;
}
```

### `agent_discovery_report`

Sent when new externally-listening services are detected.

```json
{
  "agent_id": "agt-abc123",
  "services": [
    { "protocol": "tcp", "port": 8080, "bound_ip": "0.0.0.0" },
    { "protocol": "tcp", "port": 3306, "bound_ip": "192.168.1.50" }
  ]
}
```

**Trigger:** Fingerprint changed AND there are new (unsent) services.

### `agent_discovery_gone`

Sent when previously-reported services stop listening.

```json
{
  "agent_id": "agt-abc123",
  "services": [
    { "protocol": "tcp", "port": 3306 }
  ]
}
```

**Trigger:** Services present in `previous_ports` are absent in `current_ports`.

When a service is reported gone:
- It is removed from `sent_services`, allowing it to be re-reported if it comes back
- The controller marks the row `status = 'gone'`

### `agent_discovery_heartbeat`

Sent when the port set has been stable (fingerprint unchanged) for 5 minutes.

```json
{
  "agent_id": "agt-abc123",
  "fingerprint": 12345678901234
}
```

**Trigger:** `Instant::now() - last_report_time > 5 minutes` AND no discovery report was sent this cycle.

**Purpose:** Keeps `last_seen` fresh on the controller so active services from quiet agents are not mistaken for stale data.

---

## 5. Controller gRPC Handler

**File:** `services/controller/api/control_plane.go` — `Connect()` method

The controller's bidirectional gRPC stream processes all three message types in a receive loop.

### `agent_discovery_report` Handler

```
1. Parse JSON payload → { agent_id, services[] }
2. Look up agent's workspace_id:
   SELECT workspace_id FROM tunnelers WHERE id = ?
   (done ONCE per report, outside the service loop)
3. For each service:
   UpsertAgentDiscoveredService({
     AgentID, Port, Protocol, BoundIP, WorkspaceID
   })
```

The upsert uses `ON CONFLICT(agent_id, port, protocol)` so:
- New services get inserted with `status = 'active'`, `first_seen = now`, `last_seen = now`
- Returning services get updated: `status` reset to `'active'`, `last_seen = now`, `workspace_id` updated

### `agent_discovery_gone` Handler

```
1. Parse JSON payload → { agent_id, services[] }
2. Extract port list and protocol
3. MarkServicesGone(db, agentID, ports, protocol)
   → UPDATE ... SET status = 'gone' WHERE agent_id = ? AND port = ? AND protocol = ? AND status = 'active'
```

Only transitions `active → gone`. Already-gone services are unaffected.

### `agent_discovery_heartbeat` Handler

```
1. Parse JSON payload → { agent_id }
2. TouchAgentDiscoveryLastSeen(db, agentID)
   → UPDATE ... SET last_seen = now WHERE agent_id = ? AND status = 'active'
```

Bumps `last_seen` on all active services for the agent. Does not create rows or change status.

---

## 6. State Layer (Database)

**File:** `services/controller/state/agent_discovery.go`

### Schema

```sql
CREATE TABLE agent_discovered_services (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id    TEXT NOT NULL,
    port        INTEGER NOT NULL,
    protocol    TEXT NOT NULL,
    bound_ip    TEXT NOT NULL DEFAULT '',
    first_seen  INTEGER NOT NULL,          -- Unix timestamp
    last_seen   INTEGER NOT NULL,          -- Unix timestamp
    workspace_id TEXT NOT NULL DEFAULT '',
    dismissed   INTEGER NOT NULL DEFAULT 0, -- 0 or 1
    status      TEXT NOT NULL DEFAULT 'active',  -- 'active' or 'gone'
    UNIQUE(agent_id, port, protocol)
);
```

### Core Write Functions

| Function | SQL | Notes |
|----------|-----|-------|
| `UpsertAgentDiscoveredService` | `INSERT ... ON CONFLICT DO UPDATE` | Sets `status='active'` on both insert and update. Updates `workspace_id` on conflict. |
| `TouchAgentDiscoveryLastSeen` | `UPDATE SET last_seen=? WHERE agent_id=? AND status='active'` | Heartbeat handler. Only touches active rows. |
| `MarkServicesGone` | `UPDATE SET status='gone' WHERE agent_id=? AND port=? AND protocol=? AND status='active'` | Per-port. Only transitions active→gone. |
| `DismissService` | `UPDATE SET dismissed=1 WHERE id=? [AND workspace_id=?]` | UI action. Workspace-scoped. |
| `UndismissService` | `UPDATE SET dismissed=0 WHERE id=? [AND workspace_id=?]` | UI action. Workspace-scoped. |
| `PruneDiscoveredServices` | `DELETE WHERE status='gone' AND last_seen < ?` | Background cleanup. Global (no workspace filter). |
| `PurgeDiscoveredServices` | `DELETE WHERE [agent_id=?] [AND workspace_id=?]` | Manual cleanup via UI. Workspace-scoped. |

### Query Functions

| Function | Filters | Used By |
|----------|---------|---------|
| `ListAgentDiscoveredServices` | `agent_id=? AND dismissed=0 [AND workspace_id=?]` | Results endpoint (single agent) |
| `ListAgentDiscoveredServicesAll` | `agent_id=? [AND workspace_id=?]` | Results endpoint with `include_dismissed=true` |
| `ListAllAgentDiscoveredServices` | `dismissed=0 [AND workspace_id=?]` | Results endpoint (all agents) |
| `ListAllAgentDiscoveredServicesIncludingDismissed` | `[workspace_id=?]` | Results endpoint (all, with dismissed) |
| `GetDiscoverySummary` | 4 count queries, all workspace-filtered | Summary endpoint |

All query functions take a `workspaceID string` parameter. When empty (super-admin via `ADMIN_AUTH_TOKEN`), the workspace filter is omitted — returns data across all workspaces.

---

## 7. Status Lifecycle

A discovered service row transitions through these states:

```
                    ┌──────────────────────────────────────┐
                    │                                      │
                    ▼                                      │
  ┌─────────────────────┐     agent_discovery_gone    ┌────┴────┐
  │   active             │ ─────────────────────────► │  gone    │
  │   dismissed=0        │                            │          │
  └──────┬──────────────┘                            └────┬────┘
         │       ▲                                        │
         │       │ agent_discovery_report                 │
         │       │ (re-appears)                           │
         │       └────────────────────────────────────────┘
         │
         │  UI dismiss
         ▼
  ┌─────────────────────┐
  │   active             │     (still tracked, hidden from
  │   dismissed=1        │      default UI views)
  └─────────────────────┘
         │
         │  UI undismiss
         ▼
    (back to active, dismissed=0)
```

### Transitions

| From | To | Trigger |
|------|----|---------|
| *(new)* | `active, dismissed=0` | `agent_discovery_report` — first time seeing this port |
| `active` | `gone` | `agent_discovery_gone` — agent no longer detects port |
| `gone` | `active` | `agent_discovery_report` — port reappears (upsert resets status) |
| `active` | `active, dismissed=1` | UI dismiss action |
| `active, dismissed=1` | `active, dismissed=0` | UI undismiss action |
| `gone` | *(deleted)* | Auto-prune after retention window expires |
| any | *(deleted)* | Manual purge via UI |

### Key Invariant

The prune goroutine **only deletes `status='gone'` rows**. Active services from offline agents are preserved indefinitely — they remain visible in the UI with an "Agent Offline" indicator until the agent comes back online or an admin manually purges them.

---

## 8. Auto-Prune Goroutine

**File:** `services/controller/main.go`

```go
go func() {
    retentionHours := 72  // default: 3 days
    if v := os.Getenv("DISCOVERY_RETENTION_HOURS"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 {
            retentionHours = n
        }
    }
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    for range ticker.C {
        cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
        n, err := state.PruneDiscoveredServices(db, cutoff)
        // ...
    }
}()
```

| Setting | Value |
|---------|-------|
| **Tick interval** | 1 hour |
| **Default retention** | 72 hours (3 days) |
| **Env override** | `DISCOVERY_RETENTION_HOURS` |
| **Target rows** | `status = 'gone' AND last_seen < cutoff` |
| **Workspace scoped?** | No — global cleanup across all workspaces |

### Why Only Gone Rows?

If the prune deleted all rows with old `last_seen`, it would wipe out active services from agents that happen to be offline (network outage, host reboot, etc.). Those services are still real — the admin needs to see them. Only services explicitly marked "gone" by the agent (meaning the port is confirmed to have stopped listening) should be garbage-collected.

---

## 9. Workspace Scoping (Multi-Tenancy)

### Problem

The system is multi-tenant — each workspace is an isolated security domain. Without scoping, workspace A's admin could see (or dismiss/purge) workspace B's discovered services.

### How Workspace ID Gets Set

1. Agent enrolls and gets stored in `tunnelers` table with a `workspace_id`
2. When agent sends `agent_discovery_report` via gRPC, the controller handler looks up:
   ```sql
   SELECT workspace_id FROM tunnelers WHERE id = ?
   ```
3. This `workspace_id` is passed into `UpsertAgentDiscoveredService` and written to the row
4. On `ON CONFLICT` updates, `workspace_id` is also refreshed from the upsert

### How Queries Are Scoped

All query functions use a conditional `wsFilter` helper:

```go
func wsFilter(query string, args []any, workspaceID string) (string, []any) {
    if workspaceID != "" {
        query += " AND workspace_id = ?"
        args = append(args, workspaceID)
    }
    return query, args
}
```

When `workspaceID == ""` (super-admin using `ADMIN_AUTH_TOKEN` without workspace JWT claims), no filter is applied — all data is returned. This preserves backward compatibility.

### How Mutations Are Scoped

`DismissService`, `UndismissService`, and `PurgeDiscoveredServices` all add `AND workspace_id = ?` when the workspace is set, preventing cross-tenant mutations.

### Unmanaged Count — Cross-Tenant Accuracy

The "unmanaged" summary query checks whether a discovered port/protocol exists in the `resources` table. When workspace-scoped, the subquery also matches on workspace:

```sql
NOT EXISTS (
    SELECT 1 FROM resources r
    WHERE r.port_from = ds.port
      AND LOWER(r.protocol) = LOWER(ds.protocol)
      AND r.workspace_id = ds.workspace_id   -- only when scoped
)
```

This prevents workspace B's resources from incorrectly marking workspace A's discovered services as "managed."

### Route Middleware

Agent discovery routes use `withWorkspaceContext` (same as all other UI routes):

```go
ws := s.withWorkspaceContext
mux.Handle("/api/admin/agent-discovery/results", withCORS(ws(http.HandlerFunc(...))))
```

The `withWorkspaceContext` middleware:
1. Extracts JWT from cookie or `Authorization: Bearer` header
2. Decodes workspace claims (`uid`, `wid`, `wslug`, `wrole`)
3. Adds them to request context
4. If no JWT or no workspace claims → context has empty workspace (backward compatible)

Handlers then call `workspaceIDFromContext(r.Context())` to get the workspace ID.

---

## 10. Admin HTTP API

**File:** `services/controller/admin/handlers_agent_discovery.go`

### Endpoints

#### `GET /api/admin/agent-discovery/results`

List discovered services.

| Query Parameter | Type | Default | Description |
|----------------|------|---------|-------------|
| `agent_id` | string | *(all agents)* | Filter by specific agent |
| `include_dismissed` | `"true"` | `false` | Include dismissed services |

**Response:** `200 OK`
```json
[
  {
    "id": 42,
    "agent_id": "agt-abc123",
    "port": 8080,
    "protocol": "tcp",
    "bound_ip": "0.0.0.0",
    "first_seen": 1710000000,
    "last_seen": 1710003600,
    "workspace_id": "ws-xyz",
    "dismissed": 0,
    "status": "active"
  }
]
```

#### `GET /api/admin/agent-discovery/summary`

Aggregate dashboard stats.

**Response:** `200 OK`
```json
{
  "total": 15,
  "new_24h": 3,
  "unmanaged": 8,
  "gone": 2
}
```

| Field | Meaning |
|-------|---------|
| `total` | Active, non-dismissed services |
| `new_24h` | Active, non-dismissed, `first_seen` within last 24 hours |
| `unmanaged` | Active, non-dismissed, no matching resource by port+protocol |
| `gone` | Non-dismissed services with `status='gone'` |

#### `PATCH /api/admin/agent-discovery/results/{id}/dismiss`

Hide a service from default views. Returns `{"status": "ok"}`.

#### `PATCH /api/admin/agent-discovery/results/{id}/undismiss`

Restore a dismissed service. Returns `{"status": "ok"}`.

#### `DELETE /api/admin/agent-discovery/results`

Purge (permanently delete) discovered services.

| Query Parameter | Type | Default | Description |
|----------------|------|---------|-------------|
| `agent_id` | string | *(all)* | Only purge services for this agent |

**Response:** `200 OK`
```json
{ "status": "ok", "deleted": 12 }
```

---

## 11. Frontend BFF Proxy

**File:** `apps/frontend/server/routes/agent-discovery.ts`

The Express BFF server proxies frontend requests to the Go controller, translating between camelCase (frontend) and snake_case (backend).

| Frontend Route | Controller Route |
|---------------|-----------------|
| `GET /api/agent-discovery/results` | `GET /api/admin/agent-discovery/results` |
| `GET /api/agent-discovery/summary` | `GET /api/admin/agent-discovery/summary` |
| `PATCH /api/agent-discovery/results/:id/dismiss` | `PATCH /api/admin/agent-discovery/results/:id/dismiss` |
| `PATCH /api/agent-discovery/results/:id/undismiss` | `PATCH /api/admin/agent-discovery/results/:id/undismiss` |
| `DELETE /api/agent-discovery/results` | `DELETE /api/admin/agent-discovery/results` |

**Field mapping on response:**

| Backend (snake_case) | Frontend (camelCase) |
|---------------------|---------------------|
| `agent_id` | `agentId` |
| `bound_ip` | `boundIp` |
| `first_seen` | `firstSeen` |
| `last_seen` | `lastSeen` |
| `workspace_id` | `workspaceId` |
| `dismissed` (0/1) | `dismissed` (boolean) |

---

## 12. Frontend UI

**File:** `apps/frontend/src/pages/resources/AgentDiscoveryPage.tsx`

### Summary Cards

Four metric cards at the top of the page, driven by the `/summary` endpoint:

| Card | Color | Source |
|------|-------|--------|
| Total Services | Blue | `summary.total` |
| New (24h) | Green | `summary.new_24h` |
| Unmanaged | Amber | `summary.unmanaged` |
| Gone | Red | `summary.gone` |

### Service Table

Each row shows:

| Column | Logic |
|--------|-------|
| **Status dot** | Green = active + agent online; Gray = active + agent offline; Red = gone |
| **Address** | If `boundIp` is wildcard (`0.0.0.0`, `::`) → agent hostname; else → `boundIp` |
| **Port** | Port number |
| **Protocol** | `tcp` (uppercased in display) |
| **Managed?** | Checks if a resource exists with matching port + protocol |
| **NEW badge** | Shown if `firstSeen > lastVisited` (localStorage timestamp) |

### Polling

The page polls every **15 seconds** for both results and summary data.

### Bulk Operations

Users can select multiple unmanaged, non-gone, non-dismissed services and add them as resources in batch. The resource name is auto-generated as `{agent-name}-{port}`, and the remote network is resolved from the agent's associations.

---

## 13. Timing Reference

| Component | Interval | Description |
|-----------|----------|-------------|
| Agent scan | 30s | Read `/proc/net/tcp` and compare fingerprint |
| Agent heartbeat | 5min | Sent when fingerprint unchanged for 5 minutes |
| Agent connection heartbeat | 10s | Separate `agent_heartbeat` for liveness |
| Controller prune | 1h | Delete gone rows past retention |
| Controller retention | 72h default | Configurable via `DISCOVERY_RETENTION_HOURS` |
| Frontend poll | 15s | Fetch results + summary |

---

## 14. Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│  AGENT HOST                                                     │
│                                                                 │
│  /proc/net/tcp ──► discover_exposed_services()                  │
│  /proc/net/tcp6    │                                            │
│                    ▼                                            │
│              Filter: external only, not protected, not sent     │
│                    │                                            │
│                    ▼                                            │
│              compute_fingerprint() ──► u64 hash                 │
│                    │                                            │
│           ┌────────┴────────┐                                   │
│           │ changed?        │                                   │
│           │                 │                                   │
│       YES ▼             NO  ▼                                   │
│  Detect gone ports    5min elapsed? ──YES──► heartbeat msg      │
│  Send gone msg                    └──NO───► skip                │
│  Send report msg                                                │
└────────┬──────────────────────────────────────┬─────────────────┘
         │ gRPC (mTLS via connector)            │
         ▼                                      ▼
┌────────────────────────────────────────────────────────────────┐
│  CONTROLLER                                                    │
│                                                                │
│  Connect() handler                                             │
│    ├─ agent_discovery_report                                   │
│    │    └─ lookup workspace_id from tunnelers                  │
│    │    └─ UpsertAgentDiscoveredService (status=active)        │
│    │                                                           │
│    ├─ agent_discovery_gone                                     │
│    │    └─ MarkServicesGone (status=gone)                      │
│    │                                                           │
│    └─ agent_discovery_heartbeat                                │
│         └─ TouchAgentDiscoveryLastSeen (bump last_seen)        │
│                                                                │
│  Prune goroutine (hourly)                                      │
│    └─ DELETE WHERE status='gone' AND last_seen < cutoff        │
│                                                                │
│  Admin HTTP API                                                │
│    └─ All queries scoped by workspace_id from JWT context      │
└────────┬───────────────────────────────────────────────────────┘
         │ HTTP (proxied through BFF)
         ▼
┌────────────────────────────────────────────────────────────────┐
│  FRONTEND                                                      │
│                                                                │
│  Express BFF (port 3001)                                       │
│    └─ /api/agent-discovery/* ──proxy──► /api/admin/agent-*     │
│    └─ snake_case → camelCase field mapping                     │
│                                                                │
│  React UI (port 3000)                                          │
│    └─ Polls every 15s                                          │
│    └─ Summary cards: total, new_24h, unmanaged, gone           │
│    └─ Status dots: green/gray/red                              │
│    └─ Actions: dismiss, undismiss, purge, add-as-resource      │
└────────────────────────────────────────────────────────────────┘
```
