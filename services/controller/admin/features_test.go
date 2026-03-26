package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"controller/state"
)

// ---- helpers ----------------------------------------------------------------

func newTestDB(t *testing.T) *sql.DB {
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

func newTestServer(t *testing.T, db *sql.DB) (*Server, *mockNotifier) {
	t.Helper()
	notify := &mockNotifier{}
	srv := &Server{
		Tokens:            state.NewTokenStoreWithDB(60, db),
		Reg:               state.NewRegistry(),
		Agents:            state.NewAgentStatusRegistry(),
		ACLs:              state.NewACLStoreWithDB(db),
		ACLNotify:         notify,
		Users:             state.NewUserStore(db),
		AdminAuthToken:    "test-token",
		InternalAuthToken: "internal-token",
	}
	return srv, notify
}

// mockNotifier counts NotifyPolicyChange calls.
type mockNotifier struct{ calls atomic.Int32 }

func (m *mockNotifier) NotifyACLInit()                                  {}
func (m *mockNotifier) NotifyResourceUpsert(_ state.Resource)           {}
func (m *mockNotifier) NotifyResourceRemoved(_ string)                  {}
func (m *mockNotifier) NotifyAuthorizationUpsert(_ state.Authorization) {}
func (m *mockNotifier) NotifyAuthorizationRemoved(_, _ string)          {}
func (m *mockNotifier) NotifyPolicyChange()                             { m.calls.Add(1) }
func (m *mockNotifier) notified() bool                                  { return m.calls.Load() > 0 }

type mockAllowlistRefresher struct {
	calls []string
}

func (m *mockAllowlistRefresher) RefreshConnectorAllowlist(connectorID string) error {
	m.calls = append(m.calls, connectorID)
	return nil
}

// do sends a request to the full mux (admin + UI routes).
func do(srv *Server, method, path string, body interface{}, auth bool) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux) // registers both admin and UI routes
	mux.ServeHTTP(rr, req)
	return rr
}

func insertConnector(t *testing.T, db *sql.DB, id, remoteNetworkID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, last_seen_at, installed, last_policy_version)
		 VALUES (?, 'test', 'offline', '1.0', 'host', ?, 0, '', 0, 0)`,
		id, remoteNetworkID,
	)
	if err != nil {
		t.Fatalf("insert connector: %v", err)
	}
}

func insertToken(t *testing.T, db *sql.DB, token, connectorID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO tokens (token, connector_id, expires_at, consumed) VALUES (?, ?, ?, 1)`,
		token, connectorID, time.Now().Add(time.Hour).Unix(),
	)
	if err != nil {
		t.Fatalf("insert token: %v", err)
	}
}

func insertPolicyVersion(t *testing.T, db *sql.DB, connectorID string, version int) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO connector_policy_versions (connector_id, version, compiled_at, policy_hash)
		 VALUES (?, ?, '', '')
		 ON CONFLICT(connector_id) DO UPDATE SET version=excluded.version`,
		connectorID, version,
	)
	if err != nil {
		t.Fatalf("insert policy version: %v", err)
	}
}

func insertWorkspace(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO workspaces (id, name, slug, trust_domain, ca_cert_pem, ca_key_pem, status, created_at, updated_at)
		 VALUES (?, 'Test Workspace', ?, ?, '', '', 'active', ?, ?)`,
		id, id, id+".internal", now, now,
	)
	if err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
}

// ---- Feature 1: Connector DELETE --------------------------------------------

func TestConnectorDELETE_removesFromDBAndRegistry(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)

	connID := "conn-del-1"
	insertConnector(t, db, connID, "")
	insertToken(t, db, "tok-abc", connID)
	srv.Reg.Register(connID, "10.0.0.1", "1.0")

	rr := do(srv, http.MethodDelete, "/api/connectors/"+connID, nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Connector must be gone from DB
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM connectors WHERE id = ?`, connID).Scan(&count)
	if count != 0 {
		t.Errorf("connector still in DB after DELETE, count=%d", count)
	}

	// Token must be gone
	_ = db.QueryRow(`SELECT COUNT(*) FROM tokens WHERE connector_id = ?`, connID).Scan(&count)
	if count != 0 {
		t.Errorf("token still in DB after DELETE, count=%d", count)
	}

	// Registry must not have it
	if _, ok := srv.Reg.Get(connID); ok {
		t.Error("connector still in in-memory registry after DELETE")
	}
}

func TestConnectorDELETE_unknownIDreturnsOK(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)
	rr := do(srv, http.MethodDelete, "/api/connectors/nonexistent", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (idempotent), got %d", rr.Code)
	}
}

// ---- Feature 2: Heartbeat policy version logging ----------------------------

func TestHeartbeatPatch_policyMismatch_writesLogAndNotifies(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	connID := "conn-hb-1"
	insertConnector(t, db, connID, "")
	// Controller at version 5, connector reports 3 → mismatch + update_available
	insertPolicyVersion(t, db, connID, 5)

	rr := do(srv, http.MethodPatch, "/api/connectors/"+connID+"/heartbeat",
		map[string]int{"last_policy_version": 3}, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["update_available"] != true {
		t.Errorf("expected update_available=true, got %v", resp["update_available"])
	}

	// Must have written a connector_log row
	var msg string
	if err := db.QueryRow(`SELECT message FROM connector_logs WHERE connector_id = ? LIMIT 1`, connID).Scan(&msg); err != nil {
		t.Fatalf("no connector_log entry found: %v", err)
	}
	if msg == "" {
		t.Error("connector_log message is empty")
	}

	// ACLNotify must fire when update_available=true
	if !notify.notified() {
		t.Error("expected NotifyPolicyChange when update_available=true")
	}
}

func TestHeartbeatPatch_connectorAhead_writesLogNoNotify(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	connID := "conn-hb-3"
	insertConnector(t, db, connID, "")
	// Controller at version 3, connector reports 5 → mismatch but NOT update_available
	insertPolicyVersion(t, db, connID, 3)

	rr := do(srv, http.MethodPatch, "/api/connectors/"+connID+"/heartbeat",
		map[string]int{"last_policy_version": 5}, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Log must still be written (version mismatch)
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM connector_logs WHERE connector_id = ?`, connID).Scan(&count)
	if count == 0 {
		t.Error("expected connector_log entry even when connector is ahead")
	}

	// But ACLNotify must NOT fire (update_available=false)
	if notify.notified() {
		t.Error("NotifyPolicyChange must NOT fire when connector version > controller version")
	}
}

func TestHeartbeatPatch_noMismatch_noLogNoNotify(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	connID := "conn-hb-2"
	insertConnector(t, db, connID, "")
	insertPolicyVersion(t, db, connID, 7)

	rr := do(srv, http.MethodPatch, "/api/connectors/"+connID+"/heartbeat",
		map[string]int{"last_policy_version": 7}, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM connector_logs WHERE connector_id = ?`, connID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no log when versions match, got %d entries", count)
	}
	if notify.notified() {
		t.Error("NotifyPolicyChange must NOT fire when versions match")
	}
}

// ---- Feature 3: ACLNotify on user/group CRUD --------------------------------
// Tests use /api/users (UI route, no auth) and /api/admin/users (admin route, with auth).

func TestUICreateUser_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	rr := do(srv, http.MethodPost, "/api/users",
		map[string]string{"name": "Alice", "email": "alice@example.com", "status": "active"}, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after UI CreateUser")
	}
}

func TestAdminCreateUser_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	rr := do(srv, http.MethodPost, "/api/admin/users",
		map[string]string{"name": "Bob", "email": "bob@example.com"}, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after admin CreateUser")
	}
}

func TestAdminUpdateUser_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	u := &state.User{Name: "Charlie", Email: "charlie@example.com", Status: "Active", Role: "Member",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	rr := do(srv, http.MethodPut, "/api/admin/users/"+u.ID,
		map[string]string{"name": "Charlie Updated"}, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after admin UpdateUser")
	}
}

func TestAdminDeleteUser_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	u := &state.User{Name: "Dave", Email: "dave@example.com", Status: "Active", Role: "Member",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	rr := do(srv, http.MethodDelete, "/api/admin/users/"+u.ID, nil, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after admin DeleteUser")
	}
}

func TestAdminDeleteUser_removesWorkspaceMemberships(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)

	u := &state.User{Name: "Eve", Email: "eve@example.com", Status: "Active", Role: "Member",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	wsID := "ws-delete-user"
	insertWorkspace(t, db, wsID)
	if _, err := db.Exec(
		state.Rebind(`INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`),
		wsID, u.ID, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insert workspace membership: %v", err)
	}

	rr := do(srv, http.MethodDelete, "/api/admin/users/"+u.ID, nil, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspace_members WHERE user_id = ?`, u.ID).Scan(&count); err != nil {
		t.Fatalf("count workspace memberships: %v", err)
	}
	if count != 0 {
		t.Fatalf("workspace memberships still exist for user %s", u.ID)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, u.ID).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("user still exists after delete: %s", u.ID)
	}
}

func TestUICreateGroup_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	rr := do(srv, http.MethodPost, "/api/groups",
		map[string]string{"name": "Engineers", "description": "Eng team"}, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after UI CreateGroup")
	}
}

func TestAdminCreateGroup_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	rr := do(srv, http.MethodPost, "/api/admin/user-groups",
		map[string]string{"name": "Ops"}, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after admin CreateGroup")
	}
}

func TestAdminDeleteGroup_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	g := &state.UserGroup{Name: "ToDelete", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateGroup(g); err != nil {
		t.Fatalf("create group: %v", err)
	}

	rr := do(srv, http.MethodDelete, "/api/admin/user-groups/"+g.ID, nil, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after admin DeleteGroup")
	}
}

func TestUIAddGroupMembers_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	u := &state.User{Name: "Eve", Email: "eve@example.com", Status: "Active", Role: "Member",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	g := &state.UserGroup{Name: "Grp1", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateGroup(g); err != nil {
		t.Fatalf("create group: %v", err)
	}

	// UI endpoint: POST /api/groups/:id/members with {"memberIds": [...]}
	rr := do(srv, http.MethodPost, "/api/groups/"+g.ID+"/members",
		map[string][]string{"memberIds": {u.ID}}, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after UI AddGroupMembers")
	}
}

func TestUIRemoveGroupMember_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	u := &state.User{Name: "Frank", Email: "frank@example.com", Status: "Active", Role: "Member",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	g := &state.UserGroup{Name: "Grp2", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateGroup(g); err != nil {
		t.Fatalf("create group: %v", err)
	}

	// UI endpoint: DELETE /api/groups/:groupID/members/:userID
	rr := do(srv, http.MethodDelete, "/api/groups/"+g.ID+"/members/"+u.ID, nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after UI RemoveGroupMember")
	}
}

func TestAdminAddGroupMember_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	u := &state.User{Name: "Grace", Email: "grace@example.com", Status: "Active", Role: "Member",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	g := &state.UserGroup{Name: "Grp3", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateGroup(g); err != nil {
		t.Fatalf("create group: %v", err)
	}

	rr := do(srv, http.MethodPost, "/api/admin/user-groups/"+g.ID+"/members",
		map[string]string{"user_id": u.ID}, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after admin AddUserToGroup")
	}
}

func TestAdminRemoveGroupMember_notifiesACL(t *testing.T) {
	db := newTestDB(t)
	srv, notify := newTestServer(t, db)

	u := &state.User{Name: "Henry", Email: "henry@example.com", Status: "Active", Role: "Member",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	g := &state.UserGroup{Name: "Grp4", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := srv.Users.CreateGroup(g); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := srv.Users.AddUserToGroup(u.ID, g.ID); err != nil {
		t.Fatalf("add to group: %v", err)
	}

	rr := do(srv, http.MethodDelete, "/api/admin/user-groups/"+g.ID+"/members",
		map[string]string{"user_id": u.ID}, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !notify.notified() {
		t.Error("NotifyPolicyChange not called after admin RemoveUserFromGroup")
	}
}

// ---- Feature 4: lookupConnectorNetworkID fallback ---------------------------

func TestPolicyCompile_directRemoteNetworkID(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)

	_, _ = db.Exec(`INSERT INTO remote_networks (id, name, location, created_at, updated_at) VALUES ('net-1', 'Net1', 'US', '', '')`)
	insertConnector(t, db, "conn-p1", "net-1")

	rr := do(srv, http.MethodGet, "/api/policy/compile/conn-p1", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["resources"] == nil {
		t.Error("expected resources key in response")
	}
}

func TestPolicyCompile_fallbackJunctionTable(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)

	_, _ = db.Exec(`INSERT INTO remote_networks (id, name, location, created_at, updated_at) VALUES ('net-2', 'Net2', 'EU', '', '')`)
	insertConnector(t, db, "conn-p2", "") // empty remote_network_id
	_, _ = db.Exec(`INSERT INTO remote_network_connectors (network_id, connector_id) VALUES ('net-2', 'conn-p2')`)

	rr := do(srv, http.MethodGet, "/api/policy/compile/conn-p2", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 via junction-table fallback, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPolicyCompile_noNetworkAssigned_404(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)

	insertConnector(t, db, "conn-p3", "")

	rr := do(srv, http.MethodGet, "/api/policy/compile/conn-p3", nil, false)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unassigned connector, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPolicyACL_fallbackJunctionTable(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)

	_, _ = db.Exec(`INSERT INTO remote_networks (id, name, location, created_at, updated_at) VALUES ('net-3', 'Net3', 'AS', '', '')`)
	insertConnector(t, db, "conn-p4", "")
	_, _ = db.Exec(`INSERT INTO remote_network_connectors (network_id, connector_id) VALUES ('net-3', 'conn-p4')`)

	rr := do(srv, http.MethodGet, "/api/policy/acl/conn-p4", nil, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 via junction-table fallback for ACL, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---- Feature 5: Enhanced state persistence ----------------------------------

func TestSaveConnectorToDB_setsInstalledAndStatus(t *testing.T) {
	db := newTestDB(t)
	insertConnector(t, db, "conn-s1", "")

	rec := state.ConnectorRecord{
		ID:        "conn-s1",
		PrivateIP: "192.168.1.10",
		Version:   "2.0",
		LastSeen:  time.Now().UTC(),
	}
	if err := state.SaveConnectorToDB(db, rec); err != nil {
		t.Fatalf("SaveConnectorToDB: %v", err)
	}

	var installed int
	var status, lastSeenAt string
	if err := db.QueryRow(`SELECT installed, status, last_seen_at FROM connectors WHERE id = ?`, "conn-s1").
		Scan(&installed, &status, &lastSeenAt); err != nil {
		t.Fatal(err)
	}
	if installed != 1 {
		t.Errorf("expected installed=1, got %d", installed)
	}
	if status != "online" {
		t.Errorf("expected status='online', got %q", status)
	}
	if lastSeenAt == "" {
		t.Error("expected last_seen_at to be set")
	}
}

func TestDeleteConnectorFromDB_cleansJunctionTable(t *testing.T) {
	db := newTestDB(t)
	insertConnector(t, db, "conn-d1", "")
	_, _ = db.Exec(`INSERT INTO remote_networks (id, name, location, created_at, updated_at) VALUES ('net-j', 'NetJ', 'US', '', '')`)
	_, _ = db.Exec(`INSERT INTO remote_network_connectors (network_id, connector_id) VALUES ('net-j', 'conn-d1')`)

	if err := state.DeleteConnectorFromDB(db, "conn-d1"); err != nil {
		t.Fatalf("DeleteConnectorFromDB: %v", err)
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM connectors WHERE id = ?`, "conn-d1").Scan(&count)
	if count != 0 {
		t.Error("connector row not deleted from connectors table")
	}

	_ = db.QueryRow(`SELECT COUNT(*) FROM remote_network_connectors WHERE connector_id = ?`, "conn-d1").Scan(&count)
	if count != 0 {
		t.Error("junction table row not cleaned up after DeleteConnectorFromDB")
	}
}

func TestRemoteNetworkConnectorAssign_refreshesAllowlist(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)
	refresher := &mockAllowlistRefresher{}
	srv.Allowlists = refresher

	_, _ = db.Exec(`INSERT INTO remote_networks (id, name, location, created_at, updated_at) VALUES ('net-r1', 'NetR1', 'US', '', '')`)
	insertConnector(t, db, "conn-r1", "")

	rr := do(srv, http.MethodPost, "/api/admin/remote-networks/net-r1/connectors", map[string]string{
		"connector_id": "conn-r1",
	}, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(refresher.calls) != 1 || refresher.calls[0] != "conn-r1" {
		t.Fatalf("expected allowlist refresh for conn-r1, got %#v", refresher.calls)
	}
}

func TestRemoteNetworkConnectorRemove_refreshesAllowlist(t *testing.T) {
	db := newTestDB(t)
	srv, _ := newTestServer(t, db)
	refresher := &mockAllowlistRefresher{}
	srv.Allowlists = refresher

	_, _ = db.Exec(`INSERT INTO remote_networks (id, name, location, created_at, updated_at) VALUES ('net-r2', 'NetR2', 'US', '', '')`)
	insertConnector(t, db, "conn-r2", "")
	_, _ = db.Exec(`INSERT INTO remote_network_connectors (network_id, connector_id) VALUES ('net-r2', 'conn-r2')`)

	rr := do(srv, http.MethodDelete, "/api/admin/remote-networks/net-r2/connectors", map[string]string{
		"connector_id": "conn-r2",
	}, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(refresher.calls) != 1 || refresher.calls[0] != "conn-r2" {
		t.Fatalf("expected allowlist refresh for conn-r2, got %#v", refresher.calls)
	}
}
