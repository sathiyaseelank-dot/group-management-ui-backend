package api

import (
	"database/sql"
	"os"
	"reflect"
	"testing"

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
