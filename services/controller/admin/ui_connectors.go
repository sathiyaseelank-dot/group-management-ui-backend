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

func (s *Server) handleUIConnectors(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClause, wsArgs := wsWhereOnly(wsID, "")
		rows, err := db.Query(state.Rebind(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version, private_ip FROM connectors`+wsClause+` ORDER BY name ASC`), wsArgs...)
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
	if len(parts) == 1 {
		if r.Method == http.MethodDelete {
			if s.Reg != nil {
				s.Reg.Delete(connectorID)
			}
			if s.ACLs != nil && s.ACLs.DB() != nil {
				_ = state.DeleteConnectorFromDB(s.ACLs.DB(), connectorID)
			}
			if s.Tokens != nil {
				_ = s.Tokens.DeleteByConnectorID(connectorID)
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		row := db.QueryRow(state.Rebind(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version, private_ip FROM connectors WHERE id = ?`), connectorID)
		connector, ok := scanUIConnector(row)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"connector": nil,
				"network":   nil,
				"logs":      []uiConnectorLog{},
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
			WHERE n.id = ?`), connector.RemoteNetworkID)
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
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) >= 2 && parts[1] == "heartbeat" {
		switch r.Method {
		case http.MethodPost:
			nowUnix := time.Now().UTC().Unix()
			nowISO := isoStringNow()
			_, _ = db.Exec(state.Rebind(`UPDATE connectors SET status = ?, last_seen = ?, last_seen_at = ?, installed = 1 WHERE id = ?`), "online", nowUnix, nowISO, connectorID)
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
