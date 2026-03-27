package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"controller/api"
	"controller/state"
)

// lookupConnectorNetworkID resolves the remote network ID for a connector.
// It first checks connectors.remote_network_id and falls back to the
// remote_network_connectors junction table when the column is empty.
// wsID scopes the lookup to a single workspace (pass "" to skip filtering).
func lookupConnectorNetworkID(db *sql.DB, connectorID, wsID string) (string, error) {
	wsClause, wsArgs := wsWhere(wsID, "")
	args := append([]interface{}{connectorID}, wsArgs...)
	var remoteNet sql.NullString
	if err := db.QueryRow(state.Rebind(`SELECT remote_network_id FROM connectors WHERE id = ?`+wsClause), args...).Scan(&remoteNet); err != nil {
		return "", err
	}
	if remoteNet.Valid && remoteNet.String != "" {
		return remoteNet.String, nil
	}
	var assigned sql.NullString
	if err := db.QueryRow(state.Rebind(`SELECT network_id FROM remote_network_connectors WHERE connector_id = ? LIMIT 1`), connectorID).Scan(&assigned); err != nil {
		return "", err
	}
	if assigned.Valid && assigned.String != "" {
		return assigned.String, nil
	}
	return "", sql.ErrNoRows
}

func (s *Server) uiDB(w http.ResponseWriter) (*sql.DB, bool) {
	if s == nil || s.ACLs == nil || s.ACLs.DB() == nil {
		http.Error(w, "db not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return s.ACLs.DB(), true
}

func scanUIResource(scanner interface{ Scan(dest ...any) error }) (uiResource, bool) {
	var res uiResource
	var protocol sql.NullString
	var portFrom sql.NullInt64
	var portTo sql.NullInt64
	var alias sql.NullString
	var remoteNet sql.NullString
	var firewallStatus sql.NullString
	if err := scanner.Scan(&res.ID, &res.Name, &res.Type, &res.Address, &protocol, &portFrom, &portTo, &alias, &res.Description, &remoteNet, &firewallStatus); err != nil {
		return uiResource{}, false
	}
	res.Protocol = "TCP"
	if protocol.Valid {
		res.Protocol = protocol.String
	}
	if portFrom.Valid {
		v := int(portFrom.Int64)
		res.PortFrom = &v
	}
	if portTo.Valid {
		v := int(portTo.Int64)
		res.PortTo = &v
	}
	if alias.Valid {
		res.Alias = &alias.String
	}
	if remoteNet.Valid {
		res.RemoteNetwork = &remoteNet.String
	}
	res.FirewallStatus = "unprotected"
	if firewallStatus.Valid && firewallStatus.String != "" {
		res.FirewallStatus = firewallStatus.String
	}
	return res, true
}

// connectorStaleThreshold is the duration after which a connector with no heartbeat
// is considered offline regardless of the stored status field.
const connectorStaleThreshold = 30 * time.Second

func scanUIConnector(scanner interface{ Scan(dest ...any) error }) (uiConnector, bool) {
	var c uiConnector
	var name sql.NullString
	var status sql.NullString
	var version sql.NullString
	var hostname sql.NullString
	var remoteNetworkID sql.NullString
	var lastSeen sql.NullString
	var lastSeenAt sql.NullString
	var installed sql.NullInt64
	var lastPolicyVersion sql.NullInt64
	var privateIP sql.NullString
	var revoked sql.NullInt64
	var lastSeenUnix sql.NullInt64
	if err := scanner.Scan(&c.ID, &name, &status, &version, &hostname, &remoteNetworkID, &lastSeen, &lastSeenAt, &installed, &lastPolicyVersion, &privateIP, &revoked, &lastSeenUnix); err != nil {
		return uiConnector{}, false
	}
	c.PrivateIP = privateIP.String
	c.Name = strings.TrimSpace(name.String)
	if c.Name == "" {
		c.Name = c.ID
	}
	c.Status = strings.TrimSpace(status.String)
	if c.Status == "" {
		c.Status = "offline"
	}
	if revoked.Valid && revoked.Int64 != 0 {
		c.Status = "revoked"
		c.Revoked = true
	} else if c.Status == "online" && lastSeenUnix.Valid && lastSeenUnix.Int64 > 0 {
		// Derive live status from the last heartbeat timestamp. If the connector
		// has not been seen within the stale threshold, treat it as offline even
		// if the stored status still says online.
		if time.Since(time.Unix(lastSeenUnix.Int64, 0)) > connectorStaleThreshold {
			c.Status = "offline"
		}
	}
	c.Version = strings.TrimSpace(version.String)
	if c.Version == "" {
		c.Version = "1.0.0"
	}
	c.Hostname = strings.TrimSpace(hostname.String)
	c.RemoteNetworkID = strings.TrimSpace(remoteNetworkID.String)
	if lastSeenAt.Valid {
		c.LastSeen = lastSeenAt.String
		c.LastSeenAt = &lastSeenAt.String
	} else if lastSeen.Valid {
		if ts, err := strconv.ParseInt(lastSeen.String, 10, 64); err == nil {
			iso := isoStringFromUnix(ts)
			c.LastSeen = iso
			c.LastSeenAt = &iso
		} else {
			c.LastSeen = lastSeen.String
			c.LastSeenAt = &lastSeen.String
		}
	}
	c.Installed = installed.Valid && installed.Int64 != 0
	if lastPolicyVersion.Valid {
		c.LastPolicyVersion = int(lastPolicyVersion.Int64)
	}
	return c, true
}

func scanUIAgent(scanner interface{ Scan(dest ...any) error }) (uiAgent, bool) {
	var t uiAgent
	var name sql.NullString
	var status sql.NullString
	var version sql.NullString
	var hostname sql.NullString
	var remoteNetworkID sql.NullString
	var connectorID sql.NullString
	var connectorName sql.NullString
	var revoked sql.NullInt64
	var installed sql.NullInt64
	var lastSeen sql.NullString
	var lastSeenAt sql.NullString
	var ip sql.NullString
	if err := scanner.Scan(&t.ID, &name, &status, &version, &hostname, &remoteNetworkID, &connectorID, &connectorName, &revoked, &installed, &lastSeen, &lastSeenAt, &ip); err != nil {
		return uiAgent{}, false
	}
	t.IP = ip.String
	t.ConnectorID = connectorID.String
	t.ConnectorName = strings.TrimSpace(connectorName.String)
	t.Name = strings.TrimSpace(name.String)
	if t.Name == "" {
		t.Name = t.ID
	}
	t.Installed = installed.Valid && installed.Int64 != 0
	t.Status = strings.TrimSpace(status.String)
	if t.Status == "" {
		t.Status = "offline"
	}
	if revoked.Valid && revoked.Int64 != 0 {
		t.Status = "revoked"
		t.Revoked = true
	} else if lastSeen.Valid && t.Installed {
		// Derive live status from heartbeat timestamp, same as connectors.
		// Only for enrolled agents (installed=1) — freshly created agents stay offline.
		if ts, err := strconv.ParseInt(lastSeen.String, 10, 64); err == nil && ts > 0 {
			if time.Since(time.Unix(ts, 0)) <= connectorStaleThreshold {
				t.Status = "online"
			} else if t.Status == "online" {
				t.Status = "offline"
			}
		}
	}
	t.Version = strings.TrimSpace(version.String)
	t.Hostname = strings.TrimSpace(hostname.String)
	t.RemoteNetworkID = strings.TrimSpace(remoteNetworkID.String)
	if lastSeenAt.Valid && lastSeenAt.String != "" {
		t.LastSeen = lastSeenAt.String
		t.LastSeenAt = &lastSeenAt.String
	} else if lastSeen.Valid {
		if ts, err := strconv.ParseInt(lastSeen.String, 10, 64); err == nil {
			iso := isoStringFromUnix(ts)
			t.LastSeen = iso
			t.LastSeenAt = &iso
		} else {
			t.LastSeen = lastSeen.String
			t.LastSeenAt = &lastSeen.String
		}
	}
	return t, true
}

func buildPorts(from, to *int) string {
	if from != nil && to != nil {
		return fmt.Sprintf("%d-%d", *from, *to)
	}
	if from != nil {
		return fmt.Sprintf("%d", *from)
	}
	return ""
}

func nullInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func dateStringNow() string {
	return time.Now().UTC().Format("2006-01-02")
}

func dateStringFromUnix(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02")
}

func isoStringNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func isoStringFromUnix(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02T15:04:05.000Z")
}

func policyResources(db *sql.DB, remoteNetworkID string) ([]policyResource, error) {
	return api.PolicyResourcesForUI(db, remoteNetworkID)
}

func policyHash(resources []policyResource) string {
	return api.PolicyHashForUI(resources)
}

func policyVersion(db *sql.DB, connectorID, policyHash, compiledAt string) int {
	return api.PolicyVersionForUI(db, connectorID, policyHash, compiledAt)
}

func (s *Server) workspaceIDFromRequest(r *http.Request) string {
	// First check if workspace context is already in the request context (set by workspaceAuth middleware).
	if wid := workspaceIDFromContext(r.Context()); wid != "" {
		return wid
	}
	// Try to extract from JWT in Authorization header or cookie.
	if len(s.JWTSecret) == 0 {
		return ""
	}
	tokenStr := ""
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		tokenStr = cookie.Value
	} else {
		auth := r.Header.Get("Authorization")
		if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
			tokenStr = after
		}
	}
	if tokenStr == "" {
		return ""
	}
	_, _, wsID, _, _, err := workspaceClaimsFromJWT(tokenStr, s.JWTSecret)
	if err != nil {
		return ""
	}
	return wsID
}

// wsWhere returns a SQL WHERE clause fragment and args for workspace scoping.
// If workspaceID is empty, returns empty string and nil args (no filtering).
// tableAlias is optional (e.g. "r" for "r.workspace_id = ?").
func wsWhere(workspaceID, tableAlias string) (string, []interface{}) {
	if workspaceID == "" {
		return "", nil
	}
	col := "workspace_id"
	if tableAlias != "" {
		col = tableAlias + ".workspace_id"
	}
	return " AND " + col + " = ?", []interface{}{workspaceID}
}

// wsWhereOnly returns a WHERE clause (not AND) for workspace scoping.
func wsWhereOnly(workspaceID, tableAlias string) (string, []interface{}) {
	if workspaceID == "" {
		return "", nil
	}
	col := "workspace_id"
	if tableAlias != "" {
		col = tableAlias + ".workspace_id"
	}
	return " WHERE " + col + " = ?", []interface{}{workspaceID}
}
