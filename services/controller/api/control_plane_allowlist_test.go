package api

import (
	"database/sql"
	"os"
	"reflect"
	"testing"
	"time"

	"controller/state"
)

func newAllowlistTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set (PostgreSQL-only mode)")
	}
	db, err := state.Open(dsn, "")
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertRemoteNetwork(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO remote_networks (id, name, location, created_at, updated_at) VALUES (?, ?, 'TEST', '', '')`,
		id, id,
	); err != nil {
		t.Fatalf("insert remote network %s: %v", id, err)
	}
}

func insertAgentRow(t *testing.T, db *sql.DB, id, spiffeID, remoteNetworkID string, revoked int) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO agents (id, spiffe_id, remote_network_id, revoked) VALUES (?, ?, ?, ?)`,
		id, spiffeID, remoteNetworkID, revoked,
	); err != nil {
		t.Fatalf("insert agent %s: %v", id, err)
	}
}

func TestAllowlistForConnectorFiltersByRemoteNetwork(t *testing.T) {
	db := newAllowlistTestDB(t)
	server := &ControlPlaneServer{db: db}

	insertRemoteNetwork(t, db, "net-a")
	insertRemoteNetwork(t, db, "net-b")
	if _, err := db.Exec(
		`INSERT INTO connectors (id, name, remote_network_id) VALUES (?, 'Connector A', ?)`,
		"conn-a", "net-a",
	); err != nil {
		t.Fatalf("insert connector: %v", err)
	}

	insertAgentRow(t, db, "agent-a1", "spiffe://test.internal/agent/agent-a1", "net-a", 0)
	insertAgentRow(t, db, "agent-a2", "spiffe://test.internal/agent/agent-a2", "net-a", 0)
	insertAgentRow(t, db, "agent-b1", "spiffe://test.internal/agent/agent-b1", "net-b", 0)
	insertAgentRow(t, db, "agent-revoked", "spiffe://test.internal/agent/agent-revoked", "net-a", 1)
	insertAgentRow(t, db, "agent-empty", "", "net-a", 0)

	list, networkID, err := server.allowlistForConnector("conn-a")
	if err != nil {
		t.Fatalf("allowlistForConnector: %v", err)
	}
	if networkID != "net-a" {
		t.Fatalf("expected networkID net-a, got %q", networkID)
	}

	got := make([]string, 0, len(list))
	for _, item := range list {
		got = append(got, item.ID)
	}
	want := []string{"agent-a1", "agent-a2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected allowlist %v, got %v", want, got)
	}
}

func TestAllowlistForConnectorUsesJunctionFallback(t *testing.T) {
	db := newAllowlistTestDB(t)
	server := &ControlPlaneServer{db: db}

	insertRemoteNetwork(t, db, "net-j")
	if _, err := db.Exec(
		`INSERT INTO connectors (id, name, remote_network_id) VALUES (?, 'Connector J', '')`,
		"conn-j",
	); err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO remote_network_connectors (network_id, connector_id) VALUES (?, ?)`,
		"net-j", "conn-j",
	); err != nil {
		t.Fatalf("insert junction assignment: %v", err)
	}
	insertAgentRow(t, db, "agent-j1", "spiffe://test.internal/agent/agent-j1", "net-j", 0)

	list, networkID, err := server.allowlistForConnector("conn-j")
	if err != nil {
		t.Fatalf("allowlistForConnector: %v", err)
	}
	if networkID != "net-j" {
		t.Fatalf("expected networkID net-j, got %q", networkID)
	}
	if len(list) != 1 || list[0].ID != "agent-j1" {
		t.Fatalf("expected agent-j1 in allowlist, got %#v", list)
	}
}

func TestAllowlistForConnectorWithoutNetworkReturnsEmpty(t *testing.T) {
	db := newAllowlistTestDB(t)
	server := &ControlPlaneServer{db: db}

	if _, err := db.Exec(
		`INSERT INTO connectors (id, name, remote_network_id) VALUES (?, 'Connector Empty', '')`,
		"conn-empty",
	); err != nil {
		t.Fatalf("insert connector: %v", err)
	}

	list, networkID, err := server.allowlistForConnector("conn-empty")
	if err != nil {
		t.Fatalf("allowlistForConnector: %v", err)
	}
	if networkID != "" {
		t.Fatalf("expected empty networkID, got %q", networkID)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty allowlist, got %#v", list)
	}
}

func TestNotifyAgentAllowedPreservesAssignedRemoteNetwork(t *testing.T) {
	db := newAllowlistTestDB(t)
	server := &ControlPlaneServer{db: db}

	insertRemoteNetwork(t, db, "net-a")
	if _, err := db.Exec(
		`INSERT INTO connectors (id, name, remote_network_id) VALUES (?, 'Connector A', ?)`,
		"conn-a", "net-a",
	); err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	insertAgentRow(t, db, "agent-a1", "", "net-a", 0)

	server.NotifyAgentAllowed("agent-a1", "spiffe://test.internal/agent/agent-a1", "1.2.3", "host-a1", "10.0.0.10")

	var remoteNetworkID, spiffeID, version, hostname, ip string
	if err := db.QueryRow(
		`SELECT remote_network_id, spiffe_id, version, hostname, ip FROM agents WHERE id = ?`,
		"agent-a1",
	).Scan(&remoteNetworkID, &spiffeID, &version, &hostname, &ip); err != nil {
		t.Fatalf("query enrolled agent: %v", err)
	}
	if remoteNetworkID != "net-a" {
		t.Fatalf("expected remote_network_id net-a, got %q", remoteNetworkID)
	}
	if spiffeID != "spiffe://test.internal/agent/agent-a1" {
		t.Fatalf("expected spiffe_id to be updated, got %q", spiffeID)
	}
	if version != "1.2.3" || hostname != "host-a1" || ip != "10.0.0.10" {
		t.Fatalf("unexpected persisted agent fields: version=%q hostname=%q ip=%q", version, hostname, ip)
	}

	list, networkID, err := server.allowlistForConnector("conn-a")
	if err != nil {
		t.Fatalf("allowlistForConnector: %v", err)
	}
	if networkID != "net-a" {
		t.Fatalf("expected networkID net-a, got %q", networkID)
	}
	if len(list) != 1 || list[0].ID != "agent-a1" {
		t.Fatalf("expected agent-a1 in allowlist, got %#v", list)
	}
}

func TestNotifyAgentAllowedLeavesUnassignedAgentOutOfAllowlist(t *testing.T) {
	db := newAllowlistTestDB(t)
	server := &ControlPlaneServer{db: db}

	insertRemoteNetwork(t, db, "net-a")
	if _, err := db.Exec(
		`INSERT INTO connectors (id, name, remote_network_id) VALUES (?, 'Connector A', ?)`,
		"conn-a", "net-a",
	); err != nil {
		t.Fatalf("insert connector: %v", err)
	}

	server.NotifyAgentAllowed("agent-unassigned", "spiffe://test.internal/agent/agent-unassigned", "1.0.0", "host-u", "10.0.0.20")

	var remoteNetworkID string
	if err := db.QueryRow(
		`SELECT remote_network_id FROM agents WHERE id = ?`,
		"agent-unassigned",
	).Scan(&remoteNetworkID); err != nil {
		t.Fatalf("query enrolled agent: %v", err)
	}
	if remoteNetworkID != "" {
		t.Fatalf("expected empty remote_network_id, got %q", remoteNetworkID)
	}

	list, networkID, err := server.allowlistForConnector("conn-a")
	if err != nil {
		t.Fatalf("allowlistForConnector: %v", err)
	}
	if networkID != "net-a" {
		t.Fatalf("expected networkID net-a, got %q", networkID)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty allowlist, got %#v", list)
	}
}

func TestSaveAgentToDBPreservesAssignedRemoteNetwork(t *testing.T) {
	db := newAllowlistTestDB(t)

	insertRemoteNetwork(t, db, "net-a")
	insertAgentRow(t, db, "agent-state", "spiffe://test.internal/agent/agent-state", "net-a", 0)

	rec := state.AgentStatusRecord{
		ID:          "agent-state",
		SPIFFEID:    "spiffe://test.internal/agent/agent-state",
		ConnectorID: "conn-a",
		LastSeen:    time.Unix(1_700_000_000, 0).UTC(),
		IP:          "10.0.0.30",
	}
	if err := state.SaveAgentToDB(db, rec); err != nil {
		t.Fatalf("SaveAgentToDB: %v", err)
	}

	var remoteNetworkID, connectorID, ip string
	if err := db.QueryRow(
		`SELECT remote_network_id, connector_id, ip FROM agents WHERE id = ?`,
		"agent-state",
	).Scan(&remoteNetworkID, &connectorID, &ip); err != nil {
		t.Fatalf("query saved agent: %v", err)
	}
	if remoteNetworkID != "net-a" {
		t.Fatalf("expected remote_network_id net-a, got %q", remoteNetworkID)
	}
	if connectorID != "conn-a" || ip != "10.0.0.30" {
		t.Fatalf("unexpected runtime-updated fields: connector_id=%q ip=%q", connectorID, ip)
	}
}
