package state

import (
	"database/sql"
	"time"
)

func LoadConnectorsFromDB(db *sql.DB, reg *Registry) error {
	if db == nil || reg == nil {
		return nil
	}
	rows, err := db.Query(`SELECT id, private_ip, version, last_seen FROM connectors`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, privateIP, version string
		var lastSeen int64
		if err := rows.Scan(&id, &privateIP, &version, &lastSeen); err != nil {
			return err
		}
		reg.Register(id, privateIP, version)
		if lastSeen > 0 {
			reg.setLastSeen(id, time.Unix(lastSeen, 0))
		}
	}
	return nil
}

func SaveConnectorToDB(db *sql.DB, rec ConnectorRecord) error {
	if db == nil {
		return nil
	}
	// Marking a connector as "installed" is driven by controller-observed heartbeats.
	// The UI reads connectors.installed to decide whether to show "Not installed".
	lastSeenAt := rec.LastSeen.UTC().Format(time.RFC3339)
	_, err := db.Exec(
		Rebind(`INSERT INTO connectors (id, private_ip, version, last_seen, last_seen_at, status, installed)
VALUES (?, ?, ?, ?, ?, 'online', 1)
ON CONFLICT(id) DO UPDATE SET private_ip=excluded.private_ip, version=excluded.version, last_seen=excluded.last_seen, last_seen_at=excluded.last_seen_at, status='online', installed=1`),
		rec.ID,
		rec.PrivateIP,
		rec.Version,
		rec.LastSeen.Unix(),
		lastSeenAt,
	)
	return err
}

func DeleteConnectorFromDB(db *sql.DB, connectorID string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(Rebind(`DELETE FROM connectors WHERE id = ?`), connectorID)
	_, _ = db.Exec(Rebind(`DELETE FROM connector_remote_networks WHERE connector_id = ?`), connectorID)
	return err
}

func LoadTunnelersFromDB(db *sql.DB, reg *TunnelerStatusRegistry) error {
	if db == nil || reg == nil {
		return nil
	}
	rows, err := db.Query(`SELECT id, spiffe_id, connector_id, last_seen FROM tunnelers`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, spiffeID, connectorID string
		var lastSeen int64
		if err := rows.Scan(&id, &spiffeID, &connectorID, &lastSeen); err != nil {
			return err
		}
		reg.Record(id, spiffeID, connectorID)
		if lastSeen > 0 {
			reg.setLastSeen(id, time.Unix(lastSeen, 0))
		}
	}
	return nil
}

func SaveTunnelerToDB(db *sql.DB, rec TunnelerRecord) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(
		Rebind(`INSERT INTO tunnelers (id, spiffe_id, connector_id, last_seen)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET spiffe_id=excluded.spiffe_id, connector_id=excluded.connector_id, last_seen=excluded.last_seen`),
		rec.ID,
		rec.SPIFFEID,
		rec.ConnectorID,
		rec.LastSeen.Unix(),
	)
	return err
}
