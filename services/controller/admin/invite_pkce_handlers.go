package admin

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"controller/state"

	"golang.org/x/oauth2"
)

// ── Invite code store ──────────────────────────────────────────────────────

type inviteCodeEntry struct {
	email         string
	googleSub     string
	inviteToken   string
	wsID          string
	wsSlug        string
	wsRole        string
	codeChallenge string
	expiresAt     time.Time
}

var (
	inviteCodeMu    sync.Mutex
	inviteCodeStore = map[string]inviteCodeEntry{}
)

func storeInviteCode(code string, entry inviteCodeEntry) {
	inviteCodeMu.Lock()
	defer inviteCodeMu.Unlock()
	inviteCodeStore[code] = entry
	for k, v := range inviteCodeStore {
		if time.Now().After(v.expiresAt) {
			delete(inviteCodeStore, k)
		}
	}
}

func consumeInviteCode(code string) (inviteCodeEntry, bool) {
	inviteCodeMu.Lock()
	defer inviteCodeMu.Unlock()
	entry, ok := inviteCodeStore[code]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(inviteCodeStore, code)
		return inviteCodeEntry{}, false
	}
	delete(inviteCodeStore, code)
	return entry, true
}

// isAllowedInviteRedirectURI validates a redirect_uri for the invite PKCE flow.
// Allows loopback HTTP, custom schemes, and HTTPS (for frontend origins).
func isAllowedInviteRedirectURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "https" {
		return true
	}
	return isAllowedRedirectURI(uri)
}

// ── POST /api/invite/authorize ─────────────────────────────────────────────

// handleInviteAuthorize initiates the invite PKCE flow.
// Input: { invite_token, code_challenge, redirect_uri }
// Output: { auth_url, state }
func (s *Server) handleInviteAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		InviteToken   string `json:"invite_token"`
		CodeChallenge string `json:"code_challenge"`
		RedirectURI   string `json:"redirect_uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.InviteToken == "" || req.CodeChallenge == "" || req.RedirectURI == "" {
		http.Error(w, "invite_token, code_challenge, and redirect_uri are required", http.StatusBadRequest)
		return
	}
	if !isAllowedInviteRedirectURI(req.RedirectURI) {
		http.Error(w, "redirect_uri is not allowed", http.StatusBadRequest)
		return
	}

	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	// Validate invite token
	var invitedEmail, wsID, role string
	var expiresAt int64
	var used int
	err := db.QueryRow(
		state.Rebind(`SELECT email, workspace_id, role, expires_at, used FROM workspace_invites WHERE token = ?`),
		req.InviteToken,
	).Scan(&invitedEmail, &wsID, &role, &expiresAt, &used)
	if err == sql.ErrNoRows {
		http.Error(w, "invite token not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if used != 0 {
		http.Error(w, "invite token already used", http.StatusBadRequest)
		return
	}
	if time.Now().Unix() > expiresAt {
		http.Error(w, "invite token expired", http.StatusBadRequest)
		return
	}

	cfg := s.effectiveClientOAuthConfig()
	if cfg == nil {
		http.Error(w, "OAuth not configured", http.StatusServiceUnavailable)
		return
	}

	// Generate CSRF state
	csrfState, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	inviteState := "invitepkce:" + csrfState

	_, err = db.Exec(
		state.Rebind(`INSERT INTO invite_auth_requests (state, invite_token, code_challenge, redirect_uri, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?)`),
		inviteState, req.InviteToken, req.CodeChallenge, req.RedirectURI,
		time.Now().Unix(), time.Now().Add(10*time.Minute).Unix(),
	)
	if err != nil {
		http.Error(w, "failed to store auth request", http.StatusInternalServerError)
		return
	}
	storeOAuthState(inviteState)

	authURL := cfg.AuthCodeURL(inviteState, oauth2.AccessTypeOnline)
	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    inviteState,
	})
}

// ── GET /api/invite/callback ───────────────────────────────────────────────

// handleInviteCallback is the OAuth callback for the invite PKCE flow.
// Google redirects here after consent; no CORS needed (browser navigation).
func (s *Server) handleInviteCallback(w http.ResponseWriter, r *http.Request) {
	stateParam := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if !strings.HasPrefix(stateParam, "invitepkce:") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	if !consumeOAuthState(stateParam) {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	var inviteToken, codeChallenge, redirectURI string
	var expiresAt int64
	err := db.QueryRow(
		state.Rebind(`SELECT invite_token, code_challenge, redirect_uri, expires_at FROM invite_auth_requests WHERE state = ?`),
		stateParam,
	).Scan(&inviteToken, &codeChallenge, &redirectURI, &expiresAt)
	if err == sql.ErrNoRows {
		http.Error(w, "auth request not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if time.Now().Unix() > expiresAt {
		http.Error(w, "auth request expired", http.StatusBadRequest)
		return
	}
	_, _ = db.Exec(state.Rebind(`DELETE FROM invite_auth_requests WHERE state = ?`), stateParam)

	cfg := s.effectiveClientOAuthConfig()
	if cfg == nil {
		http.Error(w, "OAuth not configured", http.StatusServiceUnavailable)
		return
	}

	tok, exchangeErr := cfg.Exchange(r.Context(), code)
	if exchangeErr != nil {
		log.Printf("invite callback: token exchange failed: %v", exchangeErr)
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}

	// Validate Google ID token
	var emailAddr, googleSub string
	if rawIDToken, ok := tok.Extra("id_token").(string); ok && rawIDToken != "" {
		if claims, idErr := validateGoogleIDToken(r.Context(), rawIDToken, cfg.ClientID); idErr != nil {
			log.Printf("invite callback: id_token validation failed: %v", idErr)
			http.Error(w, "identity verification failed", http.StatusUnauthorized)
			return
		} else {
			emailAddr = strings.ToLower(claims.Email)
			googleSub = claims.Sub
		}
	}
	if emailAddr == "" {
		client := cfg.Client(r.Context(), tok)
		var fetchErr error
		emailAddr, fetchErr = fetchGoogleEmail(client)
		if fetchErr != nil {
			http.Error(w, "failed to get user info", http.StatusInternalServerError)
			return
		}
		emailAddr = strings.ToLower(emailAddr)
	}

	// Re-validate invite (double-check it hasn't been used concurrently)
	var invitedEmail, wsID, wsRole string
	var inviteExpiresAt int64
	var used int
	err = db.QueryRow(
		state.Rebind(`SELECT email, workspace_id, role, expires_at, used FROM workspace_invites WHERE token = ?`),
		inviteToken,
	).Scan(&invitedEmail, &wsID, &wsRole, &inviteExpiresAt, &used)
	if err != nil || used != 0 || time.Now().Unix() > inviteExpiresAt {
		http.Error(w, "invite token invalid or expired", http.StatusBadRequest)
		return
	}
	if strings.ToLower(invitedEmail) != emailAddr {
		http.Error(w, "Google account does not match invited email", http.StatusForbidden)
		return
	}

	// Get workspace slug
	var wsSlug string
	_ = db.QueryRow(state.Rebind(`SELECT slug FROM workspaces WHERE id = ?`), wsID).Scan(&wsSlug)

	// Issue one-time controller code (60s TTL)
	ctrlCode, err := randomHex(24)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	storeInviteCode(ctrlCode, inviteCodeEntry{
		email:         emailAddr,
		googleSub:     googleSub,
		inviteToken:   inviteToken,
		wsID:          wsID,
		wsSlug:        wsSlug,
		wsRole:        wsRole,
		codeChallenge: codeChallenge,
		expiresAt:     time.Now().Add(60 * time.Second),
	})

	redirect := redirectURI + "?code=" + url.QueryEscape(ctrlCode) + "&state=" + url.QueryEscape(stateParam)
	http.Redirect(w, r, redirect, http.StatusFound)
}

// ── POST /api/invite/token ─────────────────────────────────────────────────

// handleInviteToken completes the invite PKCE flow.
// Input: { code, code_verifier }
// Output: { access_token, token_type, expires_in }
func (s *Server) handleInviteToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Code == "" || req.CodeVerifier == "" {
		http.Error(w, "code and code_verifier are required", http.StatusBadRequest)
		return
	}

	entry, ok := consumeInviteCode(req.Code)
	if !ok {
		http.Error(w, "invalid or expired code", http.StatusBadRequest)
		return
	}

	// Verify PKCE S256
	if entry.codeChallenge != "" {
		h := sha256.Sum256([]byte(req.CodeVerifier))
		computed := encodeBase64URL(h[:])
		if computed != entry.codeChallenge {
			http.Error(w, "pkce verification failed", http.StatusBadRequest)
			return
		}
	}

	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	// Create/look up user
	var userID string
	if s.Users != nil {
		u, lookupErr := s.Workspaces.GetUserByEmail(entry.email)
		if lookupErr == nil {
			userID = u.ID
		} else {
			newUser := state.User{
				Name:      entry.email,
				Email:     entry.email,
				Status:    "Active",
				Role:      "Member",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			if createErr := s.Users.CreateUser(&newUser); createErr != nil {
				log.Printf("invite token: failed to create user %s: %v (may already exist)", entry.email, createErr)
			}
			if u2, lookupErr2 := s.Workspaces.GetUserByEmail(entry.email); lookupErr2 == nil {
				userID = u2.ID
			}
		}
		// Store google_sub if available
		if entry.googleSub != "" && userID != "" {
			_, _ = db.Exec(
				state.Rebind(`UPDATE users SET google_sub = ? WHERE id = ? AND google_sub = ''`),
				entry.googleSub, userID,
			)
		}
	}

	// Mark invite used and add to workspace
	_, _ = db.Exec(
		state.Rebind(`UPDATE workspace_invites SET used = 1 WHERE token = ?`),
		entry.inviteToken,
	)
	if s.Workspaces != nil && userID != "" && entry.wsID != "" {
		_ = s.Workspaces.AddMember(entry.wsID, userID, entry.wsRole)
	}

	// Create session
	sessionID, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if s.Sessions != nil {
		// Enforce concurrent session limit
		if s.MaxSessionsPerUser > 0 && userID != "" {
			if count, countErr := s.Sessions.CountActiveForUser(userID); countErr == nil && count >= s.MaxSessionsPerUser {
				_ = s.Sessions.RevokeOldestSessionsForUser(userID, s.MaxSessionsPerUser-1)
			}
		}
		now := time.Now().Unix()
		sess := &state.Session{
			ID:                sessionID,
			UserID:            userID,
			WorkspaceID:       entry.wsID,
			SessionType:       "admin",
			IPAddress:         r.RemoteAddr,
			UserAgent:         r.Header.Get("User-Agent"),
			CreatedAt:         now,
			ExpiresAt:         now + 24*60*60,
			AbsoluteExpiresAt: now + 30*24*60*60,
			IPSubnet:          state.ExtractIPSubnet(r.RemoteAddr),
			UAHash:            state.HashUserAgent(r.Header.Get("User-Agent")),
		}
		if createErr := s.Sessions.Create(sess); createErr != nil {
			log.Printf("invite token: failed to create session: %v", createErr)
		}
	}

	// Sign admin JWT (24h)
	accessToken, err := s.signAdminJWT(entry.email, userID, entry.wsID, entry.wsSlug, entry.wsRole, sessionID)
	if err != nil {
		http.Error(w, "failed to create access token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   86400,
	})
}
