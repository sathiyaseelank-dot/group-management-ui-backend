package state

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TokenRecord struct {
	Hash        string
	ExpiresAt   time.Time
	Used        bool
	ConnectorID string
}

type TokenStore struct {
	mu     sync.Mutex
	tokens map[string]*TokenRecord
	ttl    time.Duration
	path   string
	db     *sql.DB
}

func NewTokenStore(ttl time.Duration, path string) *TokenStore {
	store := &TokenStore{
		tokens: make(map[string]*TokenRecord),
		ttl:    ttl,
		path:   path,
	}
	_ = store.load()
	return store
}

func NewTokenStoreWithDB(ttl time.Duration, db *sql.DB) *TokenStore {
	store := &TokenStore{
		tokens: make(map[string]*TokenRecord),
		ttl:    ttl,
		db:     db,
	}
	_ = store.load()
	return store
}

func (s *TokenStore) CreateToken() (string, time.Time, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(raw)
	hash := hashToken(token)
	expires := time.Time{}
	if s.ttl > 0 {
		expires = time.Now().Add(s.ttl)
	} else {
		expires = time.Time{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[hash] = &TokenRecord{
		Hash:      hash,
		ExpiresAt: expires,
		Used:      false,
	}
	if err := s.saveLocked(); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *TokenStore) ConsumeToken(token, connectorID string) error {
	if token == "" {
		return errors.New("missing token")
	}
	if connectorID == "" {
		return errors.New("missing connector id")
	}
	hash := hashToken(token)

	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.tokens[hash]
	if !ok {
		return errors.New("invalid token")
	}
	if !rec.ExpiresAt.IsZero() && time.Now().After(rec.ExpiresAt) {
		return errors.New("token expired")
	}
	if rec.Used {
		if rec.ConnectorID != connectorID {
			return errors.New("token already used")
		}
		return nil
	}
	rec.Used = true
	rec.ConnectorID = connectorID
	return s.saveLocked()
}

func (s *TokenStore) DeleteByConnectorID(connectorID string) error {
	if connectorID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, rec := range s.tokens {
		if rec.ConnectorID == connectorID {
			delete(s.tokens, hash)
		}
	}
	return s.saveLocked()
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *TokenStore) load() error {
	if s.db != nil {
		rows, err := s.db.Query(`SELECT hash, expires_at, used, connector_id FROM tokens`)
		if err != nil {
			return err
		}
		defer rows.Close()
		records := make(map[string]*TokenRecord)
		for rows.Next() {
			var hash string
			var expiresAt int64
			var used int
			var connectorID sql.NullString
			if err := rows.Scan(&hash, &expiresAt, &used, &connectorID); err != nil {
				return err
			}
			rec := &TokenRecord{
				Hash:        hash,
				ExpiresAt:   time.Unix(expiresAt, 0),
				Used:        used != 0,
				ConnectorID: connectorID.String,
			}
			records[hash] = rec
		}
		s.mu.Lock()
		s.tokens = records
		s.mu.Unlock()
		return nil
	}
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var records map[string]*TokenRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = records
	return nil
}

func (s *TokenStore) saveLocked() error {
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer func() {
			_ = tx.Rollback()
		}()
		for _, rec := range s.tokens {
			used := 0
			if rec.Used {
				used = 1
			}
			_, err := tx.Exec(
				`INSERT INTO tokens (hash, expires_at, used, connector_id)
VALUES (?, ?, ?, ?)
ON CONFLICT(hash) DO UPDATE SET expires_at=excluded.expires_at, used=excluded.used, connector_id=excluded.connector_id`,
				rec.Hash,
				rec.ExpiresAt.Unix(),
				used,
				rec.ConnectorID,
			)
			if err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.tokens, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
