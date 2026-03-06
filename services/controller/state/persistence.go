package state

import (
	"database/sql"
	"time"
)

func SaveConnectorToDB(db *sql.DB, rec ConnectorRecord) error {
	_, err := db.Exec(
		`INSERT INTO connectors (id, private_ip, version, last_seen)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET private_ip=excluded.private_ip, version=excluded.version, last_seen=excluded.last_seen`,
		rec.ID, rec.PrivateIP, rec.Version, rec.LastSeen.Unix(),
	)
	return err
}

func DeleteConnectorFromDB(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM connectors WHERE id = ?`, id)
	return err
}

func LoadConnectorsFromDB(db *sql.DB, registry *Registry) error {
	rows, err := db.Query(`SELECT id, private_ip, version, last_seen FROM connectors`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, privateIP, version string
		var lastSeen int64
		if err := rows.Scan(&id, &privateIP, &version, &lastSeen); err != nil {
			continue
		}
		registry.mu.Lock()
		registry.records[id] = ConnectorRecord{
			ID:        id,
			PrivateIP: privateIP,
			Version:   version,
			LastSeen:  time.Unix(lastSeen, 0).UTC(),
		}
		registry.mu.Unlock()
	}
	return nil
}

func SaveTunnelerToDB(db *sql.DB, rec TunnelerStatusRecord) error {
	_, err := db.Exec(
		`INSERT INTO tunnelers (id, spiffe_id, connector_id, last_seen)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET spiffe_id=excluded.spiffe_id, connector_id=excluded.connector_id, last_seen=excluded.last_seen`,
		rec.ID, rec.SPIFFEID, rec.ConnectorID, rec.LastSeen.Unix(),
	)
	return err
}

func LoadTunnelersFromDB(db *sql.DB, registry *TunnelerStatusRegistry) error {
	rows, err := db.Query(`SELECT id, spiffe_id, connector_id, last_seen FROM tunnelers`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, spiffeID, connectorID string
		var lastSeen int64
		if err := rows.Scan(&id, &spiffeID, &connectorID, &lastSeen); err != nil {
			continue
		}
		registry.mu.Lock()
		registry.records[id] = TunnelerStatusRecord{
			ID:          id,
			SPIFFEID:    spiffeID,
			ConnectorID: connectorID,
			LastSeen:    time.Unix(lastSeen, 0).UTC(),
		}
		registry.mu.Unlock()
	}
	return nil
}

func SaveResourceToDB(db *sql.DB, res Resource) error {
	_, err := db.Exec(
		`INSERT INTO resources (id, name, type, address, protocol, port_from, port_to, connector_id, remote_network_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, type=excluded.type, address=excluded.address, protocol=excluded.protocol, port_from=excluded.port_from, port_to=excluded.port_to, connector_id=excluded.connector_id, remote_network_id=excluded.remote_network_id`,
		res.ID, res.Name, res.Type, res.Address, res.Protocol, res.PortFrom, res.PortTo, res.ConnectorID, res.RemoteNetworkID,
	)
	return err
}

func DeleteResourceFromDB(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM resources WHERE id = ?`, id)
	return err
}

func SaveAuthorizationToDB(db *sql.DB, auth Authorization) error {
	_, err := db.Exec(
		`INSERT INTO authorizations (resource_id, principal_spiffe, filters)
		VALUES (?, ?, ?)
		ON CONFLICT(resource_id, principal_spiffe) DO UPDATE SET filters=excluded.filters`,
		auth.ResourceID, auth.PrincipalSPIFFE, marshalFilters(auth.Filters),
	)
	return err
}

func DeleteAuthorizationFromDB(db *sql.DB, resourceID, principal string) error {
	_, err := db.Exec(`DELETE FROM authorizations WHERE resource_id = ? AND principal_spiffe = ?`, resourceID, principal)
	return err
}

func LoadACLsFromDB(db *sql.DB, store *ACLStore) error {
	// Load resources
	rows, err := db.Query(`SELECT id, name, type, address, protocol, port_from, port_to, connector_id, remote_network_id FROM resources`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var res Resource
		if err := rows.Scan(&res.ID, &res.Name, &res.Type, &res.Address, &res.Protocol, &res.PortFrom, &res.PortTo, &res.ConnectorID, &res.RemoteNetworkID); err != nil {
			continue
		}
		store.AddResource(res)
	}

	// Load authorizations
	authRows, err := db.Query(`SELECT resource_id, principal_spiffe, filters FROM authorizations`)
	if err != nil {
		return err
	}
	defer authRows.Close()
	for authRows.Next() {
		var auth Authorization
		var filtersRaw string
		if err := authRows.Scan(&auth.ResourceID, &auth.PrincipalSPIFFE, &filtersRaw); err != nil {
			continue
		}
		auth.Filters = unmarshalFilters(filtersRaw)
		store.AddAuthorization(auth)
	}
	return nil
}

func PruneAuditLogs(db *sql.DB, before time.Time) error {
	_, err := db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, before.Unix())
	return err
}
