package state

import (
	"database/sql"
	"encoding/json"
	"time"
)

func LoadACLsFromDB(db *sql.DB, store *ACLStore) error {
	if db == nil || store == nil {
		return nil
	}
	// Resources
	rows, err := db.Query(`SELECT id, type, address, remote_network_id, user_group_ids_json FROM resources`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, typ, addr, remoteNetID, groupJSON string
		if err := rows.Scan(&id, &typ, &addr, &remoteNetID, &groupJSON); err != nil {
			rows.Close()
			return err
		}
		var groups []string
		if groupJSON != "" {
			_ = json.Unmarshal([]byte(groupJSON), &groups)
		}
		_ = store.UpsertResource(Resource{
			ID:              id,
			Type:            ResourceType(typ),
			Address:         addr,
			RemoteNetworkID: remoteNetID,
			UserGroupIDs:    groups,
		})
	}
	rows.Close()

	// Authorizations
	rows, err = db.Query(`SELECT principal_spiffe, resource_id, filters_json, expires_at, description FROM authorizations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var principal, resourceID, filtersJSON, description string
		var expires sql.NullInt64
		if err := rows.Scan(&principal, &resourceID, &filtersJSON, &expires, &description); err != nil {
			rows.Close()
			return err
		}
		var filters []Filter
		if filtersJSON != "" {
			_ = json.Unmarshal([]byte(filtersJSON), &filters)
		}
		var expiresAt *time.Time
		if expires.Valid {
			t := time.Unix(expires.Int64, 0)
			expiresAt = &t
		}
		_ = store.AssignPrincipal(resourceID, principal, filters)
		_ = store.UpdateFilters(resourceID, filters)
		if expiresAt != nil || description != "" {
			store.SetAuthorizationMeta(principal, resourceID, expiresAt, description)
		}
	}
	rows.Close()
	return nil
}

func SaveResourceToDB(db *sql.DB, res Resource) error {
	if db == nil {
		return nil
	}
	groupJSON, _ := json.Marshal(res.UserGroupIDs)
	_, err := db.Exec(
		`INSERT INTO resources (id, type, address, remote_network_id, user_group_ids_json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET type=excluded.type, address=excluded.address, remote_network_id=excluded.remote_network_id, user_group_ids_json=excluded.user_group_ids_json`,
		res.ID, string(res.Type), res.Address, res.RemoteNetworkID, string(groupJSON),
	)
	return err
}

func DeleteResourceFromDB(db *sql.DB, resourceID string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`DELETE FROM resources WHERE id = ?`, resourceID)
	_, _ = db.Exec(`DELETE FROM authorizations WHERE resource_id = ?`, resourceID)
	return err
}

func SaveAuthorizationToDB(db *sql.DB, auth Authorization) error {
	if db == nil {
		return nil
	}
	filtersJSON, _ := json.Marshal(auth.Filters)
	var expires interface{}
	if auth.ExpiresAt != nil {
		expires = auth.ExpiresAt.Unix()
	} else {
		expires = nil
	}
	_, err := db.Exec(
		`INSERT INTO authorizations (principal_spiffe, resource_id, filters_json, expires_at, description)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(principal_spiffe, resource_id)
		DO UPDATE SET filters_json=excluded.filters_json, expires_at=excluded.expires_at, description=excluded.description`,
		auth.PrincipalSPIFFE, auth.ResourceID, string(filtersJSON), expires, auth.Description,
	)
	return err
}

func DeleteAuthorizationFromDB(db *sql.DB, resourceID, principalSPIFFE string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`DELETE FROM authorizations WHERE resource_id = ? AND principal_spiffe = ?`, resourceID, principalSPIFFE)
	return err
}
