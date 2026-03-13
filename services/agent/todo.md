# Agent Firewall Enforcer — Issues & Fixes TODO

## Current Status
- Agent enrolls successfully, nftables table/chain created
- Agent gets "agent not in allowlist" after controller/connector restarts
- Firewall policy messages arrive but with empty `protected_ports`
- nftables rules never applied

---

## Issue 1: Agent allowlist not persisted across controller restarts (CRITICAL)

**Root cause:** `NotifyAgentAllowed()` only adds the agent to the in-memory
`AgentRegistry` and broadcasts to connectors. It does NOT write to the
`tunnelers` DB table. The DB is only written when heartbeats arrive via
`SaveAgentToDB()` — but heartbeats require the agent to be connected, which
requires the allowlist. Chicken-and-egg problem.

**What happens:**
1. Agent enrolls → `NotifyAgentAllowed()` → in-memory only
2. Controller restarts → in-memory registry lost
3. `LoadAgentRegistryFromDB()` loads from `tunnelers` table → empty (never written)
4. Connector gets empty allowlist → rejects agent

**Fix:** Persist agent to DB inside `NotifyAgentAllowed()`:

```go
// In api/control_plane.go → NotifyAgentAllowed()
func (s *ControlPlaneServer) NotifyAgentAllowed(agentID, spiffeID string) {
    if s.agents != nil {
        s.agents.Add(agentID, spiffeID)
    }
    // Persist to DB so allowlist survives controller restarts
    if s.db != nil {
        _, _ = s.db.Exec(
            state.Rebind(`INSERT INTO tunnelers (id, spiffe_id, connector_id, last_seen)
            VALUES (?, ?, '', ?)
            ON CONFLICT(id) DO UPDATE SET spiffe_id=excluded.spiffe_id, last_seen=excluded.last_seen`),
            agentID, spiffeID, time.Now().UTC().Unix(),
        )
    }
    // ... existing broadcast code ...
}
```

**Files:** `services/controller/api/control_plane.go`

---

## Issue 2: Connector loses allowlist when it reconnects to controller

**Root cause:** When the connector reconnects to the controller (e.g., after cert
renewal or network blip), it gets a fresh allowlist via `sendAllowlist()`. This
works IF the controller's `AgentRegistry` is populated. With Issue 1 fixed, this
should work.

**Already done:** `LoadAgentRegistryFromDB()` added to controller startup in
`main.go`. Combined with Issue 1 fix, this resolves the problem.

**Files:** `services/controller/main.go`, `services/controller/state/persistence.go`

---

## Issue 3: Policy snapshot lookup misses junction table (FIXED)

**Root cause:** `lookupConnectorNetwork()` in `policy_snapshot.go` only checked
`connectors.remote_network_id` but the admin UI assigns connectors via the
`remote_network_connectors` junction table.

**Status:** Already fixed — added fallback to junction table.

**Files:** `services/controller/api/policy_snapshot.go`

---

## Issue 4: Firewall policy lost due to broadcast timing (FIXED)

**Root cause:** Connector receives `policy_snapshot` from controller before any
agents connect. `firewall_tx.send()` silently fails (no subscribers).
Agents connecting later never receive the initial policy.

**Status:** Already fixed — added `LatestFirewallPolicy` store in connector.
New agent connections receive cached policy immediately.

**Files:** `services/connector/src/main.rs`, `services/connector/src/control_plane.rs`,
`services/connector/src/server.rs`

---

## Issue 5: Missing `firewall_status` in remote networks resource query (FIXED)

**Root cause:** SQL query in `ui_remote_networks.go` was missing `firewall_status`
column, causing `scanUIResource()` to fail silently (expects 11 columns, got 10).

**Status:** Already fixed.

**Files:** `services/controller/admin/ui_remote_networks.go`

---

## Issue 6: UI shows "Ignore" instead of "Unprotect" (FIXED)

**Status:** Already fixed — button text changed to "Unprotect".

**Files:** `apps/frontend/components/dashboard/resources/resources-list.tsx`

---

## Remaining Work (in order)

### Must Do
- [ ] **Fix Issue 1**: Persist agent to `tunnelers` table in `NotifyAgentAllowed()`
- [ ] **Rebuild controller**: `go build ./...` and restart
- [ ] **Test full flow**: controller restart → connector reconnects → agent connects → protect resource → verify nftables rules

### Should Verify
- [ ] After protecting SSH (port 22), run `sudo nft list table inet ztna` — expect 3 rules
- [ ] From another machine, try `ssh 192.168.1.81` — should be blocked
- [ ] From the agent machine itself, try `ssh localhost` — should work (lo accepted)
- [ ] Unprotect the resource → rules should be removed
- [ ] Restart agent service → rules should restore from `firewall_state.json`

### Nice to Have
- [ ] Add logging in connector for policy_snapshot processing (how many resources, how many protected)
- [ ] systemd agent.service needs `CAP_NET_ADMIN` capability (already updated in `systemd/agent.service`)
