package state

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type TokenStore struct {
	mu  sync.Mutex
	db  *sql.DB
	ttl time.Duration
}

func NewTokenStoreWithDB(ttlMinutes int, db *sql.DB) *TokenStore {
	ttl := 24 * time.Hour
	if ttlMinutes > 0 {
		ttl = time.Duration(ttlMinutes) * time.Minute
	}
	return &TokenStore{db: db, ttl: ttl}
}

func (s *TokenStore) CreateToken() (string, time.Time, error) {
	return s.CreateTokenForWorkspace("")
}

func (s *TokenStore) CreateTokenForWorkspace(workspaceID string) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(buf)
	expires := time.Now().UTC().Add(s.ttl)
	if s.db != nil {
		_, err := s.db.Exec(
			Rebind(`INSERT INTO tokens (token, expires_at, workspace_id) VALUES (?, ?, ?)`),
			token, expires.Unix(), workspaceID,
		)
		if err != nil {
			return "", time.Time{}, err
		}
	}
	return token, expires, nil
}

// ConsumeToken validates and consumes a token, returning the workspace_id it belongs to.
func (s *TokenStore) ConsumeToken(token, connectorID string) error {
	_, err := s.ConsumeTokenWithWorkspace(token, connectorID)
	return err
}

// ConsumeTokenWithWorkspace validates and consumes a token, returning its workspace_id.
func (s *TokenStore) ConsumeTokenWithWorkspace(token, connectorID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return "", fmt.Errorf("token store not configured")
	}
	var expiresAt int64
	var consumed int
	var boundID sql.NullString
	var workspaceID sql.NullString
	err := s.db.QueryRow(Rebind(`SELECT expires_at, consumed, connector_id, workspace_id FROM tokens WHERE token = ?`), token).Scan(&expiresAt, &consumed, &boundID, &workspaceID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("token not found")
	}
	if err != nil {
		return "", err
	}
	if time.Now().UTC().Unix() > expiresAt {
		return "", fmt.Errorf("token expired")
	}
	// Allow re-enrollment if the token was already consumed by the same ID.
	if consumed != 0 {
		if boundID.Valid && boundID.String == connectorID {
			wsID := ""
			if workspaceID.Valid {
				wsID = workspaceID.String
			}
			return wsID, nil
		}
		return "", fmt.Errorf("token already consumed")
	}
	_, err = s.db.Exec(Rebind(`UPDATE tokens SET consumed = 1, connector_id = ? WHERE token = ?`), connectorID, token)
	if err != nil {
		return "", err
	}
	wsID := ""
	if workspaceID.Valid {
		wsID = workspaceID.String
	}
	return wsID, nil
}

func (s *TokenStore) DeleteByConnectorID(connectorID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(Rebind(`DELETE FROM tokens WHERE connector_id = ?`), connectorID)
	return err
}
