package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	"controller/state"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const (
	adminSessionCookieName = "admin_access"
	adminSessionTTL        = 8 * time.Hour
)

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := s.db()
	if db == nil {
		http.Error(w, "db not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}

	inviteToken, err := randomHex(32)
	if err != nil {
		http.Error(w, "failed to create invite", http.StatusInternalServerError)
		return
	}
	inviteHash := hashSHA256(inviteToken)
	now := time.Now().Unix()
	expiresAt := time.Now().Add(10 * time.Minute).Unix()

	_, err = db.Exec(
		state.Rebind(`INSERT INTO invites (id, email, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`),
		"inv_"+uuid.NewString(), req.Email, inviteHash, expiresAt, now,
	)
	if err != nil {
		http.Error(w, "failed to store invite", http.StatusInternalServerError)
		return
	}

	base := strings.TrimRight(s.InviteBaseURL, "/")
	if base == "" {
		base = "http://localhost:8081"
	}
	inviteLink := base + "/login?invite_token=" + inviteToken

	delivered := false
	if err := s.sendInviteEmail(req.Email, inviteLink); err == nil {
		delivered = true
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"invite_link": inviteLink,
		"token":       inviteToken,
		"email_sent":  delivered,
		"expires_at":  expiresAt,
	})
}

func (s *Server) handleInviteLanding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := s.db()
	if db == nil {
		http.Error(w, "db not configured", http.StatusServiceUnavailable)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	var expiresAt int64
	var usedAt sql.NullInt64
	err := db.QueryRow(
		state.Rebind(`SELECT expires_at, used_at FROM invites WHERE token_hash = ?`),
		hashSHA256(token),
	).Scan(&expiresAt, &usedAt)
	if err != nil || usedAt.Valid || time.Now().Unix() > expiresAt {
		http.Error(w, "invalid or expired invite", http.StatusUnauthorized)
		return
	}

	verifier, challenge, err := newPKCEVerifier()
	if err != nil {
		http.Error(w, "failed to generate pkce", http.StatusInternalServerError)
		return
	}
	_, err = db.Exec(
		state.Rebind(`UPDATE invites SET pkce_verifier = ? WHERE token_hash = ?`),
		verifier, hashSHA256(token),
	)
	if err != nil {
		http.Error(w, "failed to update invite", http.StatusInternalServerError)
		return
	}

	stateToken := s.signState(token)
	http.Redirect(w, r, "/oauth/google/login?state="+stateToken+"&cc="+challenge, http.StatusFound)
}

func (s *Server) handleGoogleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := s.oauthConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	stateToken := strings.TrimSpace(r.URL.Query().Get("state"))
	challenge := strings.TrimSpace(r.URL.Query().Get("cc"))
	if stateToken == "" || challenge == "" {
		http.Error(w, "missing oauth parameters", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, cfg.AuthCodeURL(stateToken,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	), http.StatusFound)
}

func (s *Server) handleGoogleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := s.db()
	if db == nil {
		http.Error(w, "db not configured", http.StatusServiceUnavailable)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	stateToken := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || stateToken == "" {
		http.Error(w, "missing oauth callback params", http.StatusBadRequest)
		return
	}

	if _, ok := s.verifyAdminState(stateToken); ok {
		// Use the admin-specific OAuth config so the redirect_uri in the exchange
		// matches the one sent in the initial authorization request.
		adminCfg, err := s.adminOAuthConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		oauthToken, err := adminCfg.Exchange(context.Background(), code)
		if err != nil {
			http.Error(w, "oauth exchange failed", http.StatusUnauthorized)
			return
		}

		var email string
		if oidcVerifier != nil {
			rawIDToken, ok := oauthToken.Extra("id_token").(string)
			if !ok {
				http.Error(w, "no id_token in oauth response", http.StatusUnauthorized)
				return
			}
			idToken, err := oidcVerifier.Verify(context.Background(), rawIDToken)
			if err != nil {
				http.Error(w, "id_token verification failed", http.StatusUnauthorized)
				return
			}
			var idClaims struct {
				Email         string `json:"email"`
				EmailVerified bool   `json:"email_verified"`
			}
			if err := idToken.Claims(&idClaims); err != nil {
				http.Error(w, "failed to parse id_token claims", http.StatusUnauthorized)
				return
			}
			if !idClaims.EmailVerified || strings.TrimSpace(idClaims.Email) == "" {
				http.Error(w, "google email not verified", http.StatusUnauthorized)
				return
			}
			email = strings.ToLower(strings.TrimSpace(idClaims.Email))
		} else {
			email, err = fetchGoogleEmail(oauthToken.AccessToken)
			if err != nil {
				http.Error(w, "failed to read google profile", http.StatusUnauthorized)
				return
			}
			email = strings.ToLower(strings.TrimSpace(email))
		}

		if !s.isAdminAllowed(db, email) {
			http.Error(w, "forbidden: admin login denied", http.StatusForbidden)
			return
		}

		userID, err := ensureAdminUser(db, email)
		if err != nil {
			http.Error(w, "failed to ensure admin user", http.StatusInternalServerError)
			return
		}

		refreshToken, err := randomHex(32)
		if err != nil {
			http.Error(w, "failed to generate refresh token", http.StatusInternalServerError)
			return
		}

		sessionID := "sess_" + uuid.NewString()
		now := time.Now().Unix()
		_, err = db.Exec(
			state.Rebind(`INSERT INTO sessions (id, user_id, issued_at, expires_at, state, refresh_token_hash) VALUES (?, ?, ?, ?, 'active', ?)`),
			sessionID, userID, now, time.Now().Add(adminRefreshTokenTTL).Unix(), hashSHA256(refreshToken),
		)
		if err != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		browserSessionID, _ := createAdminBrowserSession(db, userID, r.UserAgent())

		jwtSecret := s.JWTSecret
		if jwtSecret == "" {
			jwtSecret = s.AdminAuthToken
		}
		accessToken, err := generateJWT([]byte(jwtSecret), userID, sessionID)
		if err != nil {
			http.Error(w, "failed to generate access token", http.StatusInternalServerError)
			return
		}

		secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
		http.SetCookie(w, &http.Cookie{
			Name:     adminSessionCookieName,
			Value:    accessToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(accessTokenTTL.Seconds()),
			Secure:   secure,
			Expires:  time.Now().Add(accessTokenTTL),
		})
		http.SetCookie(w, &http.Cookie{
			Name:     adminRefreshCookieName,
			Value:    refreshToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(adminRefreshTokenTTL.Seconds()),
			Secure:   secure,
			Expires:  time.Now().Add(adminRefreshTokenTTL),
		})
		if browserSessionID != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     adminBrowserCookieName,
				Value:    browserSessionID,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   int(adminBrowserTTL.Seconds()),
				Secure:   secure,
				Expires:  time.Now().Add(adminBrowserTTL),
			})
		}

		dashboardURL := strings.TrimRight(strings.TrimSpace(s.DashboardURL), "/")
		if dashboardURL == "" {
			dashboardURL = "http://localhost:3000"
		}
		http.Redirect(w, r, dashboardURL+"/dashboard", http.StatusFound)
		return
	}

	inviteToken, ok := s.verifyState(stateToken)
	if !ok {
		http.Error(w, "invalid state", http.StatusUnauthorized)
		return
	}
	inviteHash := hashSHA256(inviteToken)

	// Load the user invite OAuth config (has the user-specific redirect URL).
	cfg, err := s.oauthConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	var inviteEmail, pkceVerifier string
	var expiresAt int64
	var usedAt sql.NullInt64
	err = db.QueryRow(
		state.Rebind(`SELECT email, pkce_verifier, expires_at, used_at FROM invites WHERE token_hash = ?`),
		inviteHash,
	).Scan(&inviteEmail, &pkceVerifier, &expiresAt, &usedAt)
	if err != nil || pkceVerifier == "" || usedAt.Valid || time.Now().Unix() > expiresAt {
		http.Error(w, "invalid or expired invite", http.StatusUnauthorized)
		return
	}

	token, err := cfg.Exchange(context.Background(), code, oauth2.SetAuthURLParam("code_verifier", pkceVerifier))
	if err != nil {
		http.Error(w, "oauth exchange failed", http.StatusUnauthorized)
		return
	}
	email, err := fetchGoogleEmail(token.AccessToken)
	if err != nil {
		http.Error(w, "failed to read google profile", http.StatusUnauthorized)
		return
	}
	if strings.ToLower(strings.TrimSpace(email)) != strings.ToLower(inviteEmail) {
		http.Error(w, "invite email mismatch", http.StatusUnauthorized)
		return
	}

	userID, err := s.ensureActiveUserByEmail(db, inviteEmail)
	if err != nil {
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	verificationToken, err := randomHex(32)
	if err != nil {
		http.Error(w, "failed to issue verification token", http.StatusInternalServerError)
		return
	}

	now := time.Now().Unix()
	_, err = db.Exec(
		state.Rebind(`UPDATE invites SET used_at = ?, pkce_verifier = NULL WHERE token_hash = ?`),
		now, inviteHash,
	)
	if err != nil {
		http.Error(w, "failed to finalize invite", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec(
		state.Rebind(`INSERT INTO user_verifications (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`),
		"uv_"+uuid.NewString(), userID, hashSHA256(verificationToken), time.Now().Add(10*time.Minute).Unix(), now,
	)
	if err != nil {
		http.Error(w, "failed to store verification", http.StatusInternalServerError)
		return
	}

	dashboardURL := strings.TrimRight(strings.TrimSpace(s.DashboardURL), "/")
	if dashboardURL == "" {
		dashboardURL = "http://localhost:3000"
	}
	redirectURL := fmt.Sprintf("%s/login?verification_token=%s", dashboardURL, url.QueryEscape(verificationToken))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Revoke DB session if JWT is present and valid.
	jwtSecret := s.JWTSecret
	if jwtSecret == "" {
		jwtSecret = s.AdminAuthToken
	}
	if jwtSecret != "" {
		if c, err := r.Cookie(adminSessionCookieName); err == nil && c.Value != "" {
			if _, sessionID, err := validateJWT([]byte(jwtSecret), c.Value); err == nil {
				if db := s.db(); db != nil {
					_ = revokeSession(db, sessionID)
				}
			}
		}
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	for _, name := range []string{adminSessionCookieName, adminRefreshCookieName, adminBrowserCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
			Secure:   secure,
			Expires:  time.Unix(0, 0),
		})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := s.db()
	if db == nil {
		http.Error(w, "db not configured", http.StatusServiceUnavailable)
		return
	}
	jwtSecret := s.JWTSecret
	if jwtSecret == "" {
		jwtSecret = s.AdminAuthToken
	}
	if jwtSecret == "" {
		http.Error(w, "JWT not configured", http.StatusServiceUnavailable)
		return
	}

	c, err := r.Cookie(adminRefreshCookieName)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		http.Error(w, "missing refresh token", http.StatusUnauthorized)
		return
	}

	result, err := rotateRefreshToken(db, c.Value, adminRefreshTokenTTL)
	if err != nil {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	accessToken, err := generateJWT([]byte(jwtSecret), result.UserID, result.SessionID)
	if err != nil {
		http.Error(w, "failed to generate access token", http.StatusInternalServerError)
		return
	}

	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(accessTokenTTL.Seconds()),
		Secure:   secure,
		Expires:  time.Now().Add(accessTokenTTL),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     adminRefreshCookieName,
		Value:    result.NewRefreshToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(adminRefreshTokenTTL.Seconds()),
		Secure:   secure,
		Expires:  time.Now().Add(adminRefreshTokenTTL),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := s.adminOAuthConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	nonce, err := randomHex(16)
	if err != nil {
		http.Error(w, "failed to initialize login", http.StatusInternalServerError)
		return
	}
	stateToken := s.signAdminState(nonce)
	http.Redirect(w, r, cfg.AuthCodeURL(stateToken), http.StatusFound)
}

func (s *Server) handleAdminGoogleCallback(w http.ResponseWriter, r *http.Request) {
	// Alias route; callback handling is shared.
	s.handleGoogleOAuthCallback(w, r)
}

func (s *Server) db() *sql.DB {
	if s.ACLs != nil {
		return s.ACLs.DB()
	}
	return state.DB
}

func (s *Server) ensureActiveUserByEmail(db *sql.DB, email string) (string, error) {
	var id, status string
	err := db.QueryRow(
		state.Rebind(`SELECT id, status FROM users WHERE email = ?`),
		email,
	).Scan(&id, &status)
	if err == nil {
		if strings.ToLower(status) == "inactive" {
			return "", errors.New("user is inactive")
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	id = "usr_" + uuid.NewString()
	now := time.Now().Unix()
	name := email
	if i := strings.Index(email, "@"); i > 0 {
		name = email[:i]
	}
	_, err = db.Exec(
		state.Rebind(`INSERT INTO users (id, name, email, certificate_identity, status, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		id, name, email, "identity-"+uuid.NewString(), "active", "Member", now, now,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Server) oauthConfig() (*oauth2.Config, error) {
	clientID := strings.TrimSpace(s.GoogleClientID)
	clientSecret := strings.TrimSpace(s.GoogleClientSecret)
	redirectURL := strings.TrimSpace(s.OAuthRedirectURL)
	if redirectURL == "" {
		redirectURL = "http://localhost:8081/oauth/google/callback"
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}, nil
}

// adminOAuthConfig returns an OAuth2 config with the admin-specific redirect URL.
// The admin redirect URL is separate from the user invite redirect URL so each can
// be independently registered in the Google Cloud Console.
func (s *Server) adminOAuthConfig() (*oauth2.Config, error) {
	cfg, err := s.oauthConfig()
	if err != nil {
		return nil, err
	}
	adminRedirectURL := strings.TrimSpace(s.AdminOAuthRedirectURL)
	if adminRedirectURL == "" {
		adminRedirectURL = "http://localhost:8081/auth/google/callback"
	}
	cfg.RedirectURL = adminRedirectURL
	return cfg, nil
}

func fetchGoogleEmail(accessToken string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("google userinfo failed: %s", strings.TrimSpace(string(body)))
	}
	var payload struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if !payload.EmailVerified || strings.TrimSpace(payload.Email) == "" {
		return "", errors.New("google account email is not verified")
	}
	return payload.Email, nil
}

func (s *Server) sendInviteEmail(to, inviteLink string) error {
	host := strings.TrimSpace(s.SMTPHost)
	port := strings.TrimSpace(s.SMTPPort)
	from := strings.TrimSpace(s.SMTPFrom)
	if host == "" || port == "" || from == "" {
		return errors.New("smtp is not configured")
	}
	addr := host + ":" + port
	subject := "Subject: Your ZTNA invite\r\n"
	body := "You have been invited to join ZTNA.\r\n\r\nOpen this link to continue:\r\n" + inviteLink + "\r\n"
	msg := []byte(subject +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"From: " + from + "\r\n" +
		"To: " + to + "\r\n\r\n" +
		body)

	var auth smtp.Auth
	if strings.TrimSpace(s.SMTPUser) != "" {
		auth = smtp.PlainAuth("", s.SMTPUser, s.SMTPPass, host)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

func (s *Server) signState(inviteToken string) string {
	mac := hmac.New(sha256.New, []byte(s.stateSecret()))
	mac.Write([]byte(inviteToken))
	sig := hex.EncodeToString(mac.Sum(nil))
	return inviteToken + "." + sig
}

func (s *Server) verifyState(stateToken string) (string, bool) {
	parts := strings.SplitN(stateToken, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	inviteToken, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(s.stateSecret()))
	mac.Write([]byte(inviteToken))
	want := hex.EncodeToString(mac.Sum(nil))
	return inviteToken, hmac.Equal([]byte(sig), []byte(want))
}

func (s *Server) stateSecret() string {
	if strings.TrimSpace(s.InternalAuthToken) != "" {
		return s.InternalAuthToken
	}
	return "ztna-invite-state"
}

func (s *Server) signAdminState(nonce string) string {
	payload := "admin:" + nonce
	mac := hmac.New(sha256.New, []byte(s.stateSecret()))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

func (s *Server) signAdminSession(email string, expiresAt int64) string {
	payload := strings.ToLower(strings.TrimSpace(email)) + "|" + strconv.FormatInt(expiresAt, 10)
	mac := hmac.New(sha256.New, []byte(s.stateSecret()))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

func (s *Server) verifyAdminSession(token string) (string, bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	payload, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(s.stateSecret()))
	mac.Write([]byte(payload))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return "", false
	}
	payloadParts := strings.SplitN(payload, "|", 2)
	if len(payloadParts) != 2 {
		return "", false
	}
	email := strings.TrimSpace(payloadParts[0])
	if email == "" {
		return "", false
	}
	expiresAt, err := strconv.ParseInt(payloadParts[1], 10, 64)
	if err != nil {
		return "", false
	}
	if time.Now().Unix() >= expiresAt {
		return "", false
	}
	return email, true
}

func (s *Server) setAdminSessionCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
		Secure:   secure,
		Expires:  time.Now().Add(ttl),
	})
}

func (s *Server) clearAdminSessionCookie(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Secure:   secure,
		Expires:  time.Unix(0, 0),
	})
}

func (s *Server) verifyAdminState(stateToken string) (string, bool) {
	parts := strings.SplitN(stateToken, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	payload, sig := parts[0], parts[1]
	if !strings.HasPrefix(payload, "admin:") {
		return "", false
	}
	mac := hmac.New(sha256.New, []byte(s.stateSecret()))
	mac.Write([]byte(payload))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return "", false
	}
	return strings.TrimPrefix(payload, "admin:"), true
}

func (s *Server) isAdminAllowed(db *sql.DB, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	allowed := strings.TrimSpace(s.AdminLoginEmails)
	if allowed == "" {
		allowed = "gyogesh1906@gmail.com"
	}
	for _, e := range strings.Split(allowed, ",") {
		if strings.ToLower(strings.TrimSpace(e)) == email {
			return true
		}
	}
	if db == nil {
		return false
	}
	var c int
	_ = db.QueryRow(
		state.Rebind(`SELECT COUNT(*) FROM users WHERE LOWER(email)=LOWER(?) AND LOWER(status)='active' AND LOWER(role)='admin'`),
		email,
	).Scan(&c)
	return c > 0
}

func newPKCEVerifier() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashSHA256(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}
