package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"controller/state"
)

// ---- helpers ----------------------------------------------------------------

// doWithWorkspace sends a request through the full mux with a valid workspace JWT,
// so the withWorkspaceContext middleware populates the workspace context.
func doWithWorkspace(srv *Server, method, path string, body interface{}, wsEmail, wsUserID, wsID, wsSlug, wsRole string) *httptest.ResponseRecorder {
	var buf strings.Builder
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(buf.String()))
	req.Header.Set("Content-Type", "application/json")

	// Sign an admin JWT so withWorkspaceContext extracts claims.
	tok, err := srv.signAdminJWT(wsEmail, wsUserID, wsID, wsSlug, wsRole, "test-session-id")
	if err != nil {
		panic(fmt.Sprintf("signAdminJWT: %v", err))
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	mux.ServeHTTP(rr, req)
	return rr
}

func newIsolationTestServer(t *testing.T) *Server {
	t.Helper()
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)
	srv.JWTSecret = []byte("test-isolation-secret")
	return srv
}

func insertRemoteNetwork(t *testing.T, srv *Server, id, name, wsID string) {
	t.Helper()
	db := srv.ACLs.DB()
	now := isoStringNow()
	_, err := db.Exec(state.Rebind(`INSERT INTO remote_networks (id, name, location, tags_json, created_at, updated_at, workspace_id) VALUES (?, ?, 'OTHER', '{}', ?, ?, ?)`),
		id, name, now, now, wsID)
	if err != nil {
		t.Fatalf("insert remote network %s: %v", id, err)
	}
}

func insertResource(t *testing.T, srv *Server, id, name, networkID, wsID string) {
	t.Helper()
	db := srv.ACLs.DB()
	_, err := db.Exec(state.Rebind(`INSERT INTO resources (id, name, type, address, protocol, description, remote_network_id, workspace_id) VALUES (?, ?, 'dns', 'test.internal', 'TCP', 'test resource', ?, ?)`),
		id, name, networkID, wsID)
	if err != nil {
		t.Fatalf("insert resource %s: %v", id, err)
	}
}

func insertConnectorInWorkspace(t *testing.T, srv *Server, id, name, networkID, wsID string) {
	t.Helper()
	db := srv.ACLs.DB()
	nowUnix := time.Now().UTC().Unix()
	nowISO := isoStringNow()
	_, err := db.Exec(state.Rebind(`INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, last_seen_at, installed, last_policy_version, workspace_id) VALUES (?, ?, 'offline', '1.0', 'host', ?, ?, ?, 0, 0, ?)`),
		id, name, networkID, nowUnix, nowISO, wsID)
	if err != nil {
		t.Fatalf("insert connector %s: %v", id, err)
	}
}

func insertGroup(t *testing.T, srv *Server, id, name, wsID string) {
	t.Helper()
	db := srv.ACLs.DB()
	now := isoStringNow()
	_, err := db.Exec(state.Rebind(`INSERT INTO user_groups (id, name, description, created_at, updated_at, workspace_id) VALUES (?, ?, 'test group', ?, ?, ?)`),
		id, name, now, now, wsID)
	if err != nil {
		t.Fatalf("insert group %s: %v", id, err)
	}
}

// ---- Cross-workspace resource isolation ------------------------------------

func TestCrossWorkspaceResourceGet_ReturnsNull(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-iso-a")
	insertWorkspace(t, db, "ws-iso-b")

	insertRemoteNetwork(t, srv, "net-iso-a", "Net A", "ws-iso-a")
	insertRemoteNetwork(t, srv, "net-iso-b", "Net B", "ws-iso-b")

	insertResource(t, srv, "res-iso-a1", "Resource A1", "net-iso-a", "ws-iso-a")
	insertResource(t, srv, "res-iso-b1", "Resource B1", "net-iso-b", "ws-iso-b")

	// Workspace A user requests workspace B's resource — should get null.
	rr := doWithWorkspace(srv, http.MethodGet, "/api/resources/res-iso-b1",
		nil, "admin@ws-a.test", "user-a", "ws-iso-a", "ws-iso-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"resource":null`) {
		t.Errorf("expected null resource for cross-workspace GET, got: %s", body)
	}

	// Workspace A user requests own resource — should succeed.
	rr = doWithWorkspace(srv, http.MethodGet, "/api/resources/res-iso-a1",
		nil, "admin@ws-a.test", "user-a", "ws-iso-a", "ws-iso-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body = rr.Body.String()
	if strings.Contains(body, `"resource":null`) {
		t.Errorf("expected own resource to be returned, got null: %s", body)
	}
	if !strings.Contains(body, `"Resource A1"`) {
		t.Errorf("expected Resource A1 in response, got: %s", body)
	}
}

func TestCrossWorkspaceResourceList_OnlyOwnResources(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-list-a")
	insertWorkspace(t, db, "ws-list-b")

	insertRemoteNetwork(t, srv, "net-list-a", "Net A", "ws-list-a")
	insertRemoteNetwork(t, srv, "net-list-b", "Net B", "ws-list-b")

	insertResource(t, srv, "res-list-a1", "ListResA", "net-list-a", "ws-list-a")
	insertResource(t, srv, "res-list-b1", "ListResB", "net-list-b", "ws-list-b")

	// List resources from workspace A — should only see A's resource.
	rr := doWithWorkspace(srv, http.MethodGet, "/api/resources",
		nil, "admin@ws-a.test", "user-a", "ws-list-a", "ws-list-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"ListResA"`) {
		t.Errorf("expected ListResA in workspace A listing, got: %s", body)
	}
	if strings.Contains(body, `"ListResB"`) {
		t.Errorf("workspace A listing should NOT contain ListResB, got: %s", body)
	}
}

// ---- Cross-workspace DELETE blocked ----------------------------------------

func TestCrossWorkspaceResourceDelete_Blocked(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-del-a")
	insertWorkspace(t, db, "ws-del-b")

	insertRemoteNetwork(t, srv, "net-del-b", "Net B", "ws-del-b")
	insertResource(t, srv, "res-del-b1", "Resource B1", "net-del-b", "ws-del-b")

	// Try to DELETE workspace B's resource from workspace A's context.
	rr := doWithWorkspace(srv, http.MethodDelete, "/api/resources/res-del-b1",
		nil, "admin@ws-a.test", "user-a", "ws-del-a", "ws-del-a", "owner")
	// The handler returns 200 even if nothing was deleted (idempotent), but
	// the resource must still exist in workspace B.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	_ = db.QueryRow(state.Rebind(`SELECT COUNT(*) FROM resources WHERE id = ?`), "res-del-b1").Scan(&count)
	if count != 1 {
		t.Errorf("expected resource to still exist after cross-workspace delete, got count=%d", count)
	}
}

// ---- Cross-workspace connector isolation -----------------------------------

func TestCrossWorkspaceConnectorGet_ReturnsNull(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-conn-a")
	insertWorkspace(t, db, "ws-conn-b")

	insertRemoteNetwork(t, srv, "net-conn-a", "Net A", "ws-conn-a")
	insertRemoteNetwork(t, srv, "net-conn-b", "Net B", "ws-conn-b")

	insertConnectorInWorkspace(t, srv, "con-iso-a1", "Connector A1", "net-conn-a", "ws-conn-a")
	insertConnectorInWorkspace(t, srv, "con-iso-b1", "Connector B1", "net-conn-b", "ws-conn-b")

	// Workspace A user requests workspace B's connector — should get null.
	rr := doWithWorkspace(srv, http.MethodGet, "/api/connectors/con-iso-b1",
		nil, "admin@ws-a.test", "user-a", "ws-conn-a", "ws-conn-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"connector":null`) {
		t.Errorf("expected null connector for cross-workspace GET, got: %s", body)
	}

	// Workspace A user requests own connector — should succeed.
	rr = doWithWorkspace(srv, http.MethodGet, "/api/connectors/con-iso-a1",
		nil, "admin@ws-a.test", "user-a", "ws-conn-a", "ws-conn-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body = rr.Body.String()
	if strings.Contains(body, `"connector":null`) {
		t.Errorf("expected own connector to be returned, got null: %s", body)
	}
}

func TestCrossWorkspaceConnectorList_OnlyOwnConnectors(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-conl-a")
	insertWorkspace(t, db, "ws-conl-b")

	insertRemoteNetwork(t, srv, "net-conl-a", "Net A", "ws-conl-a")
	insertRemoteNetwork(t, srv, "net-conl-b", "Net B", "ws-conl-b")

	insertConnectorInWorkspace(t, srv, "con-list-a1", "ConnA", "net-conl-a", "ws-conl-a")
	insertConnectorInWorkspace(t, srv, "con-list-b1", "ConnB", "net-conl-b", "ws-conl-b")

	rr := doWithWorkspace(srv, http.MethodGet, "/api/connectors",
		nil, "admin@ws-a.test", "user-a", "ws-conl-a", "ws-conl-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"ConnA"`) {
		t.Errorf("expected ConnA in workspace A listing, got: %s", body)
	}
	if strings.Contains(body, `"ConnB"`) {
		t.Errorf("workspace A listing should NOT contain ConnB, got: %s", body)
	}
}

func TestCrossWorkspaceConnectorDelete_Blocked(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-cdel-a")
	insertWorkspace(t, db, "ws-cdel-b")

	insertRemoteNetwork(t, srv, "net-cdel-b", "Net B", "ws-cdel-b")
	insertConnectorInWorkspace(t, srv, "con-del-b1", "Connector B1", "net-cdel-b", "ws-cdel-b")

	// Try to DELETE workspace B's connector from workspace A's context.
	rr := doWithWorkspace(srv, http.MethodDelete, "/api/connectors/con-del-b1",
		nil, "admin@ws-a.test", "user-a", "ws-cdel-a", "ws-cdel-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Connector must still exist (the UPDATE SET revoked should be scoped).
	var count int
	_ = db.QueryRow(state.Rebind(`SELECT COUNT(*) FROM connectors WHERE id = ? AND (revoked IS NULL OR revoked = 0)`), "con-del-b1").Scan(&count)
	if count != 1 {
		t.Errorf("expected connector to still be non-revoked after cross-workspace delete, got count=%d", count)
	}
}

// ---- Cross-workspace group isolation ---------------------------------------

func TestCrossWorkspaceGroupGet_ReturnsNull(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-grp-a")
	insertWorkspace(t, db, "ws-grp-b")

	insertGroup(t, srv, "grp-iso-a1", "Group A1", "ws-grp-a")
	insertGroup(t, srv, "grp-iso-b1", "Group B1", "ws-grp-b")

	// Workspace A user requests workspace B's group — should get null.
	rr := doWithWorkspace(srv, http.MethodGet, "/api/groups/grp-iso-b1",
		nil, "admin@ws-a.test", "user-a", "ws-grp-a", "ws-grp-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"group":null`) {
		t.Errorf("expected null group for cross-workspace GET, got: %s", body)
	}

	// Workspace A user requests own group — should succeed.
	rr = doWithWorkspace(srv, http.MethodGet, "/api/groups/grp-iso-a1",
		nil, "admin@ws-a.test", "user-a", "ws-grp-a", "ws-grp-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body = rr.Body.String()
	if strings.Contains(body, `"group":null`) {
		t.Errorf("expected own group to be returned, got null: %s", body)
	}
}

func TestCrossWorkspaceGroupList_OnlyOwnGroups(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-grpl-a")
	insertWorkspace(t, db, "ws-grpl-b")

	insertGroup(t, srv, "grp-list-a1", "GrpListA", "ws-grpl-a")
	insertGroup(t, srv, "grp-list-b1", "GrpListB", "ws-grpl-b")

	rr := doWithWorkspace(srv, http.MethodGet, "/api/groups",
		nil, "admin@ws-a.test", "user-a", "ws-grpl-a", "ws-grpl-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"GrpListA"`) {
		t.Errorf("expected GrpListA in workspace A listing, got: %s", body)
	}
	if strings.Contains(body, `"GrpListB"`) {
		t.Errorf("workspace A listing should NOT contain GrpListB, got: %s", body)
	}
}

func TestCrossWorkspaceGroupDelete_Blocked(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-gdel-a")
	insertWorkspace(t, db, "ws-gdel-b")

	insertGroup(t, srv, "grp-del-b1", "Group B1", "ws-gdel-b")

	// Try to DELETE workspace B's group from workspace A's context.
	rr := doWithWorkspace(srv, http.MethodDelete, "/api/groups/grp-del-b1",
		nil, "admin@ws-a.test", "user-a", "ws-gdel-a", "ws-gdel-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	_ = db.QueryRow(state.Rebind(`SELECT COUNT(*) FROM user_groups WHERE id = ?`), "grp-del-b1").Scan(&count)
	if count != 1 {
		t.Errorf("expected group to still exist after cross-workspace delete, got count=%d", count)
	}
}

// ---- Cross-workspace remote network isolation ------------------------------

func TestCrossWorkspaceNetworkGet_ReturnsNull(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-net-a")
	insertWorkspace(t, db, "ws-net-b")

	insertRemoteNetwork(t, srv, "net-iso-a1", "Network A1", "ws-net-a")
	insertRemoteNetwork(t, srv, "net-iso-b1", "Network B1", "ws-net-b")

	// Workspace A user requests workspace B's network — should get null.
	rr := doWithWorkspace(srv, http.MethodGet, "/api/remote-networks/net-iso-b1",
		nil, "admin@ws-a.test", "user-a", "ws-net-a", "ws-net-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"network":null`) {
		t.Errorf("expected null network for cross-workspace GET, got: %s", body)
	}

	// Workspace A user requests own network — should succeed.
	rr = doWithWorkspace(srv, http.MethodGet, "/api/remote-networks/net-iso-a1",
		nil, "admin@ws-a.test", "user-a", "ws-net-a", "ws-net-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body = rr.Body.String()
	if strings.Contains(body, `"network":null`) {
		t.Errorf("expected own network to be returned, got null: %s", body)
	}
}

func TestCrossWorkspaceNetworkList_OnlyOwnNetworks(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-netl-a")
	insertWorkspace(t, db, "ws-netl-b")

	insertRemoteNetwork(t, srv, "net-list-a1", "NetListA", "ws-netl-a")
	insertRemoteNetwork(t, srv, "net-list-b1", "NetListB", "ws-netl-b")

	rr := doWithWorkspace(srv, http.MethodGet, "/api/remote-networks",
		nil, "admin@ws-a.test", "user-a", "ws-netl-a", "ws-netl-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"NetListA"`) {
		t.Errorf("expected NetListA in workspace A listing, got: %s", body)
	}
	if strings.Contains(body, `"NetListB"`) {
		t.Errorf("workspace A listing should NOT contain NetListB, got: %s", body)
	}
}

func TestCrossWorkspaceNetworkDelete_Blocked(t *testing.T) {
	srv := newIsolationTestServer(t)
	db := srv.ACLs.DB()

	insertWorkspace(t, db, "ws-ndel-a")
	insertWorkspace(t, db, "ws-ndel-b")

	insertRemoteNetwork(t, srv, "net-del-b1", "Network B1", "ws-ndel-b")

	// Try to DELETE workspace B's network from workspace A's context.
	rr := doWithWorkspace(srv, http.MethodDelete, "/api/remote-networks/net-del-b1",
		nil, "admin@ws-a.test", "user-a", "ws-ndel-a", "ws-ndel-a", "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	_ = db.QueryRow(state.Rebind(`SELECT COUNT(*) FROM remote_networks WHERE id = ?`), "net-del-b1").Scan(&count)
	if count != 1 {
		t.Errorf("expected network to still exist after cross-workspace delete, got count=%d", count)
	}
}
