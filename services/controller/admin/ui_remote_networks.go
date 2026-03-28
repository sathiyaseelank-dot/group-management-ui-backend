package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

func (s *Server) handleUIRemoteNetworks(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClause, wsArgs := wsWhereOnly(wsID, "n")
		rows, err := db.Query(state.Rebind(`
			SELECT n.id, n.name, n.location,
				CAST(n.created_at AS TEXT) as created_at,
				CAST(n.updated_at AS TEXT) as updated_at,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
				(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
			FROM remote_networks n`+wsClause+`
			ORDER BY n.created_at ASC`), wsArgs...)
		if err != nil {
			http.Error(w, "failed to list remote networks", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []uiRemoteNetwork{}
		for rows.Next() {
			var id, name, location string
			var created, updated sql.NullString
			var connCount, onlineCount, resCount int
			if err := rows.Scan(&id, &name, &location, &created, &updated, &connCount, &onlineCount, &resCount); err != nil {
				http.Error(w, "failed to read remote networks", http.StatusInternalServerError)
				return
			}
			createdAt := ""
			if created.Valid {
				createdAt = created.String
			}
			updatedAt := ""
			if updated.Valid {
				updatedAt = updated.String
			}
			if location == "" {
				location = "OTHER"
			}
			out = append(out, uiRemoteNetwork{
				ID:                   id,
				Name:                 name,
				Location:             location,
				ConnectorCount:       connCount,
				OnlineConnectorCount: onlineCount,
				ResourceCount:        resCount,
				CreatedAt:            createdAt,
				UpdatedAt:            updatedAt,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name     string `json:"name"`
			Location string `json:"location"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if req.Location == "" {
			req.Location = "OTHER"
		}
		id := fmt.Sprintf("net_%d", time.Now().UTC().UnixMilli())
		now := isoStringNow()
		wsID := workspaceIDFromContext(r.Context())
		_, err := db.Exec(state.Rebind(`INSERT INTO remote_networks (id, name, location, tags_json, created_at, updated_at, workspace_id) VALUES (?, ?, ?, ?, ?, ?, ?)`), id, req.Name, req.Location, "{}", now, now, wsID)
		if err != nil {
			log.Printf("remote network create: %v", err)
			http.Error(w, "failed to create remote network", http.StatusInternalServerError)
			return
		}
		s.audit(r, "network.create", id, "ok")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":        id,
			"name":      req.Name,
			"location":  req.Location,
			"createdAt": now,
			"updatedAt": now,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIRemoteNetworksSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/remote-networks/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "network id required", http.StatusBadRequest)
		return
	}
	networkID := strings.Split(path, "/")[0]
	wsID := workspaceIDFromContext(r.Context())
	wsClause, wsArgs := wsWhere(wsID, "")
	if r.Method == http.MethodDelete {
		// Collect connector IDs belonging to this network (scoped by workspace).
		delArgs := append([]interface{}{networkID}, wsArgs...)
		var connectorIDs []string
		cRows, _ := db.Query(state.Rebind(`SELECT id FROM connectors WHERE remote_network_id = ?`+wsClause), delArgs...)
		if cRows != nil {
			for cRows.Next() {
				var cid string
				if err := cRows.Scan(&cid); err == nil {
					connectorIDs = append(connectorIDs, cid)
				}
			}
			cRows.Close()
		}

		// Collect agent (tunneler) IDs belonging to this network.
		var agentIDs []string
		aRows, _ := db.Query(state.Rebind(`SELECT id FROM agents WHERE remote_network_id = ?`+wsClause), delArgs...)
		if aRows != nil {
			for aRows.Next() {
				var aid string
				if err := aRows.Scan(&aid); err == nil {
					agentIDs = append(agentIDs, aid)
				}
			}
			aRows.Close()
		}

		// Collect resource IDs for access-rule cleanup.
		var resourceIDs []string
		rRows, _ := db.Query(state.Rebind(`SELECT id FROM resources WHERE remote_network_id = ?`+wsClause), delArgs...)
		if rRows != nil {
			for rRows.Next() {
				var rid string
				if err := rRows.Scan(&rid); err == nil {
					resourceIDs = append(resourceIDs, rid)
				}
			}
			rRows.Close()
		}

		// Delete access rules and authorizations tied to these resources.
		for _, rid := range resourceIDs {
			_, _ = db.Exec(state.Rebind(`DELETE FROM access_rule_groups WHERE rule_id IN (SELECT id FROM access_rules WHERE resource_id = ?)`), rid)
			_, _ = db.Exec(state.Rebind(`DELETE FROM access_rules WHERE resource_id = ?`), rid)
			_, _ = db.Exec(state.Rebind(`DELETE FROM authorizations WHERE resource_id = ?`), rid)
			if s.ACLs != nil {
				s.ACLs.DeleteResource(rid)
			}
		}

		// Delete resources in this network.
		_, _ = db.Exec(state.Rebind(`DELETE FROM resources WHERE remote_network_id = ?`+wsClause), delArgs...)

		// Delete tokens and logs for each connector, then remove connectors.
		for _, cid := range connectorIDs {
			if s.Tokens != nil {
				_ = s.Tokens.DeleteByConnectorID(cid)
			}
			_, _ = db.Exec(state.Rebind(`DELETE FROM tokens WHERE connector_id = ?`), cid)
			_, _ = db.Exec(state.Rebind(`DELETE FROM connector_logs WHERE connector_id = ?`), cid)
			_, _ = db.Exec(state.Rebind(`DELETE FROM connector_policy_versions WHERE connector_id = ?`), cid)
			if s.Reg != nil {
				s.Reg.Delete(cid)
			}
		}
		_, _ = db.Exec(state.Rebind(`DELETE FROM connectors WHERE remote_network_id = ?`+wsClause), delArgs...)

		// Delete agents (tunnelers) in this network and clean in-memory registry.
		for _, aid := range agentIDs {
			if s.Agents != nil {
				s.Agents.Delete(aid)
			}
		}
		_, _ = db.Exec(state.Rebind(`DELETE FROM agents WHERE remote_network_id = ?`+wsClause), delArgs...)

		// Clean up join table and the network itself.
		_, _ = db.Exec(state.Rebind(`DELETE FROM remote_network_connectors WHERE network_id = ?`), networkID)
		delNetArgs := append([]interface{}{networkID}, wsArgs...)
		_, err := db.Exec(state.Rebind(`DELETE FROM remote_networks WHERE id = ?`+wsClause), delNetArgs...)
		if err != nil {
			http.Error(w, "failed to delete remote network", http.StatusInternalServerError)
			return
		}
		s.audit(r, "network.delete", networkID, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wsClauseN, wsArgsN := wsWhere(wsID, "n")
	getNetArgs := append([]interface{}{networkID}, wsArgsN...)
	row := db.QueryRow(state.Rebind(`
		SELECT n.id, n.name, n.location,
			CAST(n.created_at AS TEXT) as created_at,
			CAST(n.updated_at AS TEXT) as updated_at,
			(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
			(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
			(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
		FROM remote_networks n
		WHERE n.id = ?`+wsClauseN), getNetArgs...)
	var id, name, location string
	var created, updated sql.NullString
	var connCount, onlineCount, resCount int
	if err := row.Scan(&id, &name, &location, &created, &updated, &connCount, &onlineCount, &resCount); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"network": nil, "connectors": []uiConnector{}, "resources": []uiResource{}})
		return
	}
	if location == "" {
		location = "OTHER"
	}
	createdAt := ""
	if created.Valid {
		createdAt = created.String
	}
	updatedAt := ""
	if updated.Valid {
		updatedAt = updated.String
	}
	network := uiRemoteNetwork{
		ID:                   id,
		Name:                 name,
		Location:             location,
		ConnectorCount:       connCount,
		OnlineConnectorCount: onlineCount,
		ResourceCount:        resCount,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}
	connListArgs := append([]interface{}{networkID}, wsArgs...)
	connectorRows, _ := db.Query(state.Rebind(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version, private_ip, revoked, last_seen FROM connectors WHERE remote_network_id = ?`+wsClause+` ORDER BY name ASC`), connListArgs...)
	connectors := []uiConnector{}
	if connectorRows != nil {
		for connectorRows.Next() {
			if conn, ok := scanUIConnector(connectorRows); ok {
				connectors = append(connectors, conn)
			}
		}
		connectorRows.Close()
	}
	resListArgs := append([]interface{}{networkID}, wsArgs...)
	resourceRows, _ := db.Query(state.Rebind(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id, firewall_status FROM resources WHERE remote_network_id = ?`+wsClause+` ORDER BY name ASC`), resListArgs...)
	resources := []uiResource{}
	if resourceRows != nil {
		for resourceRows.Next() {
			if res, ok := scanUIResource(resourceRows); ok {
				resources = append(resources, res)
			}
		}
		resourceRows.Close()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"network":    network,
		"connectors": connectors,
		"resources":  resources,
	})
}
