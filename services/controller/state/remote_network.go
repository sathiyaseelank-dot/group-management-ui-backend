package state

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type RemoteNetwork struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Location  string            `json:"location"`
	Tags      map[string]string `json:"tags"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type RemoteNetworkStore struct {
	db *sql.DB
}

func NewRemoteNetworkStore(db *sql.DB) *RemoteNetworkStore {
	return &RemoteNetworkStore{db: db}
}

func (s *RemoteNetworkStore) ListNetworks() ([]RemoteNetwork, error) {
	rows, err := s.db.Query(`SELECT id, name, location, tags_json, created_at, updated_at FROM remote_networks ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nets []RemoteNetwork
	for rows.Next() {
		var n RemoteNetwork
		var tagsJSON, createdAt, updatedAt string
		if err := rows.Scan(&n.ID, &n.Name, &n.Location, &tagsJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &n.Tags)
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		nets = append(nets, n)
	}
	if nets == nil {
		nets = []RemoteNetwork{}
	}
	return nets, nil
}

func (s *RemoteNetworkStore) CreateNetwork(n *RemoteNetwork) error {
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	tagsJSON, _ := json.Marshal(n.Tags)
	_, err := s.db.Exec(
		`INSERT INTO remote_networks (id, name, location, tags_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		n.ID, n.Name, n.Location, string(tagsJSON),
		n.CreatedAt.Format(time.RFC3339), n.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *RemoteNetworkStore) ListNetworkConnectors(networkID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT connector_id FROM remote_network_connectors WHERE network_id = ? ORDER BY connector_id ASC`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

func (s *RemoteNetworkStore) AssignConnector(networkID, connectorID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO remote_network_connectors (network_id, connector_id) VALUES (?, ?)`,
		networkID, connectorID,
	)
	return err
}

func (s *RemoteNetworkStore) RemoveConnector(networkID, connectorID string) error {
	_, err := s.db.Exec(`DELETE FROM remote_network_connectors WHERE network_id = ? AND connector_id = ?`, networkID, connectorID)
	return err
}
