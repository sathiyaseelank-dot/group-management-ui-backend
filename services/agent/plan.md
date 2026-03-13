# Plan: Add Firewall Enforcer to Agent + Connector Forwarding

## Context

The agent service (formerly `services/tunneler/`, now `services/agent/`) connects to the connector via mTLS. When the connector receives a `policy_snapshot` from the controller, it extracts port rules from resources marked as **protected** in the admin dashboard, then broadcasts a `firewall_policy` message to all connected agents. Each agent applies nftables rules to protect those ports — allowing traffic only from loopback and the TUN interface, dropping all else.

## Architecture

```
Admin Dashboard --[PATCH firewall_status]--> Controller DB
Controller --[policy_snapshot]--> Connector --[firewall_policy]--> Agent
                                    │                                │
                              filter protected              apply nftables
                              extract ports                 rules per port
```

## Flow

1. Admin clicks **Protect** on a resource in the dashboard
2. Frontend sends `PATCH /api/resources/:id` with `{ "firewall_status": "protected" }`
3. Controller updates the resource and triggers a policy recompile
4. Connector receives `policy_snapshot`, filters to `firewall_status == "protected"` resources, extracts port rules
5. Connector broadcasts `firewall_policy` to all connected agents
6. Agent applies nftables rules (lo accept, tun accept, drop) for each port
7. Clicking **Ignore** reverses: sets `firewall_status = "unprotected"`, next policy sync removes the rules

## Changes Made

### Part 0: Rename `services/tunneler/` → `services/agent/`

- `git mv services/tunneler services/agent`
- Updated `Cargo.toml`: package name `agent-rs`, binary name `agent`
- Updated `main.rs`: CLI name and descriptions
- Updated `Makefile`: all tunneler targets → agent targets
- Updated `CLAUDE.md`: all tunneler references → agent

### Part 1: Admin Dashboard — Protect/Ignore UI

- **`lib/types.ts`**: Added `FirewallStatus` type and `firewallStatus` field to `Resource`
- **`lib/mock-api.ts`**: Added `setResourceFirewallStatus()` API function
- **`lib/db.ts`**: Added `firewall_status` column to resources table + migration
- **`server/routes/resources.ts`**: Added `PATCH` route, `firewallStatus` in GET formatting
- **`components/dashboard/resources/resources-list.tsx`**: Added Firewall column with shield status badge and Protect/Ignore toggle button

### Part 2: Controller — firewall_status in DB and API

- **`state/db.go`**: Added `firewall_status` column to resources schema + migration
- **`admin/ui_types.go`**: Added `FirewallStatus` field to `uiResource`
- **`admin/ui_helpers.go`**: Updated `scanUIResource` to scan `firewall_status`
- **`admin/ui_resources.go`**: Updated SELECT queries, added `PATCH` handler for toggling status
- **`api/policy_snapshot.go`**: Added `FirewallStatus` to `PolicyResource`, included in query and scanning

### Part 3: Connector — Firewall Policy Forwarding

- **`src/main.rs`**: Added `broadcast::channel` for firewall policies. On `policy_snapshot`, filters to protected resources, extracts port rules via `extract_port_rules()`, broadcasts to all agents.
- **`src/control_plane.rs`**: Added `firewall_tx` to `ConnectorControlPlane` struct. Each agent connection subscribes to the broadcast and forwards `firewall_policy` messages via `tokio::select!`.
- **`src/server.rs`**: Passes `firewall_tx` through `server_loop()` → `run_server()` → `ConnectorControlPlane`.
- **`src/policy/types.rs`**: Added `firewall_status` field to `PolicyResource`.

### Part 4: Agent — nftables Firewall Enforcer

- **`src/firewall.rs`** (new): Core module with `FirewallEnforcer` struct. Methods: `initialize()`, `sync_policy()`, `protect_port()`, `unprotect_port()`, `get_state()`, `restore_from_state()`, `cleanup_all()`. Uses `nft` CLI via `tokio::process::Command`. Three rules per port: lo accept, tun accept, drop.
- **`src/persistence.rs`**: Added `save_firewall_state()` / `load_firewall_state()` for JSON persistence to `STATE_DIRECTORY`.
- **`src/config.rs`**: Added `tun_name` field to `RunConfig` (from `TUN_NAME` env var, default `tun0`).
- **`src/run.rs`**: Creates `FirewallEnforcer`, restores state on startup, handles `firewall_policy` messages, cleans up nftables on `Ctrl+C`.
- **`src/main.rs`**: Added `mod firewall`.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TUN_NAME` | `tun0` | TUN interface name for firewall allow rules |
| `STATE_DIRECTORY` | (none) | Directory for persisting firewall state |

## Verification

1. `go build ./...` — controller compiles cleanly
2. `cargo build` — both connector and agent compile cleanly
3. `go test ./...` + `cargo test` — all tests pass
4. Manual integration: start controller → connector → agent (with sudo for nftables)
5. In admin dashboard, click **Protect** on a resource
6. Verify `nft list table inet ztna` shows correct rules on the agent
7. Click **Ignore** — verify rules are removed on next policy sync
