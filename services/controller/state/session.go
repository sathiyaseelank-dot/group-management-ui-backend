package state

import (
	"database/sql"
	"time"
)

type Session struct {
	ID               string `json:"id"`
	UserID           string `json:"user_id"`
	WorkspaceID      string `json:"workspace_id"`
	SessionType      string `json:"session_type"` // 'admin' | 'device'
	DeviceID         string `json:"device_id"`
	RefreshTokenHash string `json:"-"`
	IPAddress        string `json:"ip_address"`
	UserAgent        string `json:"user_agent"`
	CreatedAt        int64  `json:"created_at"`
	ExpiresAt        int64  `json:"expires_at"`
	Revoked          bool   `json:"revoked"`
}

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) Create(sess *Session) error {
	revoked := 0
	if sess.Revoked {
		revoked = 1
	}
	_, err := s.db.Exec(
		Rebind(`INSERT INTO sessions (id, user_id, workspace_id, session_type, device_id, refresh_token_hash, ip_address, user_agent, created_at, expires_at, revoked)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		sess.ID, sess.UserID, sess.WorkspaceID, sess.SessionType, sess.DeviceID,
		sess.RefreshTokenHash, sess.IPAddress, sess.UserAgent,
		sess.CreatedAt, sess.ExpiresAt, revoked,
	)
	return err
}

func (s *SessionStore) Get(id string) (*Session, error) {
	var sess Session
	var revoked int
	err := s.db.QueryRow(
		Rebind(`SELECT id, user_id, workspace_id, session_type, device_id, refresh_token_hash, ip_address, user_agent, created_at, expires_at, revoked
			FROM sessions WHERE id = ?`),
		id,
	).Scan(&sess.ID, &sess.UserID, &sess.WorkspaceID, &sess.SessionType, &sess.DeviceID,
		&sess.RefreshTokenHash, &sess.IPAddress, &sess.UserAgent,
		&sess.CreatedAt, &sess.ExpiresAt, &revoked)
	if err != nil {
		return nil, err
	}
	sess.Revoked = revoked != 0
	return &sess, nil
}

func (s *SessionStore) GetByRefreshTokenHash(hash string) (*Session, error) {
	var sess Session
	var revoked int
	err := s.db.QueryRow(
		Rebind(`SELECT id, user_id, workspace_id, session_type, device_id, refresh_token_hash, ip_address, user_agent, created_at, expires_at, revoked
			FROM sessions WHERE refresh_token_hash = ?`),
		hash,
	).Scan(&sess.ID, &sess.UserID, &sess.WorkspaceID, &sess.SessionType, &sess.DeviceID,
		&sess.RefreshTokenHash, &sess.IPAddress, &sess.UserAgent,
		&sess.CreatedAt, &sess.ExpiresAt, &revoked)
	if err != nil {
		return nil, err
	}
	sess.Revoked = revoked != 0
	return &sess, nil
}

func (s *SessionStore) IsValid(id string) (bool, error) {
	sess, err := s.Get(id)
	if err != nil {
		return false, err
	}
	if sess.Revoked {
		return false, nil
	}
	if time.Now().Unix() > sess.ExpiresAt {
		return false, nil
	}
	return true, nil
}

func (s *SessionStore) Revoke(id string) error {
	_, err := s.db.Exec(Rebind(`UPDATE sessions SET revoked = 1 WHERE id = ?`), id)
	return err
}

func (s *SessionStore) RevokeAllForUser(userID string) error {
	_, err := s.db.Exec(Rebind(`UPDATE sessions SET revoked = 1 WHERE user_id = ?`), userID)
	return err
}

func (s *SessionStore) UpdateRefreshToken(id, newHash string) error {
	_, err := s.db.Exec(Rebind(`UPDATE sessions SET refresh_token_hash = ? WHERE id = ?`), newHash, id)
	return err
}

func (s *SessionStore) CleanExpired() error {
	_, err := s.db.Exec(Rebind(`DELETE FROM sessions WHERE expires_at < ?`), time.Now().Unix())
	return err
}

func (s *SessionStore) ListForWorkspace(wsID string) ([]Session, error) {
	rows, err := s.db.Query(
		Rebind(`SELECT id, user_id, workspace_id, session_type, device_id, ip_address, user_agent, created_at, expires_at, revoked
			FROM sessions WHERE workspace_id = ? ORDER BY created_at DESC`),
		wsID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		var revoked int
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.WorkspaceID, &sess.SessionType, &sess.DeviceID,
			&sess.IPAddress, &sess.UserAgent, &sess.CreatedAt, &sess.ExpiresAt, &revoked); err != nil {
			return nil, err
		}
		sess.Revoked = revoked != 0
		out = append(out, sess)
	}
	return out, rows.Err()
}
