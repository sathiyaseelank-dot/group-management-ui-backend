package state

import (
	"database/sql"
	"time"
)

type DevicePosture struct {
	DeviceID          string `json:"device_id"`
	WorkspaceID       string `json:"workspace_id"`
	SPIFFEID          string `json:"spiffe_id"`
	OSType            string `json:"os_type"`
	OSVersion         string `json:"os_version"`
	Hostname          string `json:"hostname"`
	FirewallEnabled   bool   `json:"firewall_enabled"`
	DiskEncrypted     bool   `json:"disk_encrypted"`
	ScreenLockEnabled bool   `json:"screen_lock_enabled"`
	ClientVersion     string `json:"client_version"`
	CollectedAt       string `json:"collected_at"`
	ReportedAt        string `json:"reported_at"`
	UserID            string `json:"user_id"`
	DeviceName        string `json:"device_name"`
	DeviceModel       string `json:"device_model"`
	DeviceMake        string `json:"device_make"`
	SerialNumber      string `json:"serial_number"`
}

func UpsertDevicePosture(db *sql.DB, p DevicePosture) error {
	fw, de, sl := boolToInt(p.FirewallEnabled), boolToInt(p.DiskEncrypted), boolToInt(p.ScreenLockEnabled)
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := db.Exec(Rebind(`
		INSERT INTO device_posture
			(device_id, workspace_id, spiffe_id, os_type, os_version, hostname,
			 firewall_enabled, disk_encrypted, screen_lock_enabled, client_version, collected_at, reported_at,
			 user_id, device_name, device_model, device_make, serial_number)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id, workspace_id) DO UPDATE SET
			spiffe_id=excluded.spiffe_id, os_type=excluded.os_type, os_version=excluded.os_version,
			hostname=excluded.hostname, firewall_enabled=excluded.firewall_enabled,
			disk_encrypted=excluded.disk_encrypted, screen_lock_enabled=excluded.screen_lock_enabled,
			client_version=excluded.client_version, collected_at=excluded.collected_at,
			reported_at=excluded.reported_at,
			user_id=CASE WHEN excluded.user_id != '' THEN excluded.user_id ELSE device_posture.user_id END,
			device_name=CASE WHEN excluded.device_name != '' THEN excluded.device_name ELSE device_posture.device_name END,
			device_model=CASE WHEN excluded.device_model != '' THEN excluded.device_model ELSE device_posture.device_model END,
			device_make=CASE WHEN excluded.device_make != '' THEN excluded.device_make ELSE device_posture.device_make END,
			serial_number=CASE WHEN excluded.serial_number != '' THEN excluded.serial_number ELSE device_posture.serial_number END`),
		p.DeviceID, p.WorkspaceID, p.SPIFFEID, p.OSType, p.OSVersion, p.Hostname,
		fw, de, sl, p.ClientVersion, p.CollectedAt, now,
		p.UserID, p.DeviceName, p.DeviceModel, p.DeviceMake, p.SerialNumber,
	)
	return err
}

func ListDevicePosture(db *sql.DB, workspaceID string) ([]DevicePosture, error) {
	wsClause, wsArgs := "", []any{}
	if workspaceID != "" {
		wsClause = " WHERE workspace_id = ?"
		wsArgs = []any{workspaceID}
	}
	rows, err := db.Query(Rebind(`SELECT device_id, workspace_id, spiffe_id, os_type, os_version,
		hostname, firewall_enabled, disk_encrypted, screen_lock_enabled, client_version,
		collected_at, reported_at, user_id, device_name, device_model, device_make, serial_number
		FROM device_posture`+wsClause+` ORDER BY reported_at DESC`), wsArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DevicePosture
	for rows.Next() {
		var p DevicePosture
		var fw, de, sl int
		if err := rows.Scan(&p.DeviceID, &p.WorkspaceID, &p.SPIFFEID, &p.OSType, &p.OSVersion,
			&p.Hostname, &fw, &de, &sl, &p.ClientVersion, &p.CollectedAt, &p.ReportedAt,
			&p.UserID, &p.DeviceName, &p.DeviceModel, &p.DeviceMake, &p.SerialNumber); err != nil {
			continue
		}
		p.FirewallEnabled = fw != 0
		p.DiskEncrypted = de != 0
		p.ScreenLockEnabled = sl != 0
		out = append(out, p)
	}
	return out, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
