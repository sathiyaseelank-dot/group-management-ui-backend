package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

func (s *Server) handleUIDeviceTrustedProfiles(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	wsID := workspaceIDFromContext(r.Context())

	switch r.Method {
	case http.MethodGet:
		wsClause, wsArgs := wsWhereOnly(wsID, "")
		rows, err := db.Query(state.Rebind(`SELECT id, workspace_id, name, require_firewall,
			require_disk_encryption, require_screen_lock, min_os_version, created_at, updated_at
			FROM device_trusted_profiles`+wsClause+` ORDER BY created_at ASC`), wsArgs...)
		if err != nil {
			http.Error(w, "failed to list trusted profiles", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []uiTrustedProfile{}
		for rows.Next() {
			var p uiTrustedProfile
			var fw, de, sl int
			if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Name, &fw, &de, &sl,
				&p.MinOSVersion, &p.CreatedAt, &p.UpdatedAt); err != nil {
				continue
			}
			p.RequireFirewall = fw != 0
			p.RequireDiskEncryption = de != 0
			p.RequireScreenLock = sl != 0
			out = append(out, p)
		}
		writeJSON(w, http.StatusOK, out)

	case http.MethodPost:
		var req struct {
			Name                  string `json:"name"`
			RequireFirewall       bool   `json:"requireFirewall"`
			RequireDiskEncryption bool   `json:"requireDiskEncryption"`
			RequireScreenLock     bool   `json:"requireScreenLock"`
			MinOSVersion          string `json:"minOsVersion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("tp_%d", time.Now().UTC().UnixMilli())
		now := isoStringNow()
		fw, de, sl := boolToInt(req.RequireFirewall), boolToInt(req.RequireDiskEncryption), boolToInt(req.RequireScreenLock)
		if _, err := db.Exec(state.Rebind(`INSERT INTO device_trusted_profiles
			(id, workspace_id, name, require_firewall, require_disk_encryption, require_screen_lock, min_os_version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			id, wsID, req.Name, fw, de, sl, req.MinOSVersion, now, now); err != nil {
			http.Error(w, "failed to create trusted profile", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, uiTrustedProfile{
			ID: id, WorkspaceID: wsID, Name: req.Name,
			RequireFirewall: req.RequireFirewall, RequireDiskEncryption: req.RequireDiskEncryption,
			RequireScreenLock: req.RequireScreenLock, MinOSVersion: req.MinOSVersion,
			CreatedAt: now, UpdatedAt: now,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIDeviceTrustedProfilesSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	wsID := workspaceIDFromContext(r.Context())
	profileID := strings.TrimPrefix(r.URL.Path, "/api/device-trusted-profiles/")
	profileID = strings.Trim(profileID, "/")
	if profileID == "" {
		http.Error(w, "profile id required", http.StatusBadRequest)
		return
	}
	wsClause, wsArgs := wsWhere(wsID, "")

	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var req struct {
			Name                  string `json:"name"`
			RequireFirewall       bool   `json:"requireFirewall"`
			RequireDiskEncryption bool   `json:"requireDiskEncryption"`
			RequireScreenLock     bool   `json:"requireScreenLock"`
			MinOSVersion          string `json:"minOsVersion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		now := isoStringNow()
		fw, de, sl := boolToInt(req.RequireFirewall), boolToInt(req.RequireDiskEncryption), boolToInt(req.RequireScreenLock)
		updateArgs := append([]interface{}{req.Name, fw, de, sl, req.MinOSVersion, now, profileID}, wsArgs...)
		if _, err := db.Exec(state.Rebind(`UPDATE device_trusted_profiles
			SET name=?, require_firewall=?, require_disk_encryption=?, require_screen_lock=?,
			    min_os_version=?, updated_at=?
			WHERE id=?`+wsClause),
			updateArgs...); err != nil {
			http.Error(w, "failed to update trusted profile", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodDelete:
		delArgs := append([]interface{}{profileID}, wsArgs...)
		if _, err := db.Exec(state.Rebind(`DELETE FROM device_trusted_profiles WHERE id = ?`+wsClause), delArgs...); err != nil {
			http.Error(w, "failed to delete trusted profile", http.StatusInternalServerError)
			return
		}
		// Clear trusted_profile_id on any groups referencing this profile
		_, _ = db.Exec(state.Rebind(`UPDATE user_groups SET trusted_profile_id = '' WHERE trusted_profile_id = ?`), profileID)
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIDevicePosture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	wsID := workspaceIDFromContext(r.Context())
	postures, err := state.ListDevicePosture(db, wsID)
	if err != nil {
		http.Error(w, "failed to list device posture", http.StatusInternalServerError)
		return
	}
	out := make([]uiDevicePosture, 0, len(postures))
	for _, p := range postures {
		out = append(out, uiDevicePosture{
			DeviceID: p.DeviceID, WorkspaceID: p.WorkspaceID, SPIFFEID: p.SPIFFEID,
			OSType: p.OSType, OSVersion: p.OSVersion, Hostname: p.Hostname,
			FirewallEnabled: p.FirewallEnabled, DiskEncrypted: p.DiskEncrypted,
			ScreenLockEnabled: p.ScreenLockEnabled, ClientVersion: p.ClientVersion,
			CollectedAt: p.CollectedAt, ReportedAt: p.ReportedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUIDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	wsID := workspaceIDFromContext(r.Context())

	baseQuery := `
		SELECT dp.device_id, dp.workspace_id, dp.user_id, COALESCE(u.name, ''), COALESCE(u.email, ''),
			dp.device_name, dp.device_model, dp.device_make, dp.serial_number,
			dp.spiffe_id, dp.os_type, dp.os_version, dp.hostname, dp.client_version,
			dp.firewall_enabled, dp.disk_encrypted, dp.screen_lock_enabled,
			dp.collected_at, dp.reported_at
		FROM device_posture dp
		LEFT JOIN users u ON u.id = dp.user_id`
	wsClause, wsArgs := wsWhereOnly(wsID, "dp")
	rows, err := db.Query(state.Rebind(baseQuery+wsClause+` ORDER BY dp.reported_at DESC`), wsArgs...)
	if err != nil {
		http.Error(w, "failed to list devices", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []uiDevice{}
	for rows.Next() {
		var d uiDevice
		var fw, de, sl int
		if err := rows.Scan(
			&d.DeviceID, &d.WorkspaceID, &d.UserID, &d.OwnerName, &d.OwnerEmail,
			&d.DeviceName, &d.DeviceModel, &d.DeviceMake, &d.SerialNumber,
			&d.SPIFFEID, &d.OSType, &d.OSVersion, &d.Hostname, &d.ClientVersion,
			&fw, &de, &sl, &d.CollectedAt, &d.ReportedAt,
		); err != nil {
			continue
		}
		d.FirewallEnabled = fw != 0
		d.DiskEncrypted = de != 0
		d.ScreenLockEnabled = sl != 0
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, out)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
