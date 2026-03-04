package state

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// Device represents a user device registered in the system.
type Device struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DeviceName   string `json:"device_name"`
	Fingerprint  string `json:"fingerprint"`
	TrustState   string `json:"trust_state"`
	FirstSeenAt  string `json:"first_seen_at"`
	LastSeenAt   string `json:"last_seen_at"`
	PublicKeyPEM string `json:"public_key_pem,omitempty"`
	CertPEM      string `json:"cert_pem,omitempty"`
	DeviceOS     string `json:"device_os"`
}

// DeviceStore manages device records in the database.
type DeviceStore struct {
	db *sql.DB
}

// NewDeviceStore creates a new DeviceStore.
func NewDeviceStore(db *sql.DB) *DeviceStore {
	return &DeviceStore{db: db}
}

// EnrollDevice upserts a device record.
// The first device per user is auto-trusted; subsequent ones start as "pending".
// Returns the device ID and trust state.
func (s *DeviceStore) EnrollDevice(userID, deviceName, fingerprint, pubKeyPEM, certPEM, deviceOS string) (deviceID, trustState string, err error) {
	if s == nil || s.db == nil {
		return "", "", errors.New("db not configured")
	}

	// Check if device already exists for this user+fingerprint pair.
	var existing Device
	var firstSeen, lastSeen int64
	scanErr := s.db.QueryRow(
		Rebind(`SELECT id, trust_state, first_seen_at, last_seen_at FROM devices WHERE user_id = ? AND device_fingerprint = ?`),
		userID, fingerprint,
	).Scan(&existing.ID, &existing.TrustState, &firstSeen, &lastSeen)

	if scanErr == nil {
		// Device already enrolled — update last_seen_at and return.
		_, _ = s.db.Exec(
			Rebind(`UPDATE devices SET last_seen_at = ? WHERE id = ?`),
			time.Now().Unix(), existing.ID,
		)
		return existing.ID, existing.TrustState, nil
	}
	if !errors.Is(scanErr, sql.ErrNoRows) {
		return "", "", scanErr
	}

	// New device — auto-trust the first device per user.
	var count int
	_ = s.db.QueryRow(
		Rebind(`SELECT COUNT(*) FROM devices WHERE user_id = ?`),
		userID,
	).Scan(&count)

	trust := "pending"
	if count == 0 {
		trust = "trusted"
	}

	id := "dev_" + randHex(8)
	now := time.Now().Unix()

	_, err = s.db.Exec(
		Rebind(`INSERT INTO devices (id, user_id, device_name, device_fingerprint, trust_state, first_seen_at, last_seen_at, public_key_pem, cert_pem, device_os) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		id, userID, deviceName, fingerprint, trust, now, now, pubKeyPEM, certPEM, deviceOS,
	)
	if err != nil {
		return "", "", err
	}
	return id, trust, nil
}

// GetDeviceByFingerprint retrieves a device by its public-key fingerprint.
func (s *DeviceStore) GetDeviceByFingerprint(fingerprint string) (*Device, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	var d Device
	var firstSeen, lastSeen int64
	err := s.db.QueryRow(
		Rebind(`SELECT id, user_id, device_name, device_fingerprint, trust_state, first_seen_at, last_seen_at, public_key_pem, cert_pem, device_os FROM devices WHERE device_fingerprint = ?`),
		fingerprint,
	).Scan(&d.ID, &d.UserID, &d.DeviceName, &d.Fingerprint, &d.TrustState, &firstSeen, &lastSeen, &d.PublicKeyPEM, &d.CertPEM, &d.DeviceOS)
	if err != nil {
		return nil, err
	}
	d.FirstSeenAt = time.Unix(firstSeen, 0).UTC().Format(time.RFC3339)
	d.LastSeenAt = time.Unix(lastSeen, 0).UTC().Format(time.RFC3339)
	return &d, nil
}

// GetDeviceByID retrieves a device by its ID.
func (s *DeviceStore) GetDeviceByID(id string) (*Device, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	var d Device
	var firstSeen, lastSeen int64
	err := s.db.QueryRow(
		Rebind(`SELECT id, user_id, device_name, device_fingerprint, trust_state, first_seen_at, last_seen_at, public_key_pem, cert_pem, device_os FROM devices WHERE id = ?`),
		id,
	).Scan(&d.ID, &d.UserID, &d.DeviceName, &d.Fingerprint, &d.TrustState, &firstSeen, &lastSeen, &d.PublicKeyPEM, &d.CertPEM, &d.DeviceOS)
	if err != nil {
		return nil, err
	}
	d.FirstSeenAt = time.Unix(firstSeen, 0).UTC().Format(time.RFC3339)
	d.LastSeenAt = time.Unix(lastSeen, 0).UTC().Format(time.RFC3339)
	return &d, nil
}

// ListDevices lists all devices for a specific user.
func (s *DeviceStore) ListDevices(userID string) ([]Device, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	rows, err := s.db.Query(
		Rebind(`SELECT id, user_id, device_name, device_fingerprint, trust_state, first_seen_at, last_seen_at, public_key_pem, cert_pem, device_os FROM devices WHERE user_id = ? ORDER BY first_seen_at DESC`),
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDevices(rows)
}

// ListAllDevices lists all devices in the system.
func (s *DeviceStore) ListAllDevices() ([]Device, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("db not configured")
	}
	rows, err := s.db.Query(`SELECT id, user_id, device_name, device_fingerprint, trust_state, first_seen_at, last_seen_at, public_key_pem, cert_pem, device_os FROM devices ORDER BY first_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDevices(rows)
}

func scanDevices(rows *sql.Rows) ([]Device, error) {
	out := []Device{}
	for rows.Next() {
		var d Device
		var firstSeen, lastSeen int64
		if err := rows.Scan(&d.ID, &d.UserID, &d.DeviceName, &d.Fingerprint, &d.TrustState, &firstSeen, &lastSeen, &d.PublicKeyPEM, &d.CertPEM, &d.DeviceOS); err != nil {
			return nil, err
		}
		d.FirstSeenAt = time.Unix(firstSeen, 0).UTC().Format(time.RFC3339)
		d.LastSeenAt = time.Unix(lastSeen, 0).UTC().Format(time.RFC3339)
		out = append(out, d)
	}
	return out, nil
}

// SetTrustState updates the trust state of a device.
func (s *DeviceStore) SetTrustState(deviceID, trustState string) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	_, err := s.db.Exec(
		Rebind(`UPDATE devices SET trust_state = ? WHERE id = ?`),
		trustState, deviceID,
	)
	return err
}

// DeleteDevice removes a device record.
func (s *DeviceStore) DeleteDevice(deviceID string) error {
	if s == nil || s.db == nil {
		return errors.New("db not configured")
	}
	_, err := s.db.Exec(Rebind(`DELETE FROM devices WHERE id = ?`), deviceID)
	return err
}

// ConsumeVerificationToken validates a one-time verification token and marks it used.
func (s *DeviceStore) ConsumeVerificationToken(rawToken string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("db not configured")
	}
	if rawToken == "" {
		return "", errors.New("missing verification token")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	tokenHash := hashVerificationToken(rawToken)
	var userID string
	var expiresAt int64
	var usedAt sql.NullInt64
	err = tx.QueryRow(
		Rebind(`SELECT user_id, expires_at, used_at FROM user_verifications WHERE token_hash = ?`),
		tokenHash,
	).Scan(&userID, &expiresAt, &usedAt)
	if err != nil {
		return "", err
	}
	if usedAt.Valid || time.Now().Unix() > expiresAt {
		return "", errors.New("invalid or expired token")
	}

	if _, err = tx.Exec(
		Rebind(`UPDATE user_verifications SET used_at = ? WHERE token_hash = ? AND used_at IS NULL`),
		time.Now().Unix(),
		tokenHash,
	); err != nil {
		return "", err
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}
	return userID, nil
}

func hashVerificationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
