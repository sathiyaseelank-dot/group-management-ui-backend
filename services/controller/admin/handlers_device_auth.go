package admin

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"controller/state"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

func lookupWorkspaceMemberRole(db *sql.DB, workspaceID, userID string) string {
	if db == nil || workspaceID == "" || userID == "" {
		return "member"
	}
	var role string
	if err := db.QueryRow(
		state.Rebind(`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`),
		workspaceID,
		userID,
	).Scan(&role); err != nil {
		return "member"
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return "member"
	}
	return role
}

// deviceCodeEntry is a one-time code for the device PKCE flow.
type deviceCodeEntry struct {
	email         string
	wsID          string
	wsSlug        string
	state         string
	codeChallenge string
	googleSub     string
	platform      string
	expiresAt     time.Time
}

var (
	deviceCodeMu    sync.Mutex
	deviceCodeStore = map[string]deviceCodeEntry{}
)

func storeDeviceCode(code string, entry deviceCodeEntry) {
	deviceCodeMu.Lock()
	defer deviceCodeMu.Unlock()
	deviceCodeStore[code] = entry
	// Prune expired codes
	for k, v := range deviceCodeStore {
		if time.Now().After(v.expiresAt) {
			delete(deviceCodeStore, k)
		}
	}
}

func consumeDeviceCode(code string) (deviceCodeEntry, bool) {
	deviceCodeMu.Lock()
	defer deviceCodeMu.Unlock()
	entry, ok := deviceCodeStore[code]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(deviceCodeStore, code)
		return deviceCodeEntry{}, false
	}
	delete(deviceCodeStore, code)
	return entry, true
}

// isAllowedRedirectURI returns true if the URI is safe for a native/mobile client.
// Allowed:
//   - Loopback HTTP (localhost / 127.0.0.1 / ::1) — desktop CLI clients
//   - Custom URI schemes (non-http/https) — mobile deep-links like ztna://callback
func isAllowedRedirectURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		// Custom scheme (e.g. ztna://) — safe because it is registered to the app.
		return scheme != ""
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// buildIdPOAuthConfig builds an oauth2.Config from a DB-stored IdentityProvider.
func buildIdPOAuthConfig(idp *state.IdentityProvider, secret, callbackURI string) *oauth2.Config {
	cfg := &oauth2.Config{
		ClientID:     idp.ClientID,
		ClientSecret: secret,
		RedirectURL:  callbackURI,
	}
	switch idp.ProviderType {
	case "google":
		cfg.Endpoint = google.Endpoint
		cfg.Scopes = []string{"https://www.googleapis.com/auth/userinfo.email"}
	case "github":
		cfg.Endpoint = github.Endpoint
		cfg.Scopes = []string{"user:email"}
	default:
		// Generic OIDC — caller must set IssuerURL
		cfg.Endpoint = oauth2.Endpoint{
			AuthURL:  idp.IssuerURL + "/authorize",
			TokenURL: idp.IssuerURL + "/token",
		}
		cfg.Scopes = []string{"openid", "email"}
	}
	return cfg
}

// handleDeviceAuthorize handles POST /api/device/authorize
// Input: { tenant_slug, code_challenge, code_challenge_method, redirect_uri }
// Returns: { auth_url, state }
func (s *Server) handleDeviceAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TenantSlug          string `json:"tenant_slug"`
		CodeChallenge       string `json:"code_challenge"`
		CodeChallengeMethod string `json:"code_challenge_method"`
		RedirectURI         string `json:"redirect_uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.TenantSlug == "" || req.CodeChallenge == "" || req.RedirectURI == "" {
		http.Error(w, "tenant_slug, code_challenge, and redirect_uri are required", http.StatusBadRequest)
		return
	}
	if !isAllowedRedirectURI(req.RedirectURI) {
		http.Error(w, "redirect_uri must be a loopback address or a custom URI scheme", http.StatusBadRequest)
		return
	}
	if req.CodeChallengeMethod != "" && req.CodeChallengeMethod != "S256" {
		http.Error(w, "only S256 code_challenge_method is supported", http.StatusBadRequest)
		return
	}

	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	// Resolve workspace by slug
	var ws state.Workspace
	err := db.QueryRow(
		state.Rebind(`SELECT id, name, slug FROM workspaces WHERE slug = ? LIMIT 1`),
		req.TenantSlug,
	).Scan(&ws.ID, &ws.Name, &ws.Slug)
	if err == sql.ErrNoRows {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Find an enabled IdP for this workspace
	var idpID, idpType string
	if s.IdPs != nil {
		for _, pt := range []string{"google", "github", "oidc"} {
			idp, err := s.IdPs.GetEnabledByType(ws.ID, pt)
			if err == nil && idp != nil {
				idpID = idp.ID
				idpType = idp.ProviderType
				break
			}
		}
	}

	// Fallback to env-var OAuth if no DB IdP found
	if idpID == "" {
		if s.OAuthConfig != nil {
			idpType = "google"
		} else if s.GitHubOAuthConfig != nil {
			idpType = "github"
		} else {
			http.Error(w, "no identity provider configured for this workspace", http.StatusServiceUnavailable)
			return
		}
	}

	// Generate state
	csrfState, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	deviceState := "device:" + csrfState

	// Store PKCE state in DB
	_, err = db.Exec(
		state.Rebind(`INSERT INTO device_auth_requests (state, workspace_id, code_challenge, redirect_uri, idp_id, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`),
		deviceState, ws.ID, req.CodeChallenge, req.RedirectURI, idpID,
		time.Now().Unix(), time.Now().Add(10*time.Minute).Unix(),
	)
	if err != nil {
		http.Error(w, "failed to store auth request", http.StatusInternalServerError)
		return
	}
	storeOAuthState(deviceState)

	// Build callback URI pointing to this controller
	baseURL := s.InviteBaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}
	callbackURI := baseURL + "/api/device/callback"

	// Build IdP auth URL
	var authURL string
	if s.IdPs != nil && idpID != "" {
		idp, err := s.IdPs.GetEnabledByType(ws.ID, idpType)
		if err == nil {
			secret, _ := s.IdPs.DecryptSecret(idp)
			cfg := buildIdPOAuthConfig(idp, secret, callbackURI)
			authURL = cfg.AuthCodeURL(deviceState, oauth2.AccessTypeOnline)
		}
	}

	// Fallback to env-var config.
	// Use the registered RedirectURL from the OAuth config so the redirect_uri
	// in the Google auth URL matches what is registered in the OAuth provider's console.
	if authURL == "" {
		var cfg *oauth2.Config
		if idpType == "github" && s.GitHubOAuthConfig != nil {
			cfg = &oauth2.Config{
				ClientID:     s.GitHubOAuthConfig.ClientID,
				ClientSecret: s.GitHubOAuthConfig.ClientSecret,
				RedirectURL:  s.GitHubOAuthConfig.RedirectURL,
				Scopes:       s.GitHubOAuthConfig.Scopes,
				Endpoint:     s.GitHubOAuthConfig.Endpoint,
			}
		} else {
			clientCfg := s.effectiveClientOAuthConfig()
			if clientCfg != nil {
				cfg = &oauth2.Config{
					ClientID:     clientCfg.ClientID,
					ClientSecret: clientCfg.ClientSecret,
					RedirectURL:  clientCfg.RedirectURL,
					Scopes:       clientCfg.Scopes,
					Endpoint:     clientCfg.Endpoint,
				}
			}
		}
		if cfg != nil {
			authURL = cfg.AuthCodeURL(deviceState, oauth2.AccessTypeOnline)
		}
	}

	if authURL == "" {
		http.Error(w, "failed to build auth URL", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    deviceState,
	})
}

// handleDeviceAuthStart handles POST /api/device/auth/start (v2 endpoint)
// Input: { workspace_name, code_challenge, platform }
// Returns: { auth_url }
// Differences from handleDeviceAuthorize:
//   - workspace_name looked up by slug first, then by name
//   - redirect_uri is hardcoded server-side based on platform (not sent by client)
//   - state is not returned (client doesn't need it; session_code identifies session)
func (s *Server) handleDeviceAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceName string `json:"workspace_name"`
		CodeChallenge string `json:"code_challenge"`
		Platform      string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.WorkspaceName == "" || req.CodeChallenge == "" {
		http.Error(w, "workspace_name and code_challenge are required", http.StatusBadRequest)
		return
	}
	if req.Platform == "" {
		req.Platform = "mobile"
	}

	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	// Lookup workspace: first by slug, then by name
	var ws state.Workspace
	err := db.QueryRow(
		state.Rebind(`SELECT id, name, slug FROM workspaces WHERE slug = ? LIMIT 1`),
		req.WorkspaceName,
	).Scan(&ws.ID, &ws.Name, &ws.Slug)
	if err == sql.ErrNoRows {
		err = db.QueryRow(
			state.Rebind(`SELECT id, name, slug FROM workspaces WHERE name = ? LIMIT 1`),
			req.WorkspaceName,
		).Scan(&ws.ID, &ws.Name, &ws.Slug)
	}
	if err == sql.ErrNoRows {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Server-side redirect_uri based on platform
	var redirectURI string
	switch req.Platform {
	case "mobile":
		redirectURI = "ztna://callback"
	default:
		redirectURI = "ztna://callback"
	}

	// Find IdP
	var idpID, idpType string
	if s.IdPs != nil {
		for _, pt := range []string{"google", "github", "oidc"} {
			idp, err := s.IdPs.GetEnabledByType(ws.ID, pt)
			if err == nil && idp != nil {
				idpID = idp.ID
				idpType = idp.ProviderType
				break
			}
		}
	}
	if idpID == "" {
		if s.effectiveClientOAuthConfig() != nil {
			idpType = "google"
		} else if s.GitHubOAuthConfig != nil {
			idpType = "github"
		} else {
			http.Error(w, "no identity provider configured for this workspace", http.StatusServiceUnavailable)
			return
		}
	}

	// Generate CSRF state (stored server-side; not returned to client)
	csrfState, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	deviceState := "device:" + csrfState

	_, err = db.Exec(
		state.Rebind(`INSERT INTO device_auth_requests (state, workspace_id, code_challenge, redirect_uri, idp_id, platform, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		deviceState, ws.ID, req.CodeChallenge, redirectURI, idpID, req.Platform,
		time.Now().Unix(), time.Now().Add(10*time.Minute).Unix(),
	)
	if err != nil {
		http.Error(w, "failed to store auth request", http.StatusInternalServerError)
		return
	}
	storeOAuthState(deviceState)

	baseURL := s.InviteBaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}
	callbackURI := baseURL + "/api/device/callback"

	var authURL string
	if s.IdPs != nil && idpID != "" {
		idp, err := s.IdPs.GetEnabledByType(ws.ID, idpType)
		if err == nil {
			secret, _ := s.IdPs.DecryptSecret(idp)
			cfg := buildIdPOAuthConfig(idp, secret, callbackURI)
			authURL = cfg.AuthCodeURL(deviceState, oauth2.AccessTypeOnline)
		}
	}

	if authURL == "" {
		var cfg *oauth2.Config
		if idpType == "github" && s.GitHubOAuthConfig != nil {
			cfg = &oauth2.Config{
				ClientID:     s.GitHubOAuthConfig.ClientID,
				ClientSecret: s.GitHubOAuthConfig.ClientSecret,
				RedirectURL:  s.GitHubOAuthConfig.RedirectURL,
				Scopes:       s.GitHubOAuthConfig.Scopes,
				Endpoint:     s.GitHubOAuthConfig.Endpoint,
			}
		} else {
			clientCfg := s.effectiveClientOAuthConfig()
			if clientCfg != nil {
				cfg = &oauth2.Config{
					ClientID:     clientCfg.ClientID,
					ClientSecret: clientCfg.ClientSecret,
					RedirectURL:  clientCfg.RedirectURL,
					Scopes:       clientCfg.Scopes,
					Endpoint:     clientCfg.Endpoint,
				}
			}
		}
		if cfg != nil {
			authURL = cfg.AuthCodeURL(deviceState, oauth2.AccessTypeOnline)
		}
	}

	if authURL == "" {
		http.Error(w, "failed to build auth URL", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
	})
}

// handleDeviceAuthComplete handles POST /api/device/auth/complete (v2 endpoint)
// Input: { session_code, code_verifier }
// Returns: { access_token, refresh_token, acl, expires_in }
// Identical to handleDeviceToken but uses session_code instead of code.
func (s *Server) handleDeviceAuthComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionCode  string `json:"session_code"`
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.SessionCode == "" || req.CodeVerifier == "" {
		http.Error(w, "session_code and code_verifier are required", http.StatusBadRequest)
		return
	}

	entry, ok := consumeDeviceCode(req.SessionCode)
	if !ok {
		http.Error(w, "invalid or expired session_code", http.StatusBadRequest)
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

	email := entry.email
	var userID string
	if s.Users != nil {
		u, lookupErr := s.Workspaces.GetUserByEmail(email)
		if lookupErr == nil {
			userID = u.ID
			// Store google_sub if we have it and user doesn't have one yet
			if entry.googleSub != "" {
				_, _ = db.Exec(
					state.Rebind(`UPDATE users SET google_sub = ? WHERE id = ? AND google_sub = ''`),
					entry.googleSub, userID,
				)
			}
		} else {
			newUser := state.User{
				Name:      email,
				Email:     email,
				Status:    "Active",
				Role:      "Member",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			if createErr := s.Users.CreateUser(&newUser); createErr != nil {
				log.Printf("device auth complete: failed to create user %s: %v", email, createErr)
			}
			if u2, lookupErr2 := s.Workspaces.GetUserByEmail(email); lookupErr2 == nil {
				userID = u2.ID
				if entry.googleSub != "" {
					_, _ = db.Exec(
						state.Rebind(`UPDATE users SET google_sub = ? WHERE id = ? AND google_sub = ''`),
						entry.googleSub, userID,
					)
				}
			}
		}
	}

	sessionID, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	refreshTokenRaw, err := randomHex(32)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	hashBytes := sha256.Sum256([]byte(refreshTokenRaw))
	refreshTokenHash := hex.EncodeToString(hashBytes[:])

	if s.Sessions != nil {
		sess := &state.Session{
			ID:               sessionID,
			UserID:           userID,
			WorkspaceID:      entry.wsID,
			SessionType:      "device",
			IPAddress:        r.RemoteAddr,
			UserAgent:        r.Header.Get("User-Agent"),
			RefreshTokenHash: refreshTokenHash,
			CreatedAt:        time.Now().Unix(),
			ExpiresAt:        time.Now().Add(30 * 24 * time.Hour).Unix(),
		}
		if createErr := s.Sessions.Create(sess); createErr != nil {
			log.Printf("device auth complete: failed to create session: %v", createErr)
		}
	}

	wsRole := lookupWorkspaceMemberRole(db, entry.wsID, userID)
	accessToken, err := s.signDeviceJWT(email, userID, entry.wsID, entry.wsSlug, wsRole, "", sessionID)
	if err != nil {
		http.Error(w, "failed to create access token", http.StatusInternalServerError)
		return
	}

	var aclSnapshot interface{}
	if s.ACLs != nil {
		aclSnapshot = s.ACLs.Snapshot()
	} else {
		aclSnapshot = map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshTokenRaw,
		"acl":           aclSnapshot,
		"expires_in":    900,
	})
}

// handleDeviceCallback handles GET /api/device/callback
// IdP redirects browser here after consent.
func (s *Server) handleDeviceCallback(w http.ResponseWriter, r *http.Request) {
	stateParam := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if !strings.HasPrefix(stateParam, "device:") {
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

	// Retrieve PKCE state from DB before deleting the row
	var wsID, codeChallenge, redirectURI, idpID, platform string
	var expiresAt int64
	err := db.QueryRow(
		state.Rebind(`SELECT workspace_id, code_challenge, redirect_uri, idp_id, platform, expires_at FROM device_auth_requests WHERE state = ?`),
		stateParam,
	).Scan(&wsID, &codeChallenge, &redirectURI, &idpID, &platform, &expiresAt)
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

	// Clean up the DB record
	_, _ = db.Exec(state.Rebind(`DELETE FROM device_auth_requests WHERE state = ?`), stateParam)

	// Exchange code with IdP
	baseURL := s.InviteBaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}
	callbackURI := baseURL + "/api/device/callback"

	var emailAddr string
	var fetchErr error
	if s.IdPs != nil && idpID != "" {
		idp, err := s.IdPs.Get(idpID)
		if err == nil {
			secret, _ := s.IdPs.DecryptSecret(idp)
			cfg := buildIdPOAuthConfig(idp, secret, callbackURI)
			tok, exchangeErr := cfg.Exchange(r.Context(), code)
			if exchangeErr != nil {
				log.Printf("device callback: token exchange failed: %v", exchangeErr)
				http.Error(w, "token exchange failed", http.StatusBadRequest)
				return
			}
			client := cfg.Client(r.Context(), tok)
			switch idp.ProviderType {
			case "google":
				emailAddr, fetchErr = fetchGoogleEmail(client)
			case "github":
				emailAddr, fetchErr = fetchGitHubEmail(client)
			}
			if fetchErr != nil {
				http.Error(w, "failed to get user info", http.StatusInternalServerError)
				return
			}
		}
	}

	// Fallback to env-var OAuth.
	// Use the registered RedirectURL so the token exchange URI matches the one
	// used in the authorization request (required by OAuth 2.0 spec).
	var googleSubFromIDToken string
	if emailAddr == "" {
		var cfg *oauth2.Config
		var fetchFn func(*http.Client) (string, error)
		clientCfg := s.effectiveClientOAuthConfig()
		if s.GitHubOAuthConfig != nil && idpID == "" {
			// Only use GitHub if there's no DB IdP and GitHub is configured
			cfg = &oauth2.Config{
				ClientID:     s.GitHubOAuthConfig.ClientID,
				ClientSecret: s.GitHubOAuthConfig.ClientSecret,
				RedirectURL:  s.GitHubOAuthConfig.RedirectURL,
				Scopes:       s.GitHubOAuthConfig.Scopes,
				Endpoint:     s.GitHubOAuthConfig.Endpoint,
			}
			fetchFn = fetchGitHubEmail
		} else if clientCfg != nil {
			cfg = &oauth2.Config{
				ClientID:     clientCfg.ClientID,
				ClientSecret: clientCfg.ClientSecret,
				RedirectURL:  clientCfg.RedirectURL,
				Scopes:       clientCfg.Scopes,
				Endpoint:     clientCfg.Endpoint,
			}
			fetchFn = fetchGoogleEmail
		}
		if cfg != nil && fetchFn != nil {
			tok, exchangeErr := cfg.Exchange(r.Context(), code)
			if exchangeErr != nil {
				http.Error(w, "token exchange failed", http.StatusBadRequest)
				return
			}
			// Validate Google ID token if available (client OAuth config only).
			if fetchFn != nil && clientCfg != nil && s.GitHubOAuthConfig == nil {
				if rawIDToken, ok := tok.Extra("id_token").(string); ok && rawIDToken != "" {
					if claims, idErr := validateGoogleIDToken(r.Context(), rawIDToken, cfg.ClientID); idErr != nil {
						log.Printf("device callback: id_token validation failed: %v", idErr)
						http.Error(w, "identity verification failed", http.StatusUnauthorized)
						return
					} else {
						emailAddr = strings.ToLower(claims.Email)
						googleSubFromIDToken = claims.Sub
					}
				}
			}
			if emailAddr == "" {
				client := cfg.Client(r.Context(), tok)
				emailAddr, fetchErr = fetchFn(client)
				if fetchErr != nil {
					http.Error(w, "failed to get user info", http.StatusInternalServerError)
					return
				}
			}
		}
	}

	if emailAddr == "" {
		http.Error(w, "no identity provider configured", http.StatusServiceUnavailable)
		return
	}

	email := strings.ToLower(emailAddr)

	// Look up workspace slug
	var wsSlug string
	_ = db.QueryRow(state.Rebind(`SELECT slug FROM workspaces WHERE id = ?`), wsID).Scan(&wsSlug)

	// Issue one-time controller code (60s TTL)
	ctrlCode, err := randomHex(24)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	storeDeviceCode(ctrlCode, deviceCodeEntry{
		email:         email,
		wsID:          wsID,
		wsSlug:        wsSlug,
		state:         stateParam,
		codeChallenge: codeChallenge,
		googleSub:     googleSubFromIDToken,
		platform:      platform,
		expiresAt:     time.Now().Add(60 * time.Second),
	})

	// Redirect browser to native client.
	// New v2 flow (platform=mobile): simplified deep link with session_code only.
	// Legacy flow: redirect_uri?code=X&state=Y for backward compat.
	var redirect string
	if platform == "mobile" {
		redirect = fmt.Sprintf("ztna://callback?session_code=%s", url.QueryEscape(ctrlCode))
	} else {
		redirect = fmt.Sprintf("%s?code=%s&state=%s",
			redirectURI, url.QueryEscape(ctrlCode), url.QueryEscape(stateParam))
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

// handleDeviceToken handles POST /api/device/token
// Input: { code, code_verifier, state }
// Returns: { access_token, refresh_token, acl, expires_in }
func (s *Server) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"code_verifier"`
		State        string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Code == "" || req.CodeVerifier == "" {
		http.Error(w, "code and code_verifier are required", http.StatusBadRequest)
		return
	}

	// Look up the one-time code
	entry, ok := consumeDeviceCode(req.Code)
	if !ok {
		http.Error(w, "invalid or expired code", http.StatusBadRequest)
		return
	}

	// Verify PKCE S256: SHA256(code_verifier) base64url == stored code_challenge
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
	email := entry.email
	var userID string
	if s.Users != nil {
		u, lookupErr := s.Workspaces.GetUserByEmail(email)
		if lookupErr == nil {
			userID = u.ID
		} else {
			// Create user if not exists
			newUser := state.User{
				Name:      email,
				Email:     email,
				Status:    "Active",
				Role:      "Member",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			if createErr := s.Users.CreateUser(&newUser); createErr != nil {
				log.Printf("device token: failed to create user %s: %v", email, createErr)
			}
			if u2, lookupErr2 := s.Workspaces.GetUserByEmail(email); lookupErr2 == nil {
				userID = u2.ID
			}
		}
	}

	// Create device session
	sessionID, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	refreshTokenRaw, err := randomHex(32)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	hashBytes := sha256.Sum256([]byte(refreshTokenRaw))
	refreshTokenHash := hex.EncodeToString(hashBytes[:])

	if s.Sessions != nil {
		sess := &state.Session{
			ID:               sessionID,
			UserID:           userID,
			WorkspaceID:      entry.wsID,
			SessionType:      "device",
			IPAddress:        r.RemoteAddr,
			UserAgent:        r.Header.Get("User-Agent"),
			RefreshTokenHash: refreshTokenHash,
			CreatedAt:        time.Now().Unix(),
			ExpiresAt:        time.Now().Add(30 * 24 * time.Hour).Unix(),
		}
		if createErr := s.Sessions.Create(sess); createErr != nil {
			log.Printf("device token: failed to create session: %v", createErr)
		}
	}

	// Sign device JWT (15 min)
	wsRole := lookupWorkspaceMemberRole(db, entry.wsID, userID)
	accessToken, err := s.signDeviceJWT(email, userID, entry.wsID, entry.wsSlug, wsRole, "", sessionID)
	if err != nil {
		http.Error(w, "failed to create access token", http.StatusInternalServerError)
		return
	}

	// Compile ACL snapshot for this user
	var aclSnapshot interface{}
	if s.ACLs != nil {
		aclSnapshot = s.ACLs.Snapshot()
	} else {
		aclSnapshot = map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshTokenRaw,
		"acl":           aclSnapshot,
		"expires_in":    900,
	})
}

// encodeBase64URL encodes bytes to base64 URL encoding without padding.
func encodeBase64URL(b []byte) string {
	const encTable = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	n := len(b)
	dst := make([]byte, (n+2)/3*4)
	di, si := 0, 0
	n2 := n - n%3
	for si < n2 {
		val := uint(b[si])<<16 | uint(b[si+1])<<8 | uint(b[si+2])
		dst[di+0] = encTable[val>>18&0x3F]
		dst[di+1] = encTable[val>>12&0x3F]
		dst[di+2] = encTable[val>>6&0x3F]
		dst[di+3] = encTable[val&0x3F]
		si += 3
		di += 4
	}
	rem := n - n2
	if rem == 2 {
		val := uint(b[si])<<16 | uint(b[si+1])<<8
		dst[di+0] = encTable[val>>18&0x3F]
		dst[di+1] = encTable[val>>12&0x3F]
		dst[di+2] = encTable[val>>6&0x3F]
		di += 3
	} else if rem == 1 {
		val := uint(b[si]) << 16
		dst[di+0] = encTable[val>>18&0x3F]
		dst[di+1] = encTable[val>>12&0x3F]
		di += 2
	}
	return string(dst[:di])
}

// handleDeviceRefresh handles POST /api/device/refresh
// Input: { refresh_token }
// Returns: { access_token, refresh_token, expires_in }
func (s *Server) handleDeviceRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.RefreshToken == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	if s.Sessions == nil {
		http.Error(w, "session store not configured", http.StatusServiceUnavailable)
		return
	}

	hashBytes := sha256.Sum256([]byte(req.RefreshToken))
	tokenHash := hex.EncodeToString(hashBytes[:])

	sess, err := s.Sessions.GetByRefreshTokenHash(tokenHash)
	if err != nil {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	if sess.Revoked || time.Now().Unix() > sess.ExpiresAt {
		http.Error(w, "refresh token expired or revoked", http.StatusUnauthorized)
		return
	}

	// Look up user email
	var email, wsSlug string
	db := s.db()
	if db != nil && sess.UserID != "" {
		_ = db.QueryRow(state.Rebind(`SELECT email FROM users WHERE id = ?`), sess.UserID).Scan(&email)
		_ = db.QueryRow(state.Rebind(`SELECT slug FROM workspaces WHERE id = ?`), sess.WorkspaceID).Scan(&wsSlug)
	}

	// Rotate refresh token
	newRefreshRaw, err := randomHex(32)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	newHashBytes := sha256.Sum256([]byte(newRefreshRaw))
	newHash := hex.EncodeToString(newHashBytes[:])
	if err := s.Sessions.UpdateRefreshToken(sess.ID, newHash); err != nil {
		http.Error(w, "failed to rotate refresh token", http.StatusInternalServerError)
		return
	}

	// Sign new device JWT
	wsRole := lookupWorkspaceMemberRole(db, sess.WorkspaceID, sess.UserID)
	accessToken, err := s.signDeviceJWT(email, sess.UserID, sess.WorkspaceID, wsSlug, wsRole, sess.DeviceID, sess.ID)
	if err != nil {
		http.Error(w, "failed to create access token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": newRefreshRaw,
		"expires_in":    900,
	})
}

// handleDeviceRevoke handles POST /api/device/revoke
// Input: { refresh_token }
func (s *Server) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if s.Sessions == nil {
		http.Error(w, "session store not configured", http.StatusServiceUnavailable)
		return
	}

	if req.RefreshToken != "" {
		hashBytes := sha256.Sum256([]byte(req.RefreshToken))
		tokenHash := hex.EncodeToString(hashBytes[:])
		sess, err := s.Sessions.GetByRefreshTokenHash(tokenHash)
		if err == nil {
			_ = s.Sessions.Revoke(sess.ID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
