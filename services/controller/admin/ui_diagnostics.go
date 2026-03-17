package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"controller/state"
)

type uiConnectorDiagnostic struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	StreamActive     bool    `json:"streamActive"`
	StalenessSeconds float64 `json:"stalenessSeconds"`
	LastSeenAt       *string `json:"lastSeenAt"`
	RemoteNetworkID  string  `json:"remoteNetworkId"`
}

type uiTunnelerDiagnostic struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	LastSeenAt *string `json:"lastSeenAt"`
}

// handleUIDiagnostics returns aggregate health for all connectors and tunnelers.
func (s *Server) handleUIDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db, ok := s.uiDB(w)
	if !ok {
		return
	}

	rows, err := db.Query(state.Rebind(`SELECT id, name, status, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at FROM connectors ORDER BY name ASC`))
	if err != nil {
		http.Error(w, "failed to list connectors", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	connectors := []uiConnectorDiagnostic{}
	now := time.Now().UTC()
	for rows.Next() {
		var id, status string
		var name, remoteNetworkID, lastSeen, lastSeenAt sql.NullString
		if err := rows.Scan(&id, &name, &status, &remoteNetworkID, &lastSeen, &lastSeenAt); err != nil {
			http.Error(w, "failed to read connectors", http.StatusInternalServerError)
			return
		}
		var stalenessSeconds float64
		var lastSeenAtPtr *string
		if lastSeenAt.Valid && lastSeenAt.String != "" {
			lastSeenAtPtr = &lastSeenAt.String
			if t, err := time.Parse("2006-01-02T15:04:05.000Z", lastSeenAt.String); err == nil {
				stalenessSeconds = now.Sub(t).Seconds()
			}
		} else if lastSeen.Valid && lastSeen.String != "" {
			if ts, err := strconv.ParseInt(lastSeen.String, 10, 64); err == nil && ts > 0 {
				iso := isoStringFromUnix(ts)
				lastSeenAtPtr = &iso
				stalenessSeconds = float64(now.Unix() - ts)
			}
		}
		streamActive := false
		if s.StreamChecker != nil {
			streamActive = s.StreamChecker.IsStreamActive(id)
		}
		connectors = append(connectors, uiConnectorDiagnostic{
			ID:               id,
			Name:             strings.TrimSpace(name.String),
			Status:           strings.TrimSpace(status),
			StreamActive:     streamActive,
			StalenessSeconds: stalenessSeconds,
			LastSeenAt:       lastSeenAtPtr,
			RemoteNetworkID:  strings.TrimSpace(remoteNetworkID.String),
		})
	}

	tRows, err := db.Query(state.Rebind(`SELECT id, name, status, last_seen_at FROM agents ORDER BY name ASC`))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"connectors": connectors,
			"agents":  []uiTunnelerDiagnostic{},
		})
		return
	}
	defer tRows.Close()

	tunnelers := []uiTunnelerDiagnostic{}
	for tRows.Next() {
		var id, status string
		var name, lastSeenAt sql.NullString
		if err := tRows.Scan(&id, &name, &status, &lastSeenAt); err != nil {
			continue
		}
		var lastSeenAtPtr *string
		if lastSeenAt.Valid && lastSeenAt.String != "" {
			lastSeenAtPtr = &lastSeenAt.String
		}
		tunnelers = append(tunnelers, uiTunnelerDiagnostic{
			ID:         id,
			Name:       strings.TrimSpace(name.String),
			Status:     strings.TrimSpace(status),
			LastSeenAt: lastSeenAtPtr,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connectors": connectors,
		"agents":  tunnelers,
	})
}

// handleUIDiagnosticsPing checks the gRPC stream status for one connector.
func (s *Server) handleUIDiagnosticsPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	connectorID := strings.TrimPrefix(r.URL.Path, "/api/diagnostics/ping/")
	connectorID = strings.Trim(connectorID, "/")
	if connectorID == "" {
		http.Error(w, "connector id required", http.StatusBadRequest)
		return
	}

	var status string
	var lastSeen, lastSeenAt sql.NullString
	err := db.QueryRow(state.Rebind(`SELECT status, CAST(last_seen AS TEXT) as last_seen, last_seen_at FROM connectors WHERE id = ?`), connectorID).
		Scan(&status, &lastSeen, &lastSeenAt)
	if err == sql.ErrNoRows {
		http.Error(w, "connector not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to query connector", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	var stalenessSeconds float64
	var lastSeenAtPtr *string
	if lastSeenAt.Valid && lastSeenAt.String != "" {
		lastSeenAtPtr = &lastSeenAt.String
		if t, err := time.Parse("2006-01-02T15:04:05.000Z", lastSeenAt.String); err == nil {
			stalenessSeconds = now.Sub(t).Seconds()
		}
	} else if lastSeen.Valid && lastSeen.String != "" {
		if ts, err := strconv.ParseInt(lastSeen.String, 10, 64); err == nil && ts > 0 {
			iso := isoStringFromUnix(ts)
			lastSeenAtPtr = &iso
			stalenessSeconds = float64(now.Unix() - ts)
		}
	}

	streamActive := false
	if s.StreamChecker != nil {
		streamActive = s.StreamChecker.IsStreamActive(connectorID)
	}
	message := "stream inactive"
	if streamActive {
		message = "stream active"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connectorId":      connectorID,
		"streamActive":     streamActive,
		"stalenessSeconds": stalenessSeconds,
		"lastSeenAt":       lastSeenAtPtr,
		"message":          message,
	})
}

type traceHop struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Healthy bool   `json:"healthy"`
}

// handleUIDiagnosticsTrace evaluates the access path from a user to a resource.
func (s *Server) handleUIDiagnosticsTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db, ok := s.uiDB(w)
	if !ok {
		return
	}

	var req struct {
		UserID     string `json:"userId"`
		ResourceID string `json:"resourceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.ResourceID == "" {
		http.Error(w, "userId and resourceId are required", http.StatusBadRequest)
		return
	}

	// Fetch user.
	var userName, userEmail, userStatus string
	if err := db.QueryRow(state.Rebind(`SELECT name, email, status FROM users WHERE id = ?`), req.UserID).
		Scan(&userName, &userEmail, &userStatus); err == sql.ErrNoRows {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to query user", http.StatusInternalServerError)
		return
	}

	// Fetch user's groups.
	groupRows, err := db.Query(state.Rebind(`SELECT ug.id, ug.name FROM user_group_members ugm JOIN user_groups ug ON ug.id = ugm.group_id WHERE ugm.user_id = ?`), req.UserID)
	if err != nil {
		http.Error(w, "failed to query user groups", http.StatusInternalServerError)
		return
	}
	defer groupRows.Close()
	type simpleGroup struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	userGroups := []simpleGroup{}
	userGroupIDs := map[string]string{}
	for groupRows.Next() {
		var gid, gname string
		if err := groupRows.Scan(&gid, &gname); err == nil {
			userGroups = append(userGroups, simpleGroup{ID: gid, Name: gname})
			userGroupIDs[gid] = gname
		}
	}

	// Fetch resource.
	var resName, resAddress, resType string
	var resRemoteNetID sql.NullString
	if err := db.QueryRow(state.Rebind(`SELECT name, address, type, remote_network_id FROM resources WHERE id = ?`), req.ResourceID).
		Scan(&resName, &resAddress, &resType, &resRemoteNetID); err == sql.ErrNoRows {
		http.Error(w, "resource not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to query resource", http.StatusInternalServerError)
		return
	}
	_ = resAddress

	// Fetch access rules for the resource.
	ruleRows, err := db.Query(state.Rebind(`SELECT ar.id, ar.name, ar.enabled FROM access_rules ar WHERE ar.resource_id = ?`), req.ResourceID)
	if err != nil {
		http.Error(w, "failed to query access rules", http.StatusInternalServerError)
		return
	}
	defer ruleRows.Close()

	type simpleRule struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	type ruleWithGroups struct {
		simpleRule
		groups []string
	}
	var allRules []ruleWithGroups
	for ruleRows.Next() {
		var rid, rname string
		var enabled int
		if err := ruleRows.Scan(&rid, &rname, &enabled); err == nil {
			allRules = append(allRules, ruleWithGroups{
				simpleRule: simpleRule{ID: rid, Name: rname, Enabled: enabled != 0},
			})
		}
	}
	for i := range allRules {
		argRows, err := db.Query(state.Rebind(`SELECT group_id FROM access_rule_groups WHERE rule_id = ?`), allRules[i].ID)
		if err == nil {
			for argRows.Next() {
				var gid string
				if err := argRows.Scan(&gid); err == nil {
					allRules[i].groups = append(allRules[i].groups, gid)
				}
			}
			argRows.Close()
		}
	}

	// Determine access.
	allowed := false
	reason := "No matching access rules found"
	matchedRulesOut := []simpleRule{}
	matchedGroupName := ""
	matchedRuleName := ""
	for _, rule := range allRules {
		if !rule.Enabled {
			continue
		}
		for _, gid := range rule.groups {
			if gname, ok := userGroupIDs[gid]; ok {
				allowed = true
				matchedGroupName = gname
				matchedRuleName = rule.Name
				matchedRulesOut = append(matchedRulesOut, rule.simpleRule)
				break
			}
		}
	}
	if allowed {
		reason = fmt.Sprintf("Group '%s' has access via rule '%s'", matchedGroupName, matchedRuleName)
	} else if len(allRules) > 0 {
		reason = "User is not in any group that has access to this resource"
	}

	// Build path.
	userHealthy := strings.ToLower(userStatus) == "active"
	path := []traceHop{
		{Type: "user", ID: req.UserID, Name: userName, Status: strings.ToLower(userStatus), Healthy: userHealthy},
	}
	if len(userGroups) > 0 {
		g := userGroups[0]
		path = append(path, traceHop{Type: "group", ID: g.ID, Name: g.Name, Status: "member", Healthy: true})
	}
	resourceStatus := "denied"
	if allowed {
		resourceStatus = "allowed"
	}
	path = append(path, traceHop{Type: "resource", ID: req.ResourceID, Name: resName, Status: resourceStatus, Healthy: allowed})

	// Remote network and connectors.
	if resRemoteNetID.Valid && resRemoteNetID.String != "" {
		var rnName string
		if rnErr := db.QueryRow(state.Rebind(`SELECT name FROM remote_networks WHERE id = ?`), resRemoteNetID.String).Scan(&rnName); rnErr == nil {
			path = append(path, traceHop{Type: "remote_network", ID: resRemoteNetID.String, Name: rnName, Status: "active", Healthy: true})
		}
		cRows, err := db.Query(state.Rebind(`SELECT id, name, status FROM connectors WHERE remote_network_id = ? AND status = 'online' LIMIT 1`), resRemoteNetID.String)
		if err == nil {
			defer cRows.Close()
			for cRows.Next() {
				var cid, cname, cstatus string
				if err := cRows.Scan(&cid, &cname, &cstatus); err == nil {
					streamActive := false
					if s.StreamChecker != nil {
						streamActive = s.StreamChecker.IsStreamActive(cid)
					}
					path = append(path, traceHop{Type: "connector", ID: cid, Name: cname, Status: cstatus, Healthy: streamActive})
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"allowed":      allowed,
		"reason":       reason,
		"path":         path,
		"userGroups":   userGroups,
		"matchedRules": matchedRulesOut,
	})
}
