package admin

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"controller/state"
)

func TestLoadAuthorizedResources_SelectsFreshestConnectorWithLiveAgent(t *testing.T) {
	db := newTestDB(t)
	ids := seedAuthorizedResourceFixture(t, db)

	nowUnix := time.Now().UTC().Unix()
	nowISO := isoStringNow()

	insertConnectorForDeviceTest(t, db, ids.connectorA, ids.networkID, "198.51.100.10:9444", nowUnix, nowISO)
	insertConnectorForDeviceTest(t, db, ids.connectorB, ids.networkID, "198.51.100.11:9444", nowUnix+5, nowISO)
	insertAgentForDeviceTest(t, db, ids.agentID, ids.connectorB, ids.networkID, nowUnix)
	insertResourceAgentForDeviceTest(t, db, ids.resourceID, ids.agentID, ids.workspaceID)

	resources, err := loadAuthorizedResources(db, ids.workspaceID, ids.userID)
	if err != nil {
		t.Fatalf("loadAuthorizedResources: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if got := resources[0].ConnectorTunnelAddr; got != "198.51.100.11:9444" {
		t.Fatalf("expected freshest eligible connector addr, got %q", got)
	}
	if got := resources[0].AvailabilityStatus; got != resourceAvailabilityOnline {
		t.Fatalf("expected availability %q, got %q", resourceAvailabilityOnline, got)
	}
}

func TestLoadAuthorizedResources_ReportsOfflineWhenConnectorHasNoLiveAgent(t *testing.T) {
	db := newTestDB(t)
	ids := seedAuthorizedResourceFixture(t, db)

	nowUnix := time.Now().UTC().Unix()
	nowISO := isoStringNow()

	insertConnectorForDeviceTest(t, db, ids.connectorA, ids.networkID, "198.51.100.20:9444", nowUnix, nowISO)
	insertAgentForDeviceTest(t, db, ids.agentID, ids.connectorA, ids.networkID, nowUnix-(int64(connectorStaleThreshold/time.Second)+5))
	insertResourceAgentForDeviceTest(t, db, ids.resourceID, ids.agentID, ids.workspaceID)

	resources, err := loadAuthorizedResources(db, ids.workspaceID, ids.userID)
	if err != nil {
		t.Fatalf("loadAuthorizedResources: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if got := resources[0].ConnectorTunnelAddr; got != "" {
		t.Fatalf("expected empty connector addr, got %q", got)
	}
	if got := resources[0].AvailabilityStatus; got != resourceAvailabilityOffline {
		t.Fatalf("expected availability %q, got %q", resourceAvailabilityOffline, got)
	}
}

func TestLoadAuthorizedResources_UsesOwnedAgentConnectorWhenLive(t *testing.T) {
	db := newTestDB(t)
	ids := seedAuthorizedResourceFixture(t, db)

	if _, err := db.Exec(state.Rebind(`UPDATE resources SET remote_network_id = '', connector_id = ? WHERE id = ?`), ids.connectorA, ids.resourceID); err != nil {
		t.Fatalf("clear resource network: %v", err)
	}

	nowUnix := time.Now().UTC().Unix()
	nowISO := isoStringNow()

	insertConnectorForDeviceTest(t, db, ids.connectorA, "", "198.51.100.30:9444", nowUnix, nowISO)
	insertAgentForDeviceTest(t, db, ids.agentID, ids.connectorA, ids.networkID, nowUnix)
	insertResourceAgentForDeviceTest(t, db, ids.resourceID, ids.agentID, ids.workspaceID)

	resources, err := loadAuthorizedResources(db, ids.workspaceID, ids.userID)
	if err != nil {
		t.Fatalf("loadAuthorizedResources: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if got := resources[0].ConnectorTunnelAddr; got != "198.51.100.30:9444" {
		t.Fatalf("expected owned agent connector addr, got %q", got)
	}
	if got := resources[0].AvailabilityStatus; got != resourceAvailabilityOnline {
		t.Fatalf("expected availability %q, got %q", resourceAvailabilityOnline, got)
	}
}

type deviceFixtureIDs struct {
	workspaceID string
	userID      string
	groupID     string
	networkID   string
	resourceID  string
	ruleID      string
	connectorA  string
	connectorB  string
	agentID     string
}

func seedAuthorizedResourceFixture(t *testing.T, db *sql.DB) deviceFixtureIDs {
	t.Helper()

	suffix := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	ids := deviceFixtureIDs{
		workspaceID: "ws-device-" + suffix,
		userID:      "user-device-" + suffix,
		groupID:     "group-device-" + suffix,
		networkID:   "net-device-" + suffix,
		resourceID:  "res-device-" + suffix,
		ruleID:      "rule-device-" + suffix,
		connectorA:  "conn-device-a-" + suffix,
		connectorB:  "conn-device-b-" + suffix,
		agentID:     "agent-device-" + suffix,
	}

	insertWorkspace(t, db, ids.workspaceID)

	now := isoStringNow()
	if _, err := db.Exec(state.Rebind(`INSERT INTO users (id, name, email, status, role, created_at, updated_at, workspace_id)
		VALUES (?, 'Device User', ?, 'Active', 'Member', ?, ?, ?)`),
		ids.userID, ids.userID+"@example.com", now, now, ids.workspaceID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.Exec(state.Rebind(`INSERT INTO user_groups (id, name, description, created_at, updated_at, workspace_id)
		VALUES (?, 'Device Group', '', ?, ?, ?)`),
		ids.groupID, now, now, ids.workspaceID); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	if _, err := db.Exec(state.Rebind(`INSERT INTO user_group_members (user_id, group_id, joined_at) VALUES (?, ?, ?)`),
		ids.userID, ids.groupID, time.Now().UTC().Unix()); err != nil {
		t.Fatalf("insert group member: %v", err)
	}
	if _, err := db.Exec(state.Rebind(`INSERT INTO remote_networks (id, name, location, tags_json, created_at, updated_at, workspace_id)
		VALUES (?, 'Device Network', 'OTHER', '{}', ?, ?, ?)`),
		ids.networkID, now, now, ids.workspaceID); err != nil {
		t.Fatalf("insert network: %v", err)
	}
	if _, err := db.Exec(state.Rebind(`INSERT INTO resources (id, name, type, address, protocol, description, connector_id, remote_network_id, workspace_id)
		VALUES (?, 'Device Resource', 'dns', 'db.internal', 'TCP', '', '', ?, ?)`),
		ids.resourceID, ids.networkID, ids.workspaceID); err != nil {
		t.Fatalf("insert resource: %v", err)
	}
	if _, err := db.Exec(state.Rebind(`INSERT INTO access_rules (id, name, resource_id, enabled, created_at, updated_at, workspace_id)
		VALUES (?, 'Device Rule', ?, 1, ?, ?, ?)`),
		ids.ruleID, ids.resourceID, now, now, ids.workspaceID); err != nil {
		t.Fatalf("insert access rule: %v", err)
	}
	if _, err := db.Exec(state.Rebind(`INSERT INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)`),
		ids.ruleID, ids.groupID); err != nil {
		t.Fatalf("insert access_rule_group: %v", err)
	}

	return ids
}

func insertConnectorForDeviceTest(t *testing.T, db *sql.DB, connectorID, networkID, tunnelAddr string, lastSeen int64, lastSeenAt string) {
	t.Helper()
	if _, err := db.Exec(state.Rebind(`INSERT INTO connectors
		(id, name, status, version, hostname, private_ip, connector_tunnel_addr, remote_network_id, last_seen, last_seen_at, installed, last_policy_version, revoked)
		VALUES (?, 'Device Connector', 'online', '1.0.0', 'host', '10.0.0.1', ?, ?, ?, ?, 1, 0, 0)`),
		connectorID, tunnelAddr, networkID, lastSeen, lastSeenAt); err != nil {
		t.Fatalf("insert connector: %v", err)
	}
}

func insertAgentForDeviceTest(t *testing.T, db *sql.DB, agentID, connectorID, networkID string, lastSeen int64) {
	t.Helper()
	if _, err := db.Exec(state.Rebind(`INSERT INTO agents
		(id, name, spiffe_id, connector_id, status, version, hostname, remote_network_id, last_seen, revoked, last_seen_at, installed, ip)
		VALUES (?, 'Device Agent', ?, ?, 'online', '1.0.0', 'agent-host', ?, ?, 0, ?, 1, '10.0.0.2')`),
		agentID, "spiffe://test.internal/agent/"+agentID, connectorID, networkID, lastSeen, isoStringNow()); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
}

func insertResourceAgentForDeviceTest(t *testing.T, db *sql.DB, resourceID, agentID, workspaceID string) {
	t.Helper()
	if _, err := db.Exec(state.Rebind(`INSERT INTO resource_agents (resource_id, agent_id, workspace_id) VALUES (?, ?, ?)`),
		resourceID, agentID, workspaceID); err != nil {
		t.Fatalf("insert resource agent: %v", err)
	}
}
