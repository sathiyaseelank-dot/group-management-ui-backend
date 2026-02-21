package state

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
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
	Members     int       `json:"members"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type GroupMember struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) CreateUser(u *User) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	u.ID = "usr_" + randHex(6)
	u.Email = strings.TrimSpace(strings.ToLower(u.Email))
	if u.Status == "" {
		u.Status = "Active"
	}
	if u.Role == "" {
		u.Role = "Member"
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	u.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO users (id, name, email, status, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Name, u.Email, u.Status, u.Role, u.CreatedAt.Unix(), u.UpdatedAt.Unix(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (s *UserStore) ListUsers() ([]User, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	rows, err := s.db.Query(`SELECT id, name, email, status, role, created_at, updated_at FROM users ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var created, updated int64
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Status, &u.Role, &created, &updated); err != nil {
			return nil, err
		}
		u.CreatedAt = time.Unix(created, 0).UTC()
		u.UpdatedAt = time.Unix(updated, 0).UTC()
		out = append(out, u)
	}
	return out, nil
}

func (s *UserStore) CreateGroup(g *UserGroup) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	g.ID = "grp_" + randHex(6)
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	g.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO user_groups (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		g.ID, g.Name, g.Description, g.CreatedAt.Unix(), g.UpdatedAt.Unix(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (s *UserStore) ListGroups() ([]UserGroup, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	rows, err := s.db.Query(`
		SELECT g.id, g.name, g.description, g.created_at, g.updated_at,
		       (SELECT COUNT(1) FROM user_group_members m WHERE m.group_id = g.id) AS members
		FROM user_groups g
		ORDER BY g.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserGroup{}
	for rows.Next() {
		var g UserGroup
		var created, updated int64
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &created, &updated, &g.Members); err != nil {
			return nil, err
		}
		g.CreatedAt = time.Unix(created, 0).UTC()
		g.UpdatedAt = time.Unix(updated, 0).UTC()
		out = append(out, g)
	}
	return out, nil
}

func (s *UserStore) AddUserToGroup(userID, groupID string) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO user_group_members (user_id, group_id, added_at) VALUES (?, ?, ?)`,
		userID, groupID, time.Now().UTC().Unix(),
	)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`UPDATE user_groups SET updated_at = ? WHERE id = ?`, time.Now().UTC().Unix(), groupID)
	return nil
}

func (s *UserStore) RemoveUserFromGroup(userID, groupID string) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	_, err := s.db.Exec(`DELETE FROM user_group_members WHERE user_id = ? AND group_id = ?`, userID, groupID)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`UPDATE user_groups SET updated_at = ? WHERE id = ?`, time.Now().UTC().Unix(), groupID)
	return nil
}

func (s *UserStore) ListGroupMembers(groupID string) ([]GroupMember, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	rows, err := s.db.Query(`
		SELECT u.id, u.name, u.email
		FROM user_group_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.group_id = ?
		ORDER BY u.name ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GroupMember{}
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func randHex(n int) string {
	if n <= 0 {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
