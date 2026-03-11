package state

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Workspace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	TrustDomain string `json:"trust_domain"`
	CACertPEM   string `json:"ca_cert_pem,omitempty"`
	CAKeyPEM    string `json:"-"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type WorkspaceMember struct {
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	JoinedAt    string `json:"joined_at"`
}

type WorkspaceStore struct {
	db *sql.DB
}

func NewWorkspaceStore(db *sql.DB) *WorkspaceStore {
	return &WorkspaceStore{db: db}
}

func (s *WorkspaceStore) CreateWorkspace(w *Workspace) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if w.CreatedAt == "" {
		w.CreatedAt = now
	}
	if w.UpdatedAt == "" {
		w.UpdatedAt = now
	}
	if w.Status == "" {
		w.Status = "active"
	}
	_, err := s.db.Exec(
		Rebind(`INSERT INTO workspaces (id, name, slug, trust_domain, ca_cert_pem, ca_key_pem, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		w.ID, w.Name, w.Slug, w.TrustDomain, w.CACertPEM, w.CAKeyPEM, w.Status, w.CreatedAt, w.UpdatedAt,
	)
	return err
}

func (s *WorkspaceStore) GetWorkspace(id string) (*Workspace, error) {
	var w Workspace
	err := s.db.QueryRow(
		Rebind(`SELECT id, name, slug, trust_domain, ca_cert_pem, ca_key_pem, status, created_at, updated_at FROM workspaces WHERE id = ?`), id,
	).Scan(&w.ID, &w.Name, &w.Slug, &w.TrustDomain, &w.CACertPEM, &w.CAKeyPEM, &w.Status, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *WorkspaceStore) GetWorkspaceBySlug(slug string) (*Workspace, error) {
	var w Workspace
	err := s.db.QueryRow(
		Rebind(`SELECT id, name, slug, trust_domain, ca_cert_pem, ca_key_pem, status, created_at, updated_at FROM workspaces WHERE slug = ?`), slug,
	).Scan(&w.ID, &w.Name, &w.Slug, &w.TrustDomain, &w.CACertPEM, &w.CAKeyPEM, &w.Status, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *WorkspaceStore) ListWorkspacesForUser(userID string) ([]Workspace, error) {
	rows, err := s.db.Query(
		Rebind(`SELECT w.id, w.name, w.slug, w.trust_domain, w.ca_cert_pem, w.status, w.created_at, w.updated_at
			FROM workspaces w
			JOIN workspace_members m ON m.workspace_id = w.id
			WHERE m.user_id = ? AND w.status = 'active'
			ORDER BY w.name ASC`), userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workspaces []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Slug, &w.TrustDomain, &w.CACertPEM, &w.Status, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, w)
	}
	if workspaces == nil {
		workspaces = []Workspace{}
	}
	return workspaces, nil
}

func (s *WorkspaceStore) UpdateWorkspace(w *Workspace) error {
	w.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		Rebind(`UPDATE workspaces SET name = ?, updated_at = ? WHERE id = ?`),
		w.Name, w.UpdatedAt, w.ID,
	)
	return err
}

func (s *WorkspaceStore) DeleteWorkspace(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		Rebind(`UPDATE workspaces SET status = 'deleted', updated_at = ? WHERE id = ?`),
		now, id,
	)
	return err
}

func (s *WorkspaceStore) AddMember(workspaceID, userID, role string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		Rebind(`INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`),
		workspaceID, userID, role, now,
	)
	return err
}

func (s *WorkspaceStore) RemoveMember(workspaceID, userID string) error {
	_, err := s.db.Exec(
		Rebind(`DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?`),
		workspaceID, userID,
	)
	return err
}

func (s *WorkspaceStore) ListMembers(workspaceID string) ([]WorkspaceMember, error) {
	rows, err := s.db.Query(
		Rebind(`SELECT workspace_id, user_id, role, joined_at FROM workspace_members WHERE workspace_id = ? ORDER BY joined_at ASC`),
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []WorkspaceMember
	for rows.Next() {
		var m WorkspaceMember
		if err := rows.Scan(&m.WorkspaceID, &m.UserID, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if members == nil {
		members = []WorkspaceMember{}
	}
	return members, nil
}

func (s *WorkspaceStore) GetMember(workspaceID, userID string) (*WorkspaceMember, error) {
	var m WorkspaceMember
	err := s.db.QueryRow(
		Rebind(`SELECT workspace_id, user_id, role, joined_at FROM workspace_members WHERE workspace_id = ? AND user_id = ?`),
		workspaceID, userID,
	).Scan(&m.WorkspaceID, &m.UserID, &m.Role, &m.JoinedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *WorkspaceStore) UpdateMemberRole(workspaceID, userID, role string) error {
	res, err := s.db.Exec(
		Rebind(`UPDATE workspace_members SET role = ? WHERE workspace_id = ? AND user_id = ?`),
		role, workspaceID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

func (s *WorkspaceStore) GetUserByEmail(email string) (*User, error) {
	var u User
	var createdAt, updatedAt string
	err := s.db.QueryRow(
		Rebind(`SELECT id, name, email, status, role, created_at, updated_at FROM users WHERE email = ?`), email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Status, &u.Role, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
