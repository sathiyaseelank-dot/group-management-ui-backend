package admin

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"controller/state"

	"golang.org/x/oauth2"
)

// oauthStateEntry holds a CSRF state value with its expiry.
type oauthStateEntry struct {
	value     string
	expiresAt time.Time
}

// oauthReturnToEntry holds a return_to URL with its expiry.
type oauthReturnToEntry struct {
	url       string
	expiresAt time.Time
}

type oauthWorkspaceEntry struct {
	slug      string
	expiresAt time.Time
}

var (
	oauthStateMu        sync.Mutex
	oauthStateStore     = map[string]oauthStateEntry{}
	oauthReturnToStore  = map[string]oauthReturnToEntry{}
	oauthWorkspaceStore = map[string]oauthWorkspaceEntry{}
)

// storeOAuthReturnTo associates a frontend origin URL with an OAuth state key.
func storeOAuthReturnTo(state, returnTo string) {
	oauthStateMu.Lock()
	defer oauthStateMu.Unlock()
	oauthReturnToStore[state] = oauthReturnToEntry{
		url:       returnTo,
		expiresAt: time.Now().Add(10 * time.Minute),
	}
	// Prune expired entries
	for k, v := range oauthReturnToStore {
		if time.Now().After(v.expiresAt) {
			delete(oauthReturnToStore, k)
		}
	}
}

// consumeOAuthReturnTo retrieves and removes the return_to URL for the given state.
func consumeOAuthReturnTo(state string) string {
	oauthStateMu.Lock()
	defer oauthStateMu.Unlock()
	entry, ok := oauthReturnToStore[state]
	delete(oauthReturnToStore, state)
	if !ok || time.Now().After(entry.expiresAt) {
		return ""
	}
	return entry.url
}

func storeOAuthWorkspaceSlug(state, slug string) {
	oauthStateMu.Lock()
	defer oauthStateMu.Unlock()
	oauthWorkspaceStore[state] = oauthWorkspaceEntry{
		slug:      slug,
		expiresAt: time.Now().Add(10 * time.Minute),
	}
	for k, v := range oauthWorkspaceStore {
		if time.Now().After(v.expiresAt) {
			delete(oauthWorkspaceStore, k)
		}
	}
}

func consumeOAuthWorkspaceSlug(state string) string {
	oauthStateMu.Lock()
	defer oauthStateMu.Unlock()
	entry, ok := oauthWorkspaceStore[state]
	delete(oauthWorkspaceStore, state)
	if !ok || time.Now().After(entry.expiresAt) {
		return ""
	}
	return entry.slug
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func storeOAuthState(state string) {
	oauthStateMu.Lock()
	defer oauthStateMu.Unlock()
	oauthStateStore[state] = oauthStateEntry{value: state, expiresAt: time.Now().Add(10 * time.Minute)}
	// Prune expired states
	for k, v := range oauthStateStore {
		if time.Now().After(v.expiresAt) {
			delete(oauthStateStore, k)
		}
	}
}

func consumeOAuthState(state string) bool {
	oauthStateMu.Lock()
	defer oauthStateMu.Unlock()
	entry, ok := oauthStateStore[state]
	if !ok || time.Now().After(entry.expiresAt) {
		return false
	}
	delete(oauthStateStore, state)
	return true
}

// handleProviderLogin returns a handler that redirects the user to the OAuth provider's consent screen.
// If ?flow=signup is present, the CSRF state encodes signup data so it survives the redirect round-trip.
// State format for signup: "signup:<csrf>:<url-encoded ws_name>:<ws_slug>:<signup_id>"
func (s *Server) handleProviderLogin(provider string, cfg *oauth2.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg == nil {
			http.Error(w, fmt.Sprintf("%s OAuth not configured", provider), http.StatusServiceUnavailable)
			return
		}
		csrfState, err := randomHex(16)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Embed signup data in the state so it survives the OAuth redirect round-trip
		// (sessionStorage is origin-scoped and may not be available after cross-origin redirect).
		if r.URL.Query().Get("flow") == "signup" {
			wsName := r.URL.Query().Get("ws_name")
			wsSlug := r.URL.Query().Get("ws_slug")
			signupID := r.URL.Query().Get("signup_id")
			csrfState = fmt.Sprintf(
				"signup:%s:%s:%s:%s",
				csrfState,
				url.QueryEscape(wsName),
				url.QueryEscape(wsSlug),
				url.QueryEscape(signupID),
			)
		}
		// Capture the frontend origin so the callback redirects to the correct host
		// (e.g. LAN IP instead of localhost).
		if returnTo := r.URL.Query().Get("return_to"); returnTo != "" {
			storeOAuthReturnTo(csrfState, returnTo)
		}
		if wsSlug := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("workspace_slug"))); wsSlug != "" {
			storeOAuthWorkspaceSlug(csrfState, wsSlug)
		}
		storeOAuthState(csrfState)
		authURL := cfg.AuthCodeURL(csrfState, oauth2.AccessTypeOnline)
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

// handleProviderCallback returns a handler for the OAuth provider callback.
// fetchEmail is a provider-specific function to retrieve the user's email.
func (s *Server) handleProviderCallback(provider string, cfg *oauth2.Config, fetchEmail func(*http.Client) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg == nil {
			http.Error(w, fmt.Sprintf("%s OAuth not configured", provider), http.StatusServiceUnavailable)
			return
		}
		stateParam := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")

		// Retrieve the frontend origin captured during the login request.
		returnTo := consumeOAuthReturnTo(stateParam)
		selectedWorkspaceSlug := consumeOAuthWorkspaceSlug(stateParam)

		// Detect flow type from state prefix.
		isInvite := strings.HasPrefix(stateParam, "invite:")
		isSignup := strings.HasPrefix(stateParam, "signup:")
		var inviteToken string
		var signupWSName, signupWSSlug, signupID string

		if isInvite {
			inviteToken = strings.TrimPrefix(stateParam, "invite:")
			if !consumeOAuthState(stateParam) {
				http.Error(w, "invalid or expired state", http.StatusBadRequest)
				return
			}
		} else if isSignup {
			// Parse signup state: "signup:<csrf>:<ws_name>:<ws_slug>:<signup_id>"
			parts := strings.SplitN(stateParam, ":", 5)
			if len(parts) >= 4 {
				signupWSName, _ = url.QueryUnescape(parts[2])
				signupWSSlug, _ = url.QueryUnescape(parts[3])
				if len(parts) >= 5 {
					signupID, _ = url.QueryUnescape(parts[4])
				}
			}
			if !consumeOAuthState(stateParam) {
				http.Error(w, "invalid or expired state", http.StatusBadRequest)
				return
			}
		} else {
			if !consumeOAuthState(stateParam) {
				http.Error(w, "invalid or expired state", http.StatusBadRequest)
				return
			}
		}

		token, err := cfg.Exchange(r.Context(), code)
		if err != nil {
			http.Error(w, "token exchange failed", http.StatusBadRequest)
			return
		}

		client := cfg.Client(r.Context(), token)
		emailAddr, err := fetchEmail(client)
		if err != nil {
			http.Error(w, "failed to get user info", http.StatusInternalServerError)
			return
		}
		email := strings.ToLower(emailAddr)

		db := s.db()
		var userID, wsID, wsSlug, wsRole string

		if isInvite {
			if db == nil {
				http.Error(w, "database not available", http.StatusInternalServerError)
				return
			}

			// Check workspace_invites first, then fall back to legacy invite_tokens.
			var invitedEmail, wsInviteID, wsInviteRole string
			var expiresAt int64
			var used int
			isWorkspaceInvite := false

			err := db.QueryRow(
				state.Rebind(`SELECT email, workspace_id, role, expires_at, used FROM workspace_invites WHERE token = ?`), inviteToken,
			).Scan(&invitedEmail, &wsInviteID, &wsInviteRole, &expiresAt, &used)
			if err == nil {
				isWorkspaceInvite = true
			} else {
				// Fall back to legacy invite_tokens.
				err = db.QueryRow(
					state.Rebind(`SELECT email, expires_at, used FROM invite_tokens WHERE token = ?`), inviteToken,
				).Scan(&invitedEmail, &expiresAt, &used)
			}
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
			if strings.ToLower(invitedEmail) != email {
				http.Error(w, fmt.Sprintf("%s account does not match the invited email", provider), http.StatusForbidden)
				return
			}

			// Create the user record.
			var userID string
			if s.Users != nil {
				user := state.User{
					Name:      email,
					Email:     email,
					Status:    "Active",
					Role:      "Member",
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				}
				if createErr := s.Users.CreateUser(&user); createErr != nil {
					log.Printf("invite: failed to create user %s: %v (may already exist)", email, createErr)
				}
				// Retrieve the user ID (whether just created or existing).
				if s.Workspaces != nil {
					if u, lookupErr := s.Workspaces.GetUserByEmail(email); lookupErr == nil {
						userID = u.ID
					}
				}
			}

			// Mark invite token as used and add to workspace if applicable.
			if isWorkspaceInvite {
				_, _ = db.Exec(state.Rebind(`UPDATE workspace_invites SET used = 1 WHERE token = ?`), inviteToken)
				// Add user as workspace member.
				if s.Workspaces != nil && userID != "" && wsInviteID != "" {
					if wsInviteRole == "" {
						wsInviteRole = "member"
					}
					if addErr := s.Workspaces.AddMember(wsInviteID, userID, wsInviteRole); addErr != nil {
						log.Printf("invite: failed to add user %s to workspace %s: %v", email, wsInviteID, addErr)
					}
				}
			} else {
				_, _ = db.Exec(state.Rebind(`UPDATE invite_tokens SET used = 1 WHERE token = ?`), inviteToken)
			}
			s.writeAdminAudit(db, email, "invite_redeemed", email, "ok")

			// For workspace invites, sign a workspace JWT so the user lands directly in the workspace.
			if isWorkspaceInvite && s.Workspaces != nil && userID != "" && wsInviteID != "" {
				if ws, wsErr := s.Workspaces.GetWorkspace(wsInviteID); wsErr == nil {
					inviteSessionID, _ := randomHex(16)
					if s.Sessions != nil {
						// Enforce concurrent session limit
						if s.MaxSessionsPerUser > 0 && userID != "" {
							if count, countErr := s.Sessions.CountActiveForUser(userID); countErr == nil && count >= s.MaxSessionsPerUser {
								_ = s.Sessions.RevokeOldestSessionsForUser(userID, s.MaxSessionsPerUser-1)
							}
						}
						now := time.Now().Unix()
						sess := &state.Session{
							ID:                inviteSessionID,
							UserID:            userID,
							WorkspaceID:       ws.ID,
							SessionType:       "admin",
							IPAddress:         r.RemoteAddr,
							UserAgent:         r.Header.Get("User-Agent"),
							CreatedAt:         now,
							ExpiresAt:         now + 24*60*60,
							AbsoluteExpiresAt: now + 30*24*60*60,
							IPSubnet:          state.ExtractIPSubnet(r.RemoteAddr),
							UAHash:            state.HashUserAgent(r.Header.Get("User-Agent")),
						}
						_ = s.Sessions.Create(sess)
					}
					wsToken, wsJWTErr := s.signAdminJWT(email, userID, ws.ID, ws.Slug, wsInviteRole, inviteSessionID)
					if wsJWTErr == nil {
						s.setSessionCookie(w, wsToken)
						dashboardURL := s.resolveDashboardURL(returnTo)
						redirect := fmt.Sprintf("%s/app/welcome?token=%s", dashboardURL, url.QueryEscape(wsToken))
						http.Redirect(w, r, redirect, http.StatusFound)
						return
					}
					log.Printf("invite: failed to sign workspace JWT: %v", wsJWTErr)
				}
			}
			http.Error(w, "workspace invite context required", http.StatusForbidden)
			return
		} else if isSignup {
			// Signup flow: check domain allowlist if configured.
			if len(s.SignupAllowedDomains) > 0 {
				parts := strings.SplitN(email, "@", 2)
				if len(parts) != 2 {
					http.Error(w, "invalid email", http.StatusBadRequest)
					return
				}
				if _, ok := s.SignupAllowedDomains[parts[1]]; !ok {
					http.Error(w, "signup not allowed for this email domain", http.StatusForbidden)
					return
				}
			}
			// Create user record if not exists.
			if s.Users != nil {
				user := state.User{
					Name:      email,
					Email:     email,
					Status:    "Active",
					Role:      "Owner",
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				}
				if createErr := s.Users.CreateUser(&user); createErr != nil {
					log.Printf("signup: failed to create user %s: %v (may already exist)", email, createErr)
				}
			}
			if db != nil {
				s.writeAdminAudit(db, email, "signup", email, "ok")
			}
		} else {
			if selectedWorkspaceSlug != "" && s.Workspaces != nil {
				ws, wsErr := s.Workspaces.GetWorkspaceBySlug(selectedWorkspaceSlug)
				if wsErr != nil || ws == nil || strings.ToLower(strings.TrimSpace(ws.Status)) != "active" {
					http.Error(w, "workspace not found", http.StatusForbidden)
					return
				}
				u, lookupErr := s.Workspaces.GetUserByEmail(email)
				if lookupErr != nil {
					http.Error(w, "email not authorised", http.StatusForbidden)
					return
				}
				member, memberErr := s.Workspaces.GetMember(ws.ID, u.ID)
				if memberErr != nil {
					http.Error(w, "email not authorised", http.StatusForbidden)
					return
				}
				userID = u.ID
				wsID = ws.ID
				wsSlug = ws.Slug
				wsRole = member.Role
				if db != nil {
					s.writeAdminAudit(db, email, "workspace_login", email, "ok")
				}
			} else {
				// Regular login without a workspace selection remains admin-only.
				if len(s.AdminLoginEmails) > 0 {
					if _, ok := s.AdminLoginEmails[email]; !ok {
						http.Error(w, "email not authorised", http.StatusForbidden)
						return
					}
				}
				if db != nil {
					s.writeAdminAudit(db, email, "admin_login", email, "ok")
				}
			}
		}

		sessionID, _ := randomHex(16)
		if userID == "" && s.Users != nil && s.Workspaces != nil {
			if u, lookupErr := s.Workspaces.GetUserByEmail(email); lookupErr == nil {
				userID = u.ID
			}
		}
		if s.Sessions != nil {
			// Enforce concurrent session limit for admin sessions
			maxSessions := s.MaxSessionsPerUser
			if maxSessions <= 0 {
				maxSessions = 5 // Default limit
			}
			activeCount, countErr := s.Sessions.CountActiveForUser(userID)
			if countErr != nil {
				log.Printf("oauth callback: failed to count sessions: %v", countErr)
			} else if activeCount >= maxSessions {
				// Revoke oldest sessions to make room
				if revokeErr := s.Sessions.RevokeOldestSessionsForUser(userID, maxSessions-1); revokeErr != nil {
					log.Printf("oauth callback: failed to revoke old sessions: %v", revokeErr)
				}
				// Audit log the session limit enforcement
				s.writeAdminAudit(db, email, "session_limit_enforced", "revoked oldest sessions", "ok")
			}

			now := time.Now().Unix()
			sess := &state.Session{
				ID:                sessionID,
				UserID:            userID,
				WorkspaceID:       wsID,
				SessionType:       "admin",
				IPAddress:         r.RemoteAddr,
				UserAgent:         r.Header.Get("User-Agent"),
				CreatedAt:         now,
				ExpiresAt:         now + 24*60*60,    // 24 hours
				AbsoluteExpiresAt: now + 30*24*60*60, // 30 days max lifetime
				IPSubnet:          state.ExtractIPSubnet(r.RemoteAddr),
				UAHash:            state.HashUserAgent(r.Header.Get("User-Agent")),
			}
			_ = s.Sessions.Create(sess)
		}
		jwtToken, jwtErr := s.signAdminJWT(email, userID, wsID, wsSlug, wsRole, sessionID)
		if jwtErr != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}
		s.setSessionCookie(w, jwtToken)

		dashboardURL := s.resolveDashboardURL(returnTo)
		redirect := fmt.Sprintf("%s?token=%s", dashboardURL, url.QueryEscape(jwtToken))
		// For signup flow, include workspace data in the redirect so the frontend
		// can auto-create the workspace without relying on sessionStorage.
		if isSignup && signupWSName != "" && signupWSSlug != "" {
			redirect += fmt.Sprintf(
				"&ws_name=%s&ws_slug=%s&signup_id=%s",
				url.QueryEscape(signupWSName),
				url.QueryEscape(signupWSSlug),
				url.QueryEscape(signupID),
			)
		}
		http.Redirect(w, r, redirect, http.StatusFound)
	}
}

// handleOAuthLogin redirects the user to Google's consent screen (backward compat wrapper).
func (s *Server) handleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	s.handleProviderLogin("Google", s.OAuthConfig)(w, r)
}

// handleOAuthCallback handles the redirect from Google after user consent (backward compat wrapper).
// It also handles the device PKCE flow when the state starts with "device:".
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Query().Get("state"), "device:") {
		s.handleDeviceCallback(w, r)
		return
	}
	s.handleProviderCallback("Google", s.OAuthConfig, fetchGoogleEmail)(w, r)
}

// handleOAuthLogout clears the session cookie.
func (s *Server) handleOAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	email := ""
	if len(s.JWTSecret) > 0 {
		email, _ = s.sessionFromRequest(r)
	}
	s.clearSessionCookie(w)
	if db := s.db(); db != nil && email != "" {
		s.writeAdminAudit(db, email, "admin_logout", email, "ok")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// handleInviteAccept validates an invite token and starts the OAuth flow.
// GET /invite?token=<token>
func (s *Server) handleInviteAccept(w http.ResponseWriter, r *http.Request) {
	if s.OAuthConfig == nil {
		http.Error(w, "OAuth not configured", http.StatusServiceUnavailable)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}
	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	// Check workspace_invites first, then fall back to legacy invite_tokens.
	var expiresAt int64
	var used int
	err := db.QueryRow(state.Rebind(`SELECT expires_at, used FROM workspace_invites WHERE token = ?`), token).
		Scan(&expiresAt, &used)
	if err == sql.ErrNoRows {
		// Fall back to legacy invite_tokens table.
		err = db.QueryRow(state.Rebind(`SELECT expires_at, used FROM invite_tokens WHERE token = ?`), token).
			Scan(&expiresAt, &used)
	}
	if err == sql.ErrNoRows {
		http.Error(w, "invite token not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if used != 0 {
		http.Error(w, "invite token already used", http.StatusGone)
		return
	}
	if time.Now().Unix() > expiresAt {
		http.Error(w, "invite token expired", http.StatusGone)
		return
	}

	// Embed the invite token in the OAuth state so it survives the redirect round-trip.
	oauthState := "invite:" + token
	if returnTo := r.URL.Query().Get("return_to"); returnTo != "" {
		storeOAuthReturnTo(oauthState, returnTo)
	}
	storeOAuthState(oauthState)
	authURL := s.OAuthConfig.AuthCodeURL(oauthState, oauth2.AccessTypeOnline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleInviteUser sends an invite email.
// POST /api/admin/users/invite
func (s *Server) handleInviteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email       string `json:"email"`
		WorkspaceID string `json:"workspace_id"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	inviteToken, err := randomHex(24)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	expiresAt := time.Now().Add(48 * time.Hour).Unix()

	// Determine workspace ID: prefer explicit field, then JWT context.
	wsID := req.WorkspaceID
	if wsID == "" {
		wsID = s.workspaceIDFromRequest(r)
	}
	if wsID == "" {
		http.Error(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}

	_, err = db.Exec(
		state.Rebind(`INSERT INTO workspace_invites (token, workspace_id, email, role, expires_at, used) VALUES (?, ?, ?, ?, ?, 0)`),
		inviteToken, wsID, email, req.Role, expiresAt,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create invite token: %v", err), http.StatusInternalServerError)
		return
	}

	inviteBaseURL := s.InviteBaseURL
	if inviteBaseURL == "" {
		inviteBaseURL = "http://localhost:8081"
	}
	inviteURL := fmt.Sprintf("%s/invite?token=%s", inviteBaseURL, inviteToken)

	if s.Mailer != nil {
		if mailErr := s.Mailer.SendInvite(email, inviteURL); mailErr != nil {
			log.Printf("invite: failed to send email to %s: %v", email, mailErr)
			http.Error(w, "failed to send invite email", http.StatusInternalServerError)
			return
		}
	} else {
		log.Printf("invite: SMTP not configured; invite URL for %s: %s", email, inviteURL)
	}

	actor := sessionEmailFromContext(r.Context())
	if actor == "" {
		actor = "admin"
	}
	s.writeAdminAudit(db, actor, "invite_sent", email, "ok")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "invited",
		"email":      email,
		"invite_url": inviteURL,
	})
}

// handleAdminAuditLogs returns paginated admin audit log entries.
// GET /api/admin/audit-logs?limit=50&offset=0
func (s *Server) handleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := s.db()
	if db == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := db.Query(
		state.Rebind(`SELECT id, timestamp, actor, action, target, result FROM admin_audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`),
		limit, offset,
	)
	if err != nil {
		http.Error(w, "failed to query audit logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type entry struct {
		ID        int64  `json:"id"`
		Timestamp int64  `json:"timestamp"`
		Actor     string `json:"actor"`
		Action    string `json:"action"`
		Target    string `json:"target"`
		Result    string `json:"result"`
	}
	out := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Actor, &e.Action, &e.Target, &e.Result); err != nil {
			http.Error(w, "scan error", http.StatusInternalServerError)
			return
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

// writeAdminAudit inserts a row into admin_audit_logs.
func (s *Server) writeAdminAudit(db *sql.DB, actor, action, target, result string) {
	_, err := db.Exec(
		state.Rebind(`INSERT INTO admin_audit_logs (timestamp, actor, action, target, result) VALUES (?, ?, ?, ?, ?)`),
		time.Now().Unix(), actor, action, target, result,
	)
	if err != nil {
		log.Printf("audit: failed to write admin audit log: %v", err)
	}
}

// resolveDashboardURL returns the URL to redirect the user to after OAuth.
// It prefers the return_to value if it matches an allowed origin, then falls
// back to the configured DashboardURL, then to localhost.
func (s *Server) resolveDashboardURL(returnTo string) string {
	if returnTo != "" && s.isAllowedRedirectOrigin(returnTo) {
		return strings.TrimRight(returnTo, "/")
	}
	if s.DashboardURL != "" {
		return s.DashboardURL
	}
	return "http://localhost:5173"
}

// isAllowedRedirectOrigin checks if a URL's origin matches the configured
// dashboard URL or CORS allowed origins. Prevents open redirect attacks.
func (s *Server) isAllowedRedirectOrigin(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	target := u.Scheme + "://" + u.Host
	if s.DashboardURL != "" {
		if du, err := url.Parse(s.DashboardURL); err == nil {
			allowed := du.Scheme + "://" + du.Host
			if target == allowed {
				return true
			}
		}
	}
	for _, origin := range corsAllowedOrigins {
		if ou, err := url.Parse(origin); err == nil {
			allowed := ou.Scheme + "://" + ou.Host
			if target == allowed {
				return true
			}
		}
	}
	// Allow loopback and private/LAN IPs — safe since they are not
	// internet-routable and this is a self-hosted tool.
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return true
	}
	return false
}

// BuildOAuthConfig returns a configured *oauth2.Config for Google if clientID is set, otherwise nil.
// Deprecated: Use BuildGoogleOAuthConfig instead.
func BuildOAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return BuildGoogleOAuthConfig(clientID, clientSecret, redirectURL)
}
