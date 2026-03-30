package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

// resolveConnectorIDFromAddress finds the connector that owns the agent whose
// IP matches the resource address.  Returns "" if no match (backward compat).
func resolveConnectorIDFromAddress(db *sql.DB, address, networkID, wsID string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	var connID string
	q := `SELECT connector_id FROM agents WHERE ip = ? AND remote_network_id = ? AND COALESCE(TRIM(connector_id), '') <> '' LIMIT 1`
	if err := db.QueryRow(state.Rebind(q), address, networkID).Scan(&connID); err != nil {
		return ""
	}
	return strings.TrimSpace(connID)
}

func resolveResourceNetworkID(db *sql.DB, networkID, connectorID, wsID string) (string, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID != "" {
		return networkID, nil
	}
	if connectorID != "" {
		if resolved, err := lookupConnectorNetworkID(db, connectorID, wsID); err == nil && resolved != "" {
			return resolved, nil
		}
	}
	// Fallback: if there's exactly one remote network in this workspace, use it.
	wsClause, wsArgs := wsWhereOnly(wsID, "")
	rows, err := db.Query(state.Rebind(`SELECT id FROM remote_networks`+wsClause+` ORDER BY id ASC LIMIT 2`), wsArgs...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && strings.TrimSpace(id) != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no remote networks available")
	}
	return "", fmt.Errorf("remote network required")
}

func (s *Server) handleUIResources(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClause, wsArgs := wsWhereOnly(wsID, "")
		rows, err := db.Query(state.Rebind(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id, connector_id, firewall_status FROM resources`+wsClause+` ORDER BY name ASC`), wsArgs...)
		if err != nil {
			http.Error(w, "failed to list resources", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []uiResource{}
		for rows.Next() {
			if res, ok := scanUIResource(rows); ok {
				out = append(out, res)
			}
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			NetworkID   string  `json:"network_id"`
			ConnectorID string  `json:"connector_id"`
			Name        string  `json:"name"`
			Type        string  `json:"type"`
			Address     string  `json:"address"`
			Protocol    string  `json:"protocol"`
			PortFrom    *int    `json:"port_from"`
			PortTo      *int    `json:"port_to"`
			Alias       *string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Type == "" || req.Address == "" || req.Protocol == "" {
			http.Error(w, "name, type, address, and protocol are required", http.StatusBadRequest)
			return
		}
		wsID := workspaceIDFromContext(r.Context())
		networkID, err := resolveResourceNetworkID(db, req.NetworkID, req.ConnectorID, wsID)
		if err != nil {
			http.Error(w, "failed to resolve remote network", http.StatusBadRequest)
			return
		}
		connectorID := strings.TrimSpace(req.ConnectorID)
		if connectorID == "" {
			connectorID = resolveConnectorIDFromAddress(db, req.Address, networkID, wsID)
		}
		ports := buildPorts(req.PortFrom, req.PortTo)
		id := fmt.Sprintf("res_%d", time.Now().UTC().UnixMilli())
		if _, err := db.Exec(state.Rebind(`INSERT INTO resources (id, name, type, address, ports, protocol, port_from, port_to, alias, description, remote_network_id, connector_id, workspace_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			id, req.Name, req.Type, req.Address, ports, req.Protocol, nullInt(req.PortFrom), nullInt(req.PortTo), req.Alias, fmt.Sprintf("A new %s resource", strings.ToLower(req.Type)), networkID, connectorID, wsID); err != nil {
			http.Error(w, "failed to create resource", http.StatusBadRequest)
			return
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		s.audit(r, "resource.create", id, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIResourcesBatch(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Resources []struct {
			NetworkID string  `json:"network_id"`
			Name      string  `json:"name"`
			Type      string  `json:"type"`
			Address   string  `json:"address"`
			Protocol  string  `json:"protocol"`
			PortFrom  *int    `json:"port_from"`
			PortTo    *int    `json:"port_to"`
			Alias     *string `json:"alias"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(req.Resources) == 0 {
		http.Error(w, "resources array is required", http.StatusBadRequest)
		return
	}

	wsID := workspaceIDFromContext(r.Context())
	created := 0
	var errors []string
	for _, res := range req.Resources {
		if res.Name == "" || res.Type == "" || res.Address == "" || res.Protocol == "" {
			errors = append(errors, fmt.Sprintf("skipping resource with missing fields: %s", res.Name))
			continue
		}
		networkID, err := resolveResourceNetworkID(db, res.NetworkID, "", wsID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("network error for %s: %v", res.Name, err))
			continue
		}
		batchConnID := resolveConnectorIDFromAddress(db, res.Address, networkID, wsID)
		ports := buildPorts(res.PortFrom, res.PortTo)
		id := fmt.Sprintf("res_%d_%d", time.Now().UTC().UnixMilli(), created)
		if _, err := db.Exec(state.Rebind(`INSERT INTO resources (id, name, type, address, ports, protocol, port_from, port_to, alias, description, remote_network_id, connector_id, workspace_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			id, res.Name, res.Type, res.Address, ports, res.Protocol, nullInt(res.PortFrom), nullInt(res.PortTo), res.Alias, fmt.Sprintf("A new %s resource", strings.ToLower(res.Type)), networkID, batchConnID, wsID); err != nil {
			errors = append(errors, fmt.Sprintf("insert error for %s: %v", res.Name, err))
			continue
		}
		created++
	}

	if s.ACLNotify != nil && created > 0 {
		s.ACLNotify.NotifyPolicyChange()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"created": created,
		"errors":  errors,
	})
}

func (s *Server) handleUIResourcesSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/resources/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "resource id required", http.StatusBadRequest)
		return
	}
	resourceID := strings.Split(path, "/")[0]
	wsID := workspaceIDFromContext(r.Context())
	wsClause, wsArgs := wsWhere(wsID, "")
	switch r.Method {
	case http.MethodGet:
		args := append([]interface{}{resourceID}, wsArgs...)
		row := db.QueryRow(state.Rebind(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id, connector_id, firewall_status FROM resources WHERE id = ?`+wsClause), args...)
		res, ok := scanUIResource(row)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"resource":    nil,
				"accessRules": []uiAccessRule{},
			})
			return
		}
		arArgs := append([]interface{}{resourceID}, wsArgs...)
		accessRows, _ := db.Query(state.Rebind(`SELECT id, name, resource_id, enabled, created_at, updated_at FROM access_rules WHERE resource_id = ?`+wsClause+` ORDER BY created_at ASC`), arArgs...)
		accessRules := []uiAccessRule{}
		if accessRows != nil {
			groupStmt, _ := db.Prepare(state.Rebind(`SELECT group_id FROM access_rule_groups WHERE rule_id = ? ORDER BY group_id ASC`))
			for accessRows.Next() {
				var ar uiAccessRule
				var enabled int
				if err := accessRows.Scan(&ar.ID, &ar.Name, &ar.ResourceID, &enabled, &ar.CreatedAt, &ar.UpdatedAt); err == nil {
					ar.Enabled = enabled != 0
					ar.AllowedGroups = []string{}
					if groupStmt != nil {
						rows, _ := groupStmt.Query(ar.ID)
						for rows != nil && rows.Next() {
							var gid string
							if err := rows.Scan(&gid); err == nil {
								ar.AllowedGroups = append(ar.AllowedGroups, gid)
							}
						}
						if rows != nil {
							rows.Close()
						}
					}
					accessRules = append(accessRules, ar)
				}
			}
			if groupStmt != nil {
				groupStmt.Close()
			}
			accessRows.Close()
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"resource":    res,
			"accessRules": accessRules,
		})
	case http.MethodPut:
		var req struct {
			NetworkID   string  `json:"network_id"`
			ConnectorID string  `json:"connector_id"`
			Name        string  `json:"name"`
			Type        string  `json:"type"`
			Address     string  `json:"address"`
			Protocol    string  `json:"protocol"`
			PortFrom    *int    `json:"port_from"`
			PortTo      *int    `json:"port_to"`
			Alias       *string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Type == "" || req.Address == "" || req.Protocol == "" {
			http.Error(w, "name, type, address, and protocol are required", http.StatusBadRequest)
			return
		}
		resolvedNetworkID, err := resolveResourceNetworkID(db, req.NetworkID, req.ConnectorID, wsID)
		if err != nil {
			var current string
			fallbackArgs := append([]interface{}{resourceID}, wsArgs...)
			if scanErr := db.QueryRow(state.Rebind(`SELECT remote_network_id FROM resources WHERE id = ?`+wsClause), fallbackArgs...).Scan(&current); scanErr == nil && strings.TrimSpace(current) != "" {
				resolvedNetworkID = current
			} else {
				http.Error(w, "failed to resolve remote network", http.StatusBadRequest)
				return
			}
		}
		connectorID := strings.TrimSpace(req.ConnectorID)
		if connectorID == "" {
			connectorID = resolveConnectorIDFromAddress(db, req.Address, resolvedNetworkID, wsID)
		}
		ports := buildPorts(req.PortFrom, req.PortTo)
		updateArgs := append([]interface{}{req.Name, req.Type, req.Address, ports, req.Protocol, nullInt(req.PortFrom), nullInt(req.PortTo), req.Alias, resolvedNetworkID, connectorID, resourceID}, wsArgs...)
		_, err = db.Exec(state.Rebind(`UPDATE resources SET name = ?, type = ?, address = ?, ports = ?, protocol = ?, port_from = ?, port_to = ?, alias = ?, remote_network_id = ?, connector_id = ? WHERE id = ?`+wsClause),
			updateArgs...)
		if err != nil {
			http.Error(w, "failed to update resource", http.StatusBadRequest)
			return
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		s.audit(r, "resource.update", resourceID, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodPatch:
		var req struct {
			FirewallStatus string `json:"firewall_status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.FirewallStatus != "protected" && req.FirewallStatus != "unprotected" {
			http.Error(w, "firewall_status must be 'protected' or 'unprotected'", http.StatusBadRequest)
			return
		}
		patchArgs := append([]interface{}{req.FirewallStatus, resourceID}, wsArgs...)
		_, err := db.Exec(state.Rebind(`UPDATE resources SET firewall_status = ? WHERE id = ?`+wsClause), patchArgs...)
		if err != nil {
			http.Error(w, "failed to update firewall status", http.StatusInternalServerError)
			return
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		s.audit(r, "resource.update_firewall", resourceID, "ok")
		writeJSON(w, http.StatusOK, map[string]string{"firewall_status": req.FirewallStatus})
	case http.MethodDelete:
		// Scope access rule cleanup by workspace
		delRuleArgs := append([]interface{}{resourceID}, wsArgs...)
		if _, err := db.Exec(state.Rebind(`DELETE FROM access_rule_groups WHERE rule_id IN (SELECT id FROM access_rules WHERE resource_id = ?`+wsClause+`)`), delRuleArgs...); err != nil {
			http.Error(w, "failed to delete resource access rules", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec(state.Rebind(`DELETE FROM access_rules WHERE resource_id = ?`+wsClause), delRuleArgs...); err != nil {
			http.Error(w, "failed to delete resource access rules", http.StatusInternalServerError)
			return
		}
		delResArgs := append([]interface{}{resourceID}, wsArgs...)
		if _, err := db.Exec(state.Rebind(`DELETE FROM resources WHERE id = ?`+wsClause), delResArgs...); err != nil {
			http.Error(w, "failed to delete resource", http.StatusInternalServerError)
			return
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		s.audit(r, "resource.delete", resourceID, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
