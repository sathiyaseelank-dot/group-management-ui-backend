package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

func (s *Server) handleUIResources(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClause, wsArgs := wsWhereOnly(wsID, "")
		rows, err := db.Query(state.Rebind(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id FROM resources`+wsClause+` ORDER BY name ASC`), wsArgs...)
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
			NetworkID string  `json:"network_id"`
			Name      string  `json:"name"`
			Type      string  `json:"type"`
			Address   string  `json:"address"`
			Protocol  string  `json:"protocol"`
			PortFrom  *int    `json:"port_from"`
			PortTo    *int    `json:"port_to"`
			Alias     *string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Type == "" || req.Address == "" || req.Protocol == "" {
			http.Error(w, "name, type, address, and protocol are required", http.StatusBadRequest)
			return
		}
		ports := buildPorts(req.PortFrom, req.PortTo)
		id := fmt.Sprintf("res_%d", time.Now().UTC().UnixMilli())
		wsID := workspaceIDFromContext(r.Context())
		if _, err := db.Exec(state.Rebind(`INSERT INTO resources (id, name, type, address, ports, protocol, port_from, port_to, alias, description, remote_network_id, workspace_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			id, req.Name, req.Type, req.Address, ports, req.Protocol, nullInt(req.PortFrom), nullInt(req.PortTo), req.Alias, fmt.Sprintf("A new %s resource", strings.ToLower(req.Type)), req.NetworkID, wsID); err != nil {
			http.Error(w, "failed to create resource", http.StatusBadRequest)
			return
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
	switch r.Method {
	case http.MethodGet:
		row := db.QueryRow(state.Rebind(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id FROM resources WHERE id = ?`), resourceID)
		res, ok := scanUIResource(row)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"resource":    nil,
				"accessRules": []uiAccessRule{},
			})
			return
		}
		accessRows, _ := db.Query(state.Rebind(`SELECT id, name, resource_id, enabled, created_at, updated_at FROM access_rules WHERE resource_id = ? ORDER BY created_at ASC`), resourceID)
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
			NetworkID string  `json:"network_id"`
			Name      string  `json:"name"`
			Type      string  `json:"type"`
			Address   string  `json:"address"`
			Protocol  string  `json:"protocol"`
			PortFrom  *int    `json:"port_from"`
			PortTo    *int    `json:"port_to"`
			Alias     *string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Type == "" || req.Address == "" || req.Protocol == "" {
			http.Error(w, "name, type, address, and protocol are required", http.StatusBadRequest)
			return
		}
		ports := buildPorts(req.PortFrom, req.PortTo)
		_, err := db.Exec(state.Rebind(`UPDATE resources SET name = ?, type = ?, address = ?, ports = ?, protocol = ?, port_from = ?, port_to = ?, alias = ?, remote_network_id = ? WHERE id = ?`),
			req.Name, req.Type, req.Address, ports, req.Protocol, nullInt(req.PortFrom), nullInt(req.PortTo), req.Alias, req.NetworkID, resourceID)
		if err != nil {
			http.Error(w, "failed to update resource", http.StatusBadRequest)
			return
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
