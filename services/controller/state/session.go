package state

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net"
	"strings"
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
	AbsoluteExpiresAt int64 `json:"absolute_expires_at"` // Max session lifetime
	Revoked          bool   `json:"revoked"`
	IPSubnet         string `json:"ip_subnet"`
	UAHash           string `json:"ua_hash"`
	TokenVersion         int    `json:"token_version"`
	PrevRefreshTokenHash string `json:"-"`
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
	if sess.TokenVersion == 0 {
		sess.TokenVersion = 1
	}
	_, err := s.db.Exec(
		Rebind(`INSERT INTO sessions (id, user_id, workspace_id, session_type, device_id, refresh_token_hash, ip_address, user_agent, created_at, expires_at, absolute_expires_at, revoked, ip_subnet, ua_hash, token_version, prev_refresh_token_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		sess.ID, sess.UserID, sess.WorkspaceID, sess.SessionType, sess.DeviceID,
		sess.RefreshTokenHash, sess.IPAddress, sess.UserAgent,
		sess.CreatedAt, sess.ExpiresAt, sess.AbsoluteExpiresAt, revoked, sess.IPSubnet, sess.UAHash,
		sess.TokenVersion, sess.PrevRefreshTokenHash,
	)
	return err
}

func (s *SessionStore) Get(id string) (*Session, error) {
	var sess Session
	var revoked int
	err := s.db.QueryRow(
		Rebind(`SELECT id, user_id, workspace_id, session_type, device_id, refresh_token_hash, ip_address, user_agent, created_at, expires_at, absolute_expires_at, revoked, ip_subnet, ua_hash, token_version, prev_refresh_token_hash
			FROM sessions WHERE id = ?`),
		id,
	).Scan(&sess.ID, &sess.UserID, &sess.WorkspaceID, &sess.SessionType, &sess.DeviceID,
		&sess.RefreshTokenHash, &sess.IPAddress, &sess.UserAgent,
		&sess.CreatedAt, &sess.ExpiresAt, &sess.AbsoluteExpiresAt, &revoked, &sess.IPSubnet, &sess.UAHash,
		&sess.TokenVersion, &sess.PrevRefreshTokenHash)
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
		Rebind(`SELECT id, user_id, workspace_id, session_type, device_id, refresh_token_hash, ip_address, user_agent, created_at, expires_at, absolute_expires_at, revoked, ip_subnet, ua_hash, token_version, prev_refresh_token_hash
			FROM sessions WHERE refresh_token_hash = ?`),
		hash,
	).Scan(&sess.ID, &sess.UserID, &sess.WorkspaceID, &sess.SessionType, &sess.DeviceID,
		&sess.RefreshTokenHash, &sess.IPAddress, &sess.UserAgent,
		&sess.CreatedAt, &sess.ExpiresAt, &sess.AbsoluteExpiresAt, &revoked, &sess.IPSubnet, &sess.UAHash,
		&sess.TokenVersion, &sess.PrevRefreshTokenHash)
	if err != nil {
		return nil, err
	}
	sess.Revoked = revoked != 0
	return &sess, nil
}

// GetByPrevRefreshTokenHash looks up a session by its previous refresh token hash.
// This is used for replay detection — if someone presents the old token after rotation.
func (s *SessionStore) GetByPrevRefreshTokenHash(hash string) (*Session, error) {
	var sess Session
	var revoked int
	err := s.db.QueryRow(
		Rebind(`SELECT id, user_id, workspace_id, session_type, device_id, refresh_token_hash, ip_address, user_agent, created_at, expires_at, absolute_expires_at, revoked, ip_subnet, ua_hash, token_version, prev_refresh_token_hash
			FROM sessions WHERE prev_refresh_token_hash = ? AND prev_refresh_token_hash != ''`),
		hash,
	).Scan(&sess.ID, &sess.UserID, &sess.WorkspaceID, &sess.SessionType, &sess.DeviceID,
		&sess.RefreshTokenHash, &sess.IPAddress, &sess.UserAgent,
		&sess.CreatedAt, &sess.ExpiresAt, &sess.AbsoluteExpiresAt, &revoked, &sess.IPSubnet, &sess.UAHash,
		&sess.TokenVersion, &sess.PrevRefreshTokenHash)
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
	now := time.Now().Unix()
	if now > sess.ExpiresAt {
		return false, nil
	}
	// Check absolute expiration (max session lifetime)
	if sess.AbsoluteExpiresAt > 0 && now > sess.AbsoluteExpiresAt {
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

// RotateRefreshToken atomically rotates the refresh token using compare-and-swap.
// Returns (true, nil) on success. Returns (false, nil) if the old hash no longer matches
// (replay detected). Returns (false, err) on database error.
func (s *SessionStore) RotateRefreshToken(id, oldHash, newHash string, newVersion int) (bool, error) {
	result, err := s.db.Exec(
		Rebind(`UPDATE sessions SET refresh_token_hash = ?, prev_refresh_token_hash = ?, token_version = ?
			WHERE id = ? AND refresh_token_hash = ? AND revoked = 0`),
		newHash, oldHash, newVersion, id, oldHash,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *SessionStore) UpdateDeviceID(id, deviceID string) error {
	_, err := s.db.Exec(Rebind(`UPDATE sessions SET device_id = ? WHERE id = ?`), deviceID, id)
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

// CountActiveForUser returns the count of active (non-revoked, non-expired) sessions for a user.
func (s *SessionStore) CountActiveForUser(userID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		Rebind(`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND revoked = 0 AND expires_at > ?`),
		userID, time.Now().Unix(),
	).Scan(&count)
	return count, err
}

// RevokeOldestSessionsForUser revokes the oldest sessions for a user, keeping only maxSessions active.
func (s *SessionStore) RevokeOldestSessionsForUser(userID string, maxSessions int) error {
	// Get IDs of sessions to revoke (oldest ones beyond the limit)
	rows, err := s.db.Query(
		Rebind(`SELECT id FROM sessions WHERE user_id = ? AND revoked = 0 ORDER BY created_at ASC LIMIT -1 OFFSET ?`),
		userID, maxSessions,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	var idsToRevoke []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		idsToRevoke = append(idsToRevoke, id)
	}
	// Revoke them
	for _, id := range idsToRevoke {
		if err := s.Revoke(id); err != nil {
			return err
		}
	}
	return nil
}

// ExtractIPSubnet returns the first two octets of an IP address.
func ExtractIPSubnet(remoteAddr string) string {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return host
}

// HashUserAgent returns SHA256 hex of a User-Agent string.
func HashUserAgent(ua string) string {
	h := sha256.Sum256([]byte(ua))
	return hex.EncodeToString(h[:])
}

// ValidateFingerprint checks if the current request matches the session's fingerprint.
// Returns (matches, reason).
func (sess *Session) ValidateFingerprint(remoteAddr, userAgent string) (bool, string) {
	if sess.IPSubnet == "" && sess.UAHash == "" {
		return true, "" // no fingerprint stored (legacy session)
	}
	currentSubnet := ExtractIPSubnet(remoteAddr)
	currentUAHash := HashUserAgent(userAgent)
	if sess.IPSubnet != "" && sess.IPSubnet != currentSubnet {
		return false, "ip_subnet_changed"
	}
	if sess.UAHash != "" && sess.UAHash != currentUAHash {
		return false, "user_agent_changed"
	}
	return true, ""
}
