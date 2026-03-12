package state

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

type IdentityProvider struct {
	ID                    string `json:"id"`
	WorkspaceID           string `json:"workspace_id"`
	ProviderType          string `json:"provider_type"` // 'google', 'github', 'oidc'
	ClientID              string `json:"client_id"`
	ClientSecretEncrypted string `json:"-"`
	RedirectURI           string `json:"redirect_uri"`
	IssuerURL             string `json:"issuer_url"`
	Enabled               bool   `json:"enabled"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}

type IdentityProviderStore struct {
	db     *sql.DB
	encKey []byte
}

func NewIdentityProviderStore(db *sql.DB, encKey []byte) *IdentityProviderStore {
	if len(encKey) != 32 {
		hash := sha256.Sum256(encKey)
		encKey = hash[:]
	}
	return &IdentityProviderStore{db: db, encKey: encKey}
}

func (s *IdentityProviderStore) Create(idp *IdentityProvider) error {
	if idp.ID == "" {
		idp.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if idp.CreatedAt == "" {
		idp.CreatedAt = now
	}
	idp.UpdatedAt = now
	enabled := 0
	if idp.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(
		Rebind(`INSERT INTO identity_providers (id, workspace_id, provider_type, client_id, client_secret_encrypted, redirect_uri, issuer_url, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		idp.ID, idp.WorkspaceID, idp.ProviderType, idp.ClientID, idp.ClientSecretEncrypted,
		idp.RedirectURI, idp.IssuerURL, enabled, idp.CreatedAt, idp.UpdatedAt,
	)
	return err
}

func (s *IdentityProviderStore) ListForWorkspace(wsID string) ([]IdentityProvider, error) {
	rows, err := s.db.Query(
		Rebind(`SELECT id, workspace_id, provider_type, client_id, client_secret_encrypted, redirect_uri, issuer_url, enabled, created_at, updated_at
			FROM identity_providers WHERE workspace_id = ? ORDER BY created_at`),
		wsID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IdentityProvider
	for rows.Next() {
		var idp IdentityProvider
		var enabled int
		if err := rows.Scan(&idp.ID, &idp.WorkspaceID, &idp.ProviderType, &idp.ClientID, &idp.ClientSecretEncrypted,
			&idp.RedirectURI, &idp.IssuerURL, &enabled, &idp.CreatedAt, &idp.UpdatedAt); err != nil {
			return nil, err
		}
		idp.Enabled = enabled != 0
		out = append(out, idp)
	}
	return out, rows.Err()
}

func (s *IdentityProviderStore) Get(id string) (*IdentityProvider, error) {
	var idp IdentityProvider
	var enabled int
	err := s.db.QueryRow(
		Rebind(`SELECT id, workspace_id, provider_type, client_id, client_secret_encrypted, redirect_uri, issuer_url, enabled, created_at, updated_at
			FROM identity_providers WHERE id = ?`),
		id,
	).Scan(&idp.ID, &idp.WorkspaceID, &idp.ProviderType, &idp.ClientID, &idp.ClientSecretEncrypted,
		&idp.RedirectURI, &idp.IssuerURL, &enabled, &idp.CreatedAt, &idp.UpdatedAt)
	if err != nil {
		return nil, err
	}
	idp.Enabled = enabled != 0
	return &idp, nil
}

func (s *IdentityProviderStore) GetEnabledByType(wsID, providerType string) (*IdentityProvider, error) {
	var idp IdentityProvider
	var enabled int
	err := s.db.QueryRow(
		Rebind(`SELECT id, workspace_id, provider_type, client_id, client_secret_encrypted, redirect_uri, issuer_url, enabled, created_at, updated_at
			FROM identity_providers WHERE workspace_id = ? AND provider_type = ? AND enabled = 1 LIMIT 1`),
		wsID, providerType,
	).Scan(&idp.ID, &idp.WorkspaceID, &idp.ProviderType, &idp.ClientID, &idp.ClientSecretEncrypted,
		&idp.RedirectURI, &idp.IssuerURL, &enabled, &idp.CreatedAt, &idp.UpdatedAt)
	if err != nil {
		return nil, err
	}
	idp.Enabled = enabled != 0
	return &idp, nil
}

func (s *IdentityProviderStore) Update(idp *IdentityProvider) error {
	idp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if idp.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(
		Rebind(`UPDATE identity_providers SET provider_type=?, client_id=?, client_secret_encrypted=?, redirect_uri=?, issuer_url=?, enabled=?, updated_at=?
			WHERE id=?`),
		idp.ProviderType, idp.ClientID, idp.ClientSecretEncrypted,
		idp.RedirectURI, idp.IssuerURL, enabled, idp.UpdatedAt, idp.ID,
	)
	return err
}

func (s *IdentityProviderStore) Delete(id string) error {
	_, err := s.db.Exec(Rebind(`DELETE FROM identity_providers WHERE id = ?`), id)
	return err
}

// EncryptSecret encrypts a plaintext client secret and sets ClientSecretEncrypted.
func (s *IdentityProviderStore) EncryptSecret(idp *IdentityProvider, plaintext string) error {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	idp.ClientSecretEncrypted = base64.StdEncoding.EncodeToString(ciphertext)
	return nil
}

// DecryptSecret decrypts the ClientSecretEncrypted and returns the plaintext.
func (s *IdentityProviderStore) DecryptSecret(idp *IdentityProvider) (string, error) {
	data, err := base64.StdEncoding.DecodeString(idp.ClientSecretEncrypted)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted secret: %w", err)
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}
	return string(plaintext), nil
}
