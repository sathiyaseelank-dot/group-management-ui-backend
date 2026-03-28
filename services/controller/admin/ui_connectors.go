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

func (s *Server) handleUIConnectors(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClause, wsArgs := wsWhereOnly(wsID, "")
		rows, err := db.Query(state.Rebind(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version, private_ip, revoked, last_seen FROM connectors`+wsClause+` ORDER BY name ASC`), wsArgs...)
		if err != nil {
			http.Error(w, "failed to list connectors", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []uiConnector{}
		for rows.Next() {
			if conn, ok := scanUIConnector(rows); ok {
				out = append(out, conn)
			}
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name            string `json:"name"`
			RemoteNetworkID string `json:"remoteNetworkId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.RemoteNetworkID == "" {
			http.Error(w, "name and remoteNetworkId are required", http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("con_%d", time.Now().UTC().UnixMilli())
		hostname := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")) + ".local"
		nowUnix := time.Now().UTC().Unix()
		nowISO := isoStringNow()
		wsID := workspaceIDFromContext(r.Context())
		_, err := db.Exec(state.Rebind(`INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, last_policy_version, last_seen_at, installed, workspace_id) VALUES (?, ?, 'offline', '1.0.0', ?, ?, ?, 0, ?, 0, ?)`), id, req.Name, hostname, req.RemoteNetworkID, nowUnix, nowISO, wsID)
		if err != nil {
			http.Error(w, "failed to create connector", http.StatusBadRequest)
			return
		}
		s.audit(r, "connector.create", id, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIConnectorsSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/connectors/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "connector id required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	connectorID := parts[0]
	wsID := workspaceIDFromContext(r.Context())
	wsClause, wsArgs := wsWhere(wsID, "")
	if len(parts) == 1 {
		if r.Method == http.MethodDelete {
			if s.ControlPlane != nil {
				payload, _ := json.Marshal(map[string]string{
					"connector_id": connectorID,
					"reason":       "deleted",
				})
				if err := s.ControlPlane.SendToConnector(connectorID, "connector_shutdown", payload); err != nil && !strings.Contains(err.Error(), "not connected") {
					log.Printf("connector delete shutdown send failed: connector_id=%s err=%v", connectorID, err)
				}
			}
			// Cascade-delete agents (tunnelers) bound to this connector.
			var agentIDs []string
			delArgs := append([]interface{}{connectorID}, wsArgs...)
			aRows, _ := db.Query(state.Rebind(`SELECT id FROM agents WHERE connector_id = ?`+wsClause), delArgs...)
			if aRows != nil {
				for aRows.Next() {
					var aid string
					if err := aRows.Scan(&aid); err == nil {
						agentIDs = append(agentIDs, aid)
					}
				}
				aRows.Close()
			}
			for _, aid := range agentIDs {
				if s.Agents != nil {
					s.Agents.Delete(aid)
				}
			}
			_, _ = db.Exec(state.Rebind(`DELETE FROM agents WHERE connector_id = ?`), connectorID)

			// Delete connector tokens, logs, and policy versions.
			if s.Tokens != nil {
				_ = s.Tokens.DeleteByConnectorID(connectorID)
			}
			_, _ = db.Exec(state.Rebind(`DELETE FROM tokens WHERE connector_id = ?`), connectorID)
			_, _ = db.Exec(state.Rebind(`DELETE FROM connector_logs WHERE connector_id = ?`), connectorID)
			_, _ = db.Exec(state.Rebind(`DELETE FROM connector_policy_versions WHERE connector_id = ?`), connectorID)
			_, _ = db.Exec(state.Rebind(`DELETE FROM remote_network_connectors WHERE connector_id = ?`), connectorID)

			// Remove connector from in-memory registry.
			if s.Reg != nil {
				s.Reg.Delete(connectorID)
			}
			if s.ACLs != nil && s.ACLs.DB() != nil {
				// Revoke (tombstone) so the running connector process cannot re-register via gRPC.
				// SaveConnectorToDB checks revoked=1 and skips the upsert - never hard-delete.
				_ = state.RevokeConnectorInDB(s.ACLs.DB(), connectorID)
			}
			revokeArgs := append([]interface{}{connectorID}, wsArgs...)
			_, _ = db.Exec(state.Rebind(`UPDATE connectors SET revoked = 1, status = 'offline' WHERE id = ?`+wsClause), revokeArgs...)

			s.audit(r, "connector.delete", connectorID, "ok")
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		getArgs := append([]interface{}{connectorID}, wsArgs...)
		row := db.QueryRow(state.Rebind(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version, private_ip, revoked, last_seen FROM connectors WHERE id = ?`+wsClause), getArgs...)
		connector, ok := scanUIConnector(row)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"connector": nil,
				"network":   nil,
				"logs":      []uiConnectorLog{},
			})
			return
		}
		wsClauseN, wsArgsN := wsWhere(wsID, "n")
		netArgs := append([]interface{}{connector.RemoteNetworkID}, wsArgsN...)
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
		logRows, _ := db.Query(state.Rebind(`SELECT id, timestamp, message FROM connector_logs WHERE connector_id = ? ORDER BY id ASC`), connectorID)
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
			"connector": connector,
			"network":   network,
			"logs":      logs,
		})
		return
	}
	if len(parts) >= 2 && parts[1] == "revoke" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.ControlPlane != nil {
			payload, _ := json.Marshal(map[string]string{
				"connector_id": connectorID,
				"reason":       "revoked",
			})
			if err := s.ControlPlane.SendToConnector(connectorID, "connector_shutdown", payload); err != nil && !strings.Contains(err.Error(), "not connected") {
				log.Printf("connector revoke shutdown send failed: connector_id=%s err=%v", connectorID, err)
			}
		}
		if s.Reg != nil {
			s.Reg.Delete(connectorID)
		}
		if s.ACLs != nil && s.ACLs.DB() != nil {
			_ = state.RevokeConnectorInDB(s.ACLs.DB(), connectorID)
		}
		if s.Tokens != nil {
			_ = s.Tokens.DeleteByConnectorID(connectorID)
		}
		nowISO := isoStringNow()
		_, _ = db.Exec(state.Rebind(`INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)`), connectorID, nowISO, "connector revoked")
		s.audit(r, "connector.revoke", connectorID, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) >= 2 && parts[1] == "grant" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.ACLs != nil && s.ACLs.DB() != nil {
			_ = state.GrantConnectorInDB(s.ACLs.DB(), connectorID)
		}
		nowISO := isoStringNow()
		_, _ = db.Exec(state.Rebind(`INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)`), connectorID, nowISO, "connector access granted")
		s.audit(r, "connector.grant", connectorID, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) >= 2 && parts[1] == "heartbeat" {
		switch r.Method {
		case http.MethodPatch:
			var req struct {
				LastPolicyVersion int `json:"last_policy_version"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			nowUnix := time.Now().UTC().Unix()
			nowISO := isoStringNow()
			_, _ = db.Exec(state.Rebind(`UPDATE connectors SET last_seen = ?, last_seen_at = ?, last_policy_version = ? WHERE id = ?`), nowUnix, nowISO, req.LastPolicyVersion, connectorID)
			var currentVersion int
			_ = db.QueryRow(state.Rebind(`SELECT version FROM connector_policy_versions WHERE connector_id = ?`), connectorID).Scan(&currentVersion)
			updateAvailable := req.LastPolicyVersion < currentVersion
			if req.LastPolicyVersion != currentVersion {
				msg := fmt.Sprintf("policy version mismatch: connector=%d controller=%d", req.LastPolicyVersion, currentVersion)
				_, _ = db.Exec(state.Rebind(`INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)`), connectorID, nowISO, msg)
				if updateAvailable && s.ACLNotify != nil {
					s.ACLNotify.NotifyPolicyChange()
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"update_available": updateAvailable,
				"current_version":  currentVersion,
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	http.Error(w, "unknown subresource", http.StatusNotFound)
}
