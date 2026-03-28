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

const agentShutdownAckWait = 5 * time.Second

func waitForAgentShutdownAck(db *sql.DB, agentID, reason string, timeout time.Duration) bool {
	if db == nil || strings.TrimSpace(agentID) == "" {
		return false
	}
	want := fmt.Sprintf("shutdown ack: firewall cleanup complete reason=%s", reason)
	deadline := time.Now().Add(timeout)
	for {
		var count int
		if err := db.QueryRow(
			state.Rebind(`SELECT COUNT(*) FROM agent_logs WHERE agent_id = ? AND message = ?`),
			agentID,
			want,
		).Scan(&count); err == nil && count > 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Server) handleUIAgents(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClauseWithAlias, wsArgsWithAlias := wsWhereOnly(wsID, "t")
		rows, err := db.Query(state.Rebind(`SELECT t.id, t.name, t.status, t.version, t.hostname, t.remote_network_id, t.connector_id, COALESCE(c.name, '') as connector_name, t.revoked, t.installed, CAST(t.last_seen AS TEXT) as last_seen, t.last_seen_at, t.ip FROM agents t LEFT JOIN connectors c ON t.connector_id = c.id`+wsClauseWithAlias+` ORDER BY t.name ASC`), wsArgsWithAlias...)
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
		nowUnix := time.Now().UTC().Unix()
		nowISO := isoStringNow()
		wsID := workspaceIDFromContext(r.Context())
		_, err := db.Exec(state.Rebind(`INSERT INTO agents (id, name, status, version, hostname, remote_network_id, connector_id, last_seen, last_seen_at, installed, workspace_id) VALUES (?, ?, 'offline', '1.0.0', '', ?, ?, ?, ?, 0, ?)`), id, req.Name, req.RemoteNetworkID, req.ConnectorID, nowUnix, nowISO, wsID)
		if err != nil {
			http.Error(w, "failed to create agent", http.StatusBadRequest)
			return
		}
		s.audit(r, "agent.create", id, "ok")
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
	wsID := workspaceIDFromContext(r.Context())
	wsClauseT, wsArgsT := wsWhere(wsID, "t")
	if len(parts) == 1 {
		if r.Method == http.MethodDelete {
			// Verify agent belongs to workspace before deleting
			delArgs := append([]interface{}{agentID}, wsArgsT...)
			var exists string
			if err := db.QueryRow(state.Rebind(`SELECT id FROM agents t WHERE t.id = ?`+wsClauseT), delArgs...).Scan(&exists); err != nil {
				http.Error(w, "agent not found", http.StatusNotFound)
				return
			}
			// Look up connector before deleting so we can refresh its allowlist.
			var connID string
			_ = db.QueryRow(state.Rebind(`SELECT connector_id FROM agents WHERE id = ?`), agentID).Scan(&connID)
			if s.ControlPlane != nil && connID != "" {
				payload, _ := json.Marshal(map[string]string{
					"agent_id": agentID,
					"reason":   "deleted",
				})
				if err := s.ControlPlane.SendToConnector(connID, "agent_shutdown", payload); err != nil {
					if !strings.Contains(err.Error(), "not connected") {
						log.Printf("agent delete shutdown send failed: agent_id=%s connector_id=%s err=%v", agentID, connID, err)
					}
				} else if !waitForAgentShutdownAck(db, agentID, "deleted", agentShutdownAckWait) {
					log.Printf("agent delete shutdown ack timed out: agent_id=%s connector_id=%s", agentID, connID)
				}
			}
			_ = state.DeleteAgentFromDB(db, agentID)
			// Delete enrollment token so the agent cannot re-enroll after deletion.
			if s.Tokens != nil {
				_ = s.Tokens.DeleteByConnectorID(agentID)
			}
			_, _ = db.Exec(state.Rebind(`DELETE FROM tokens WHERE connector_id = ?`), agentID)
			// Remove from in-memory registry.
			if s.Agents != nil {
				s.Agents.Delete(agentID)
			}
			if s.Allowlists != nil && connID != "" {
				_ = s.Allowlists.RefreshConnectorAllowlist(connID)
			}
			s.audit(r, "agent.delete", agentID, "ok")
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		getArgs := append([]interface{}{agentID}, wsArgsT...)
		row := db.QueryRow(state.Rebind(`SELECT t.id, t.name, t.status, t.version, t.hostname, t.remote_network_id, t.connector_id, COALESCE(c.name, '') as connector_name, t.revoked, t.installed, CAST(t.last_seen AS TEXT) as last_seen, t.last_seen_at, t.ip FROM agents t LEFT JOIN connectors c ON t.connector_id = c.id WHERE t.id = ?`+wsClauseT), getArgs...)
		agent, ok := scanUIAgent(row)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"agent":   nil,
				"network": nil,
				"logs":    []uiConnectorLog{},
			})
			return
		}
		wsClauseN, wsArgsN := wsWhere(wsID, "n")
		netArgs := append([]interface{}{agent.RemoteNetworkID}, wsArgsN...)
		networkRow := db.QueryRow(state.Rebind(`
			SELECT n.id, n.name, n.location,
				CAST(n.created_at AS TEXT) as created_at,
				CAST(n.updated_at AS TEXT) as updated_at,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
				(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
			FROM remote_networks n
			WHERE n.id = ?`+wsClauseN), netArgs...)
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
		logRows, _ := db.Query(state.Rebind(`SELECT id, timestamp, message FROM agent_logs WHERE agent_id = ? ORDER BY id ASC`), agentID)
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
		var connID string
		_ = db.QueryRow(state.Rebind(`SELECT connector_id FROM agents WHERE id = ?`), agentID).Scan(&connID)
		if s.ControlPlane != nil && connID != "" {
			payload, _ := json.Marshal(map[string]string{
				"agent_id": agentID,
				"reason":   "revoked",
			})
			if err := s.ControlPlane.SendToConnector(connID, "agent_shutdown", payload); err != nil {
				if !strings.Contains(err.Error(), "not connected") {
					log.Printf("agent revoke shutdown send failed: agent_id=%s connector_id=%s err=%v", agentID, connID, err)
				}
			} else if !waitForAgentShutdownAck(db, agentID, "revoked", agentShutdownAckWait) {
				log.Printf("agent revoke shutdown ack timed out: agent_id=%s connector_id=%s", agentID, connID)
			}
		}
		_ = state.RevokeAgentInDB(db, agentID)
		if s.Allowlists != nil && connID != "" {
			_ = s.Allowlists.RefreshConnectorAllowlist(connID)
		}
		nowISO := isoStringNow()
		_, _ = db.Exec(state.Rebind(`INSERT INTO agent_logs (agent_id, timestamp, message) VALUES (?, ?, ?)`), agentID, nowISO, "agent revoked")
		s.audit(r, "agent.revoke", agentID, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) >= 2 && parts[1] == "grant" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = state.GrantAgentInDB(db, agentID)
		nowISO := isoStringNow()
		_, _ = db.Exec(state.Rebind(`INSERT INTO agent_logs (agent_id, timestamp, message) VALUES (?, ?, ?)`), agentID, nowISO, "agent access granted")
		s.audit(r, "agent.grant", agentID, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	http.Error(w, "unknown subresource", http.StatusNotFound)
}
