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

func (s *Server) handleUIAgents(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClauseWithAlias, wsArgsWithAlias := wsWhereOnly(wsID, "t")
		rows, err := db.Query(state.Rebind(`SELECT t.id, t.name, t.status, t.version, t.hostname, t.remote_network_id, t.connector_id, COALESCE(c.name, '') as connector_name, t.revoked, t.installed, CAST(t.last_seen AS TEXT) as last_seen, t.last_seen_at FROM tunnelers t LEFT JOIN connectors c ON t.connector_id = c.id`+wsClauseWithAlias+` ORDER BY t.name ASC`), wsArgsWithAlias...)
		if err != nil {
			http.Error(w, "failed to list agents", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []uiAgent{}
		for rows.Next() {
			if t, ok := scanUIAgent(rows); ok {
				out = append(out, t)
			}
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name            string `json:"name"`
			RemoteNetworkID string `json:"remoteNetworkId"`
			ConnectorID     string `json:"connectorId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("tun_%d", time.Now().UTC().UnixMilli())
		hostname := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")) + ".local"
		nowUnix := time.Now().UTC().Unix()
		nowISO := isoStringNow()
		wsID := workspaceIDFromContext(r.Context())
		_, err := db.Exec(state.Rebind(`INSERT INTO tunnelers (id, name, status, version, hostname, remote_network_id, connector_id, last_seen, last_seen_at, installed, workspace_id) VALUES (?, ?, 'offline', '1.0.0', ?, ?, ?, ?, ?, 0, ?)`), id, req.Name, hostname, req.RemoteNetworkID, req.ConnectorID, nowUnix, nowISO, wsID)
		if err != nil {
			http.Error(w, "failed to create agent", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIAgentsSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	agentID := parts[0]
	if len(parts) == 1 {
		if r.Method == http.MethodDelete {
			_ = state.DeleteTunnelerFromDB(db, agentID)
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		row := db.QueryRow(state.Rebind(`SELECT t.id, t.name, t.status, t.version, t.hostname, t.remote_network_id, t.connector_id, COALESCE(c.name, '') as connector_name, t.revoked, t.installed, CAST(t.last_seen AS TEXT) as last_seen, t.last_seen_at FROM tunnelers t LEFT JOIN connectors c ON t.connector_id = c.id WHERE t.id = ?`), agentID)
		agent, ok := scanUIAgent(row)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"agent":   nil,
				"network": nil,
				"logs":    []uiConnectorLog{},
			})
			return
		}
		networkRow := db.QueryRow(state.Rebind(`
			SELECT n.id, n.name, n.location,
				CAST(n.created_at AS TEXT) as created_at,
				CAST(n.updated_at AS TEXT) as updated_at,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
				(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
			FROM remote_networks n
			WHERE n.id = ?`), agent.RemoteNetworkID)
		var network *uiRemoteNetwork
		{
			var id, name, location string
			var created, updated sql.NullString
			var connCount, onlineCount, resCount int
			if err := networkRow.Scan(&id, &name, &location, &created, &updated, &connCount, &onlineCount, &resCount); err == nil {
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
				n := uiRemoteNetwork{
					ID:                   id,
					Name:                 name,
					Location:             location,
					ConnectorCount:       connCount,
					OnlineConnectorCount: onlineCount,
					ResourceCount:        resCount,
					CreatedAt:            createdAt,
					UpdatedAt:            updatedAt,
				}
				network = &n
			}
		}
		logs := []uiConnectorLog{}
		logRows, _ := db.Query(state.Rebind(`SELECT id, timestamp, message FROM tunneler_logs WHERE tunneler_id = ? ORDER BY id ASC`), agentID)
		if logRows != nil {
			for logRows.Next() {
				var l uiConnectorLog
				if err := logRows.Scan(&l.ID, &l.Timestamp, &l.Message); err == nil {
					logs = append(logs, l)
				}
			}
			logRows.Close()
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"agent":   agent,
			"network": network,
			"logs":    logs,
		})
		return
	}
	if len(parts) >= 2 && parts[1] == "revoke" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = state.RevokeTunnelerInDB(db, agentID)
		nowISO := isoStringNow()
		_, _ = db.Exec(state.Rebind(`INSERT INTO tunneler_logs (tunneler_id, timestamp, message) VALUES (?, ?, ?)`), agentID, nowISO, "agent revoked")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) >= 2 && parts[1] == "grant" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = state.GrantTunnelerInDB(db, agentID)
		nowISO := isoStringNow()
		_, _ = db.Exec(state.Rebind(`INSERT INTO tunneler_logs (tunneler_id, timestamp, message) VALUES (?, ?, ?)`), agentID, nowISO, "agent access granted")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	http.Error(w, "unknown subresource", http.StatusNotFound)
}
