package state

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserGroup struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, name, email, status, role, created_at, updated_at FROM users ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var createdAt, updatedAt string
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Status, &u.Role, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		users = append(users, u)
	}
	if users == nil {
		users = []User{}
	}
	return users, nil
}

func (s *UserStore) CreateUser(u *User) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	_, err := s.db.Exec(
		Rebind(`INSERT INTO users (id, name, email, status, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		u.ID, u.Name, u.Email, u.Status, u.Role,
		u.CreatedAt.Format(time.RFC3339), u.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *UserStore) GetUser(id string) (*User, error) {
	var u User
	var createdAt, updatedAt string
	err := s.db.QueryRow(Rebind(`SELECT id, name, email, status, role, created_at, updated_at FROM users WHERE id = ?`), id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Status, &u.Role, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &u, nil
}

func (s *UserStore) UpdateUser(u *User) error {
	u.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		Rebind(`UPDATE users SET name = ?, email = ?, status = ?, role = ?, updated_at = ? WHERE id = ?`),
		u.Name, u.Email, u.Status, u.Role, u.UpdatedAt.Format(time.RFC3339), u.ID,
	)
	return err
}

func (s *UserStore) DeleteUser(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(Rebind(`DELETE FROM user_group_members WHERE user_id = ?`), id); err != nil {
		return err
	}
	if _, err := tx.Exec(Rebind(`DELETE FROM workspace_members WHERE user_id = ?`), id); err != nil {
		return err
	}
	if _, err := tx.Exec(Rebind(`DELETE FROM users WHERE id = ?`), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *UserStore) ListGroups() ([]UserGroup, error) {
	rows, err := s.db.Query(`SELECT id, name, description, created_at, updated_at FROM user_groups ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []UserGroup
	for rows.Next() {
		var g UserGroup
		var createdAt, updatedAt string
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		g.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		groups = append(groups, g)
	}
	if groups == nil {
		groups = []UserGroup{}
	}
	return groups, nil
}

func (s *UserStore) CreateGroup(g *UserGroup) error {
	if g.ID == "" {
		g.ID = uuid.New().String()
	}
	_, err := s.db.Exec(
		Rebind(`INSERT INTO user_groups (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`),
		g.ID, g.Name, g.Description,
		g.CreatedAt.Format(time.RFC3339), g.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *UserStore) GetGroup(id string) (*UserGroup, error) {
	var g UserGroup
	var createdAt, updatedAt string
	err := s.db.QueryRow(Rebind(`SELECT id, name, description, created_at, updated_at FROM user_groups WHERE id = ?`), id).
		Scan(&g.ID, &g.Name, &g.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	g.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	g.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &g, nil
}

func (s *UserStore) UpdateGroup(g *UserGroup) error {
	_, err := s.db.Exec(
		Rebind(`UPDATE user_groups SET name = ?, description = ?, updated_at = ? WHERE id = ?`),
		g.Name, g.Description, g.UpdatedAt.Format(time.RFC3339), g.ID,
	)
	return err
}

func (s *UserStore) DeleteGroup(id string) error {
	_, err := s.db.Exec(Rebind(`DELETE FROM user_groups WHERE id = ?`), id)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(Rebind(`DELETE FROM user_group_members WHERE group_id = ?`), id)
	return nil
}

func (s *UserStore) ListGroupMembers(groupID string) ([]User, error) {
	rows, err := s.db.Query(
		Rebind(`SELECT u.id, u.name, u.email, u.status, u.role, u.created_at, u.updated_at
		FROM user_group_members m JOIN users u ON u.id = m.user_id
		WHERE m.group_id = ? ORDER BY u.name ASC`), groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var createdAt, updatedAt string
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Status, &u.Role, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		users = append(users, u)
	}
	if users == nil {
		users = []User{}
	}
	return users, nil
}

func (s *UserStore) AddUserToGroup(userID, groupID string) error {
	_, err := s.db.Exec(
		Rebind(`INSERT INTO user_group_members (user_id, group_id, joined_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`),
		userID, groupID, time.Now().UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w", err)
	}
	return nil
}

func (s *UserStore) RemoveUserFromGroup(userID, groupID string) error {
	_, err := s.db.Exec(Rebind(`DELETE FROM user_group_members WHERE user_id = ? AND group_id = ?`), userID, groupID)
	return err
}
