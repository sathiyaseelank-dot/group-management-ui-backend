package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type RemoteNetwork struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags"`
	Connectors int               `json:"connectors"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type RemoteNetworkStore struct {
	db *sql.DB
}

func NewRemoteNetworkStore(db *sql.DB) *RemoteNetworkStore {
	return &RemoteNetworkStore{db: db}
}

func (s *RemoteNetworkStore) CreateNetwork(n *RemoteNetwork) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	n.ID = "net_" + randHex(6)
	if n.Tags == nil {
		n.Tags = map[string]string{}
	}
	n.CreatedAt = time.Now().UTC()
	n.UpdatedAt = time.Now().UTC()
	tagsJSON, _ := json.Marshal(n.Tags)
	_, err := s.db.Exec(
		`INSERT INTO remote_networks (id, name, location, tags_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		n.ID, n.Name, n.Location, string(tagsJSON), n.CreatedAt.Unix(), n.UpdatedAt.Unix(),
	)
	return err
}

func (s *RemoteNetworkStore) ListNetworks() ([]RemoteNetwork, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	rows, err := s.db.Query(`
		SELECT r.id, r.name, r.location, r.tags_json, r.created_at, r.updated_at,
		       (SELECT COUNT(1) FROM connector_remote_networks c WHERE c.remote_network_id = r.id) AS connectors
		FROM remote_networks r
		ORDER BY r.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RemoteNetwork{}
	for rows.Next() {
		var n RemoteNetwork
		var tagsJSON string
		var created, updated int64
		if err := rows.Scan(&n.ID, &n.Name, &n.Location, &tagsJSON, &created, &updated, &n.Connectors); err != nil {
			return nil, err
		}
		n.CreatedAt = time.Unix(created, 0).UTC()
		n.UpdatedAt = time.Unix(updated, 0).UTC()
		if tagsJSON != "" {
			_ = json.Unmarshal([]byte(tagsJSON), &n.Tags)
		} else {
			n.Tags = map[string]string{}
		}
		out = append(out, n)
	}
	return out, nil
}

func (s *RemoteNetworkStore) AssignConnector(networkID, connectorID string) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO connector_remote_networks (connector_id, remote_network_id, assigned_at) VALUES (?, ?, ?)`,
		connectorID, networkID, time.Now().UTC().Unix(),
	)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`UPDATE remote_networks SET updated_at = ? WHERE id = ?`, time.Now().UTC().Unix(), networkID)
	return nil
}

func (s *RemoteNetworkStore) RemoveConnector(networkID, connectorID string) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	_, err := s.db.Exec(`DELETE FROM connector_remote_networks WHERE connector_id = ? AND remote_network_id = ?`, connectorID, networkID)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`UPDATE remote_networks SET updated_at = ? WHERE id = ?`, time.Now().UTC().Unix(), networkID)
	return nil
}

func (s *RemoteNetworkStore) ListNetworkConnectors(networkID string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	rows, err := s.db.Query(`SELECT connector_id FROM connector_remote_networks WHERE remote_network_id = ?`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
