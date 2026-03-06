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
			`INSERT INTO tokens (token, expires_at) VALUES (?, ?)`,
			token, expires.Unix(),
		)
		if err != nil {
			return "", time.Time{}, err
		}
	}
	return token, expires, nil
}

func (s *TokenStore) ConsumeToken(token, connectorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("token store not configured")
	}
	var expiresAt int64
	var consumed int
	err := s.db.QueryRow(`SELECT expires_at, consumed FROM tokens WHERE token = ?`, token).Scan(&expiresAt, &consumed)
	if err == sql.ErrNoRows {
		return fmt.Errorf("token not found")
	}
	if err != nil {
		return err
	}
	if consumed != 0 {
		return fmt.Errorf("token already consumed")
	}
	if time.Now().UTC().Unix() > expiresAt {
		return fmt.Errorf("token expired")
	}
	_, err = s.db.Exec(`UPDATE tokens SET consumed = 1, connector_id = ? WHERE token = ?`, connectorID, token)
	return err
}

func (s *TokenStore) DeleteByConnectorID(connectorID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM tokens WHERE connector_id = ?`, connectorID)
	return err
}
