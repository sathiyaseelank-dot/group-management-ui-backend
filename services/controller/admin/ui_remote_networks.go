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
		rows, err := db.Query(`
			SELECT n.id, n.name, n.location,
				CAST(n.created_at AS TEXT) as created_at,
				CAST(n.updated_at AS TEXT) as updated_at,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
				(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
			FROM remote_networks n
			ORDER BY n.created_at ASC`)
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
		_, err := db.Exec(state.Rebind(`INSERT INTO remote_networks (id, name, location, tags_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`), id, req.Name, req.Location, "{}", now, now)
		if err != nil {
			log.Printf("remote network create: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	row := db.QueryRow(state.Rebind(`
		SELECT n.id, n.name, n.location,
			CAST(n.created_at AS TEXT) as created_at,
			CAST(n.updated_at AS TEXT) as updated_at,
			(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
			(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
			(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
		FROM remote_networks n
		WHERE n.id = ?`), networkID)
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
	connectorRows, _ := db.Query(state.Rebind(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version, private_ip, last_seen FROM connectors WHERE remote_network_id = ? ORDER BY name ASC`), networkID)
	connectors := []uiConnector{}
	if connectorRows != nil {
		for connectorRows.Next() {
			if conn, ok := scanUIConnector(connectorRows); ok {
				connectors = append(connectors, conn)
			}
		}
		connectorRows.Close()
	}
	resourceRows, _ := db.Query(state.Rebind(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id FROM resources WHERE remote_network_id = ? ORDER BY name ASC`), networkID)
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
