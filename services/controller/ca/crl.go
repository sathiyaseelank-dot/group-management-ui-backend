package ca

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"log"
	"math/big"
	"sync"
	"time"
)

// CRL manages an in-memory certificate revocation list.
type CRL struct {
	mu      sync.RWMutex
	revoked map[string]time.Time // serial number hex -> revocation time
}

// NewCRL creates a new empty CRL.
func NewCRL() *CRL {
	return &CRL{revoked: make(map[string]time.Time)}
}

// Revoke adds a certificate serial number to the revocation list.
func (c *CRL) Revoke(serial *big.Int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revoked[serial.String()] = time.Now()
}

// IsRevoked checks if a certificate serial number is revoked.
func (c *CRL) IsRevoked(serial *big.Int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.revoked[serial.String()]
	return ok
}

// Encode generates a PEM-encoded CRL signed by the given CA.
func (c *CRL) Encode(issuerCA *CA) ([]byte, error) {
	c.mu.RLock()
	var revokedCerts []pkix.RevokedCertificate
	for serialStr, revokedAt := range c.revoked {
		serial := new(big.Int)
		serial.SetString(serialStr, 10)
		revokedCerts = append(revokedCerts, pkix.RevokedCertificate{
			SerialNumber:   serial,
			RevocationTime: revokedAt,
		})
	}
	c.mu.RUnlock()

	template := &x509.RevocationList{
		RevokedCertificateEntries: make([]x509.RevocationListEntry, len(revokedCerts)),
		Number:                    big.NewInt(time.Now().Unix()),
		ThisUpdate:                time.Now(),
		NextUpdate:                time.Now().Add(1 * time.Hour),
	}
	for i, rc := range revokedCerts {
		template.RevokedCertificateEntries[i] = x509.RevocationListEntry{
			SerialNumber:   rc.SerialNumber,
			RevocationTime: rc.RevocationTime,
		}
	}

	crlDER, err := x509.CreateRevocationList(
		rand.Reader,
		template,
		issuerCA.cert,
		issuerCA.key,
	)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crlDER}), nil
}

// Count returns the number of revoked certificates.
func (c *CRL) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.revoked)
}

// PersistRevocation writes a revocation to the database for durability across restarts.
func (c *CRL) PersistRevocation(db *sql.DB, serial *big.Int) {
	if db == nil {
		return
	}
	_, err := db.Exec(
		`INSERT INTO revoked_certificates (serial_number, revoked_at) VALUES ($1, $2) ON CONFLICT (serial_number) DO NOTHING`,
		serial.String(), time.Now().Unix(),
	)
	if err != nil {
		log.Printf("crl: failed to persist revocation serial=%s: %v", serial.String(), err)
	}
}

// LoadFromDB restores the CRL from the database on startup.
func (c *CRL) LoadFromDB(db *sql.DB) error {
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT serial_number, revoked_at FROM revoked_certificates`)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for rows.Next() {
		var serialStr string
		var revokedAtUnix int64
		if err := rows.Scan(&serialStr, &revokedAtUnix); err != nil {
			continue
		}
		c.revoked[serialStr] = time.Unix(revokedAtUnix, 0)
		count++
	}
	if count > 0 {
		log.Printf("crl: loaded %d revoked certificates from database", count)
	}
	return rows.Err()
}
