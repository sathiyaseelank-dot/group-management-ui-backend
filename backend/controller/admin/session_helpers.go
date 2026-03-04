package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	oidclib "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"controller/state"
)

const (
	accessTokenTTL        = 5 * time.Minute
	adminRefreshTokenTTL  = 12 * time.Hour
	adminBrowserTTL       = 8 * time.Hour
	adminRefreshCookieName = "admin_refresh"
	adminBrowserCookieName = "admin_browser"
)

// oidcVerifier is the package-level OIDC verifier, set by InitOIDC.
var oidcVerifier *oidclib.IDTokenVerifier

// InitOIDC initializes the OIDC provider for Google.
func InitOIDC(clientID string) error {
	provider, err := oidclib.NewProvider(context.Background(), "https://accounts.google.com")
	if err != nil {
		return fmt.Errorf("oidc provider init: %w", err)
	}
	oidcVerifier = provider.Verifier(&oidclib.Config{ClientID: clientID})
	return nil
}

type adminClaims struct {
	jwt.RegisteredClaims
	SessionID string `json:"sid"`
}

func generateJWT(secret []byte, userID, sessionID string) (string, error) {
	now := time.Now()
	claims := adminClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
		},
		SessionID: sessionID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func validateJWT(secret []byte, tokenStr string) (userID, sessionID string, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &adminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return "", "", err
	}
	claims, ok := token.Claims.(*adminClaims)
	if !ok || !token.Valid {
		return "", "", errors.New("invalid token claims")
	}
	return claims.Subject, claims.SessionID, nil
}

func validateSessionInDB(db *sql.DB, sessionID string) (userID string, err error) {
	var sessionState string
	var expiresAt int64
	var revokedAt sql.NullInt64
	err = db.QueryRow(
		state.Rebind(`SELECT user_id, state, expires_at, revoked_at FROM sessions WHERE id = ?`),
		sessionID,
	).Scan(&userID, &sessionState, &expiresAt, &revokedAt)
	if err != nil {
		return "", err
	}
	if sessionState != "active" || revokedAt.Valid {
		return "", errors.New("session is not active")
	}
	if time.Now().Unix() > expiresAt {
		return "", errors.New("session expired")
	}
	return userID, nil
}

type refreshResult struct {
	SessionID, UserID, NewRefreshToken string
}

func rotateRefreshToken(db *sql.DB, refreshToken string, ttl time.Duration) (*refreshResult, error) {
	tokenHash := hashSHA256(refreshToken)

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var sessionID, userID, sessionState string
	var expiresAt int64
	var revokedAt sql.NullInt64
	err = tx.QueryRow(
		state.Rebind(`SELECT id, user_id, state, expires_at, revoked_at FROM sessions WHERE refresh_token_hash = ?`),
		tokenHash,
	).Scan(&sessionID, &userID, &sessionState, &expiresAt, &revokedAt)

	if errors.Is(err, sql.ErrNoRows) {
		// Token not in active sessions — check history (reuse detection).
		var histSessionID string
		histErr := tx.QueryRow(
			state.Rebind(`SELECT session_id FROM refresh_token_history WHERE token_hash = ?`),
			tokenHash,
		).Scan(&histSessionID)
		if histErr == nil {
			// Reuse detected — revoke the compromised session.
			now := time.Now().Unix()
			_, _ = tx.Exec(
				state.Rebind(`UPDATE sessions SET state = 'revoked', revoked_at = ? WHERE id = ?`),
				now, histSessionID,
			)
			_ = tx.Commit()
		}
		return nil, errors.New("refresh token not found")
	}
	if err != nil {
		return nil, err
	}

	if sessionState != "active" || revokedAt.Valid || time.Now().Unix() > expiresAt {
		return nil, errors.New("session is expired or revoked")
	}

	now := time.Now().Unix()
	// Archive old token before rotating.
	_, err = tx.Exec(
		state.Rebind(`INSERT INTO refresh_token_history (id, session_id, token_hash, revoked_at, created_at) VALUES (?, ?, ?, ?, ?)`),
		"rth_"+uuid.NewString(), sessionID, tokenHash, now, now,
	)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	newHash := hashSHA256(newRefreshToken)

	_, err = tx.Exec(
		state.Rebind(`UPDATE sessions SET refresh_token_hash = ?, expires_at = ? WHERE id = ?`),
		newHash, time.Now().Add(ttl).Unix(), sessionID,
	)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return &refreshResult{
		SessionID:       sessionID,
		UserID:          userID,
		NewRefreshToken: newRefreshToken,
	}, nil
}

func createAdminBrowserSession(db *sql.DB, userID, userAgent string) (string, error) {
	id := "abs_" + uuid.NewString()
	now := time.Now().Unix()
	_, err := db.Exec(
		state.Rebind(`INSERT INTO admin_browser_sessions (id, user_id, user_agent_hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`),
		id, userID, hashSHA256(userAgent), now, time.Now().Add(adminBrowserTTL).Unix(),
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// IsAdmin checks whether the given user has admin privileges in the DB.
// Checks the user_roles table first, then falls back to the role column.
func IsAdmin(db *sql.DB, userID, _ string) bool {
	if db == nil || userID == "" {
		return false
	}
	var c int
	_ = db.QueryRow(
		state.Rebind(`SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = 'admin'`),
		userID,
	).Scan(&c)
	if c > 0 {
		return true
	}
	c = 0
	_ = db.QueryRow(
		state.Rebind(`SELECT COUNT(*) FROM users WHERE id = ? AND LOWER(role)='admin' AND LOWER(status)='active'`),
		userID,
	).Scan(&c)
	return c > 0
}

// ensureAdminUser creates the user if they don't exist and grants admin role.
func ensureAdminUser(db *sql.DB, email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var id, statusStr string
	err := db.QueryRow(
		state.Rebind(`SELECT id, status FROM users WHERE LOWER(email) = ?`),
		email,
	).Scan(&id, &statusStr)

	if err == nil {
		if strings.ToLower(statusStr) == "inactive" {
			return "", errors.New("admin user is inactive")
		}
		_, _ = db.Exec(
			state.Rebind(`INSERT OR IGNORE INTO user_roles (user_id, role) VALUES (?, 'admin')`),
			id,
		)
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	// Create new admin user.
	id = "usr_" + uuid.NewString()
	now := time.Now().Unix()
	name := email
	if i := strings.Index(email, "@"); i > 0 {
		name = email[:i]
	}
	_, err = db.Exec(
		state.Rebind(`INSERT INTO users (id, name, email, certificate_identity, status, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', 'Admin', ?, ?)`),
		id, name, email, "identity-"+uuid.NewString(), now, now,
	)
	if err != nil {
		return "", err
	}
	_, _ = db.Exec(
		state.Rebind(`INSERT OR IGNORE INTO user_roles (user_id, role) VALUES (?, 'admin')`),
		id,
	)
	return id, nil
}

func revokeSession(db *sql.DB, sessionID string) error {
	_, err := db.Exec(
		state.Rebind(`UPDATE sessions SET state = 'revoked', revoked_at = ? WHERE id = ?`),
		time.Now().Unix(), sessionID,
	)
	return err
}
