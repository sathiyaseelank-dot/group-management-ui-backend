package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/ca"
	"controller/mailer"
	"controller/state"

	"golang.org/x/oauth2"
)

// ConnectorStreamChecker allows the admin server to query whether a connector
// has an active gRPC control-plane stream without importing the api package.
type ConnectorStreamChecker interface {
	IsStreamActive(id string) bool
}

type Server struct {
	Tokens    *state.TokenStore
	Reg       *state.Registry
	Agents    *state.AgentStatusRegistry
	ACLs      *state.ACLStore
	ACLNotify ACLNotifier
	Users     *state.UserStore
	RemoteNet *state.RemoteNetworkStore

	// Discovery
	ScanStore    *state.ScanStore
	ControlPlane DiscoverySender

	// Diagnostics
	StreamChecker ConnectorStreamChecker

	AdminAuthToken    string
	InternalAuthToken string
	CACertPEM         []byte

	// OAuth + JWT session
	OAuthConfig       *oauth2.Config // Google (backward compat)
	GitHubOAuthConfig *oauth2.Config
	JWTSecret         []byte
	AdminLoginEmails  map[string]struct{}
	DashboardURL      string
	InviteBaseURL     string

	// SMTP mailer (nil = disabled)
	Mailer *mailer.Mailer

	// Workspace multi-tenancy
	Workspaces     *state.WorkspaceStore
	IntermediateCA *ca.CA
	SystemDomain   string // e.g. "zerotrust.com"

	// Phase 1: Multi-IdP
	IdPs *state.IdentityProviderStore

	// Phase 2: Session management
	Sessions       *state.SessionStore
	SecureCookies  bool
	AllowedOrigins []string
}

// db returns the underlying *sql.DB via the ACLStore, or nil.
func (s *Server) db() *sql.DB {
	if s.ACLs != nil {
		return s.ACLs.DB()
	}
	return nil
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// CA cert is public — no auth required. Connectors and agents fetch it
	// during bootstrap before any trust is established (same pattern as Vault
	// /v1/pki/ca/pem, Consul /v1/connect/ca/roots, Teleport, etc.)
	mux.HandleFunc("/ca.crt", s.handleCACert)
	mux.Handle("/api/admin/tokens", s.adminAuth(http.HandlerFunc(s.handleCreateToken)))
	mux.Handle("/api/admin/connectors", s.adminAuth(http.HandlerFunc(s.handleListConnectors)))
	mux.Handle("/api/admin/connectors/", s.adminAuth(http.HandlerFunc(s.handleConnectorSubroutes)))
	mux.Handle("/api/admin/agents", s.adminAuth(http.HandlerFunc(s.handleListAgents)))
	mux.Handle("/api/admin/agents/", s.adminAuth(http.HandlerFunc(s.handleAgentSubroutes)))
	mux.Handle("/api/admin/resources", s.adminAuth(http.HandlerFunc(s.handleResources)))
	mux.Handle("/api/admin/resources/", s.adminAuth(http.HandlerFunc(s.handleResourceSubroutes)))
	mux.Handle("/api/admin/audit", s.adminAuth(http.HandlerFunc(s.handleAuditLog)))
	mux.Handle("/api/admin/users", s.adminAuth(http.HandlerFunc(s.handleUsers)))
	mux.Handle("/api/admin/users/", s.adminAuth(http.HandlerFunc(s.handleUserSubroutes)))
	mux.Handle("/api/admin/user-groups", s.adminAuth(http.HandlerFunc(s.handleUserGroups)))
	mux.Handle("/api/admin/user-groups/", s.adminAuth(http.HandlerFunc(s.handleUserGroupMembers)))
	mux.Handle("/api/admin/remote-networks", s.adminAuth(http.HandlerFunc(s.handleRemoteNetworks)))
	mux.Handle("/api/admin/remote-networks/", s.adminAuth(http.HandlerFunc(s.handleRemoteNetworkConnectors)))
	mux.Handle("/api/internal/consume-token", s.internalAuth(http.HandlerFunc(s.handleConsumeToken)))
	s.RegisterWorkspaceRoutes(mux)
	s.RegisterUIRoutes(mux)
}

type ACLNotifier interface {
	NotifyACLInit()
	NotifyResourceUpsert(res state.Resource)
	NotifyResourceRemoved(resourceID string)
	NotifyAuthorizationUpsert(auth state.Authorization)
	NotifyAuthorizationRemoved(resourceID, principalSPIFFE string)
	NotifyPolicyChange()
}

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.AdminAuthToken == "" {
			http.Error(w, "admin auth not configured", http.StatusServiceUnavailable)
			return
		}
		// Accept Bearer ADMIN_AUTH_TOKEN (BFF compat).
		auth := r.Header.Get("Authorization")
		if auth == "Bearer "+s.AdminAuthToken {
			next.ServeHTTP(w, r)
			return
		}
		// Accept valid JWT session.
		if len(s.JWTSecret) > 0 {
			claims, err := parseAllClaims(s.getTokenFromRequest(r), s.JWTSecret)
			if err == nil {
				// Reject device tokens on admin endpoints.
				if claims.aud == "device" {
					http.Error(w, "device tokens cannot access admin endpoints", http.StatusUnauthorized)
					return
				}
				// Validate session not revoked (if Sessions store and jti present).
				if s.Sessions != nil && claims.jti != "" {
					if valid, err := s.Sessions.IsValid(claims.jti); err == nil && !valid {
						http.Error(w, "session revoked or expired", http.StatusUnauthorized)
						return
					}
				}
				if !s.isAdminEmail(claims.email) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				ctx := withSessionEmail(r.Context(), claims.email)
				if claims.userID != "" {
					ctx = withWorkspace(ctx, claims.userID, claims.wsID, claims.wsSlug, claims.wsRole)
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func (s *Server) isAdminEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	if len(s.AdminLoginEmails) > 0 {
		_, ok := s.AdminLoginEmails[email]
		return ok
	}
	db := s.db()
	if db == nil {
		return false
	}
	var status, role string
	err := db.QueryRow(state.Rebind(`SELECT status, role FROM users WHERE LOWER(email) = ?`), email).Scan(&status, &role)
	if err != nil {
		return false
	}
	status = strings.ToLower(strings.TrimSpace(status))
	role = strings.ToLower(strings.TrimSpace(role))
	if status != "active" {
		return false
	}
	return role == "admin" || role == "owner"
}

// deviceAuth accepts only device JWTs (aud:"device").
func (s *Server) deviceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.JWTSecret) == 0 {
			http.Error(w, "JWT not configured", http.StatusServiceUnavailable)
			return
		}
		claims, err := parseAllClaims(s.getTokenFromRequest(r), s.JWTSecret)
		if err != nil || claims.aud != "device" {
			http.Error(w, "unauthorized: device token required", http.StatusUnauthorized)
			return
		}
		if s.Sessions != nil && claims.jti != "" {
			if valid, err := s.Sessions.IsValid(claims.jti); err == nil && !valid {
				http.Error(w, "session revoked or expired", http.StatusUnauthorized)
				return
			}
		}
		ctx := withSessionEmail(r.Context(), claims.email)
		role := claims.wsRole
		if strings.TrimSpace(role) == "" {
			role = "member"
		}
		ctx = withWorkspace(ctx, claims.userID, claims.wsID, claims.wsSlug, role)
		ctx = context.WithValue(ctx, contextKey("device_id"), claims.deviceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) internalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.InternalAuthToken == "" {
			http.Error(w, "internal auth not configured", http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("X-Internal-Token") != s.InternalAuthToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// workspaceAuth validates JWT and extracts workspace claims into context.
// Workspace claims are optional — JWTs without them are still valid.
func (s *Server) workspaceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.JWTSecret) == 0 {
			http.Error(w, "JWT not configured", http.StatusServiceUnavailable)
			return
		}
		tokenStr := ""
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			tokenStr = cookie.Value
		} else {
			auth := r.Header.Get("Authorization")
			if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
				tokenStr = after
			}
		}
		if tokenStr == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		email, userID, wsID, wsSlug, wsRole, err := workspaceClaimsFromJWT(tokenStr, s.JWTSecret)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := withSessionEmail(r.Context(), email)
		ctx = withWorkspace(ctx, userID, wsID, wsSlug, wsRole)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireWorkspace rejects requests without workspace claims in the JWT.
func requireWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if workspaceIDFromContext(r.Context()) == "" {
			http.Error(w, "workspace context required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireWorkspaceRole rejects requests where the user's workspace role is insufficient.
// Role hierarchy: owner > admin > member.
func requireWorkspaceRole(minRole string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := workspaceRoleFromContext(r.Context())
		if !roleAtLeast(role, minRole) {
			http.Error(w, "insufficient workspace role", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func roleAtLeast(role, minRole string) bool {
	levels := map[string]int{"member": 1, "admin": 2, "owner": 3}
	return levels[role] >= levels[minRole]
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wsID := s.workspaceIDFromRequest(r)
	token, expires, err := s.Tokens.CreateTokenForWorkspace(wsID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create token: %v", err), http.StatusInternalServerError)
		return
	}

	resp := map[string]string{
		"token":      token,
		"expires_at": expires.UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleConsumeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Token       string `json:"token"`
		ConnectorID string `json:"connector_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ConnectorID == "" {
		http.Error(w, "missing connector_id", http.StatusBadRequest)
		return
	}
	if err := s.Tokens.ConsumeToken(req.Token, req.ConnectorID); err != nil {
		http.Error(w, fmt.Sprintf("token invalid: %v", err), http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	records := s.Reg.List()
	now := time.Now().UTC()
	type respConnector struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		PrivateIP string `json:"private_ip"`
		LastSeen  string `json:"last_seen"`
		Version   string `json:"version"`
	}
	resp := make([]respConnector, 0, len(records))
	for _, rec := range records {
		status := "OFFLINE"
		if now.Sub(rec.LastSeen) < 30*time.Second {
			status = "ONLINE"
		}
		resp = append(resp, respConnector{
			ID:        rec.ID,
			Status:    status,
			PrivateIP: rec.PrivateIP,
			LastSeen:  humanizeDuration(now.Sub(rec.LastSeen)),
			Version:   rec.Version,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleConnectorSubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/connectors/")
	if id == "" {
		http.Error(w, "connector id required", http.StatusBadRequest)
		return
	}
	s.Reg.Delete(id)
	if s.ACLs != nil && s.ACLs.DB() != nil {
		_ = state.DeleteConnectorFromDB(s.ACLs.DB(), id)
	}
	if s.Tokens != nil {
		_ = s.Tokens.DeleteByConnectorID(id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ACLs == nil || s.ACLs.DB() == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	rows, err := s.ACLs.DB().Query(`SELECT principal_spiffe, tunneler_id, resource_id, destination, protocol, port, decision, reason, connection_id, created_at FROM audit_logs ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		http.Error(w, "failed to query audit logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type audit struct {
		PrincipalSPIFFE string `json:"principal_spiffe"`
		TunnelerID      string `json:"tunneler_id"`
		ResourceID      string `json:"resource_id"`
		Destination     string `json:"destination"`
		Protocol        string `json:"protocol"`
		Port            int    `json:"port"`
		Decision        string `json:"decision"`
		Reason          string `json:"reason"`
		ConnectionID    string `json:"connection_id"`
		CreatedAt       int64  `json:"created_at"`
	}
	out := []audit{}
	for rows.Next() {
		var row audit
		if err := rows.Scan(&row.PrincipalSPIFFE, &row.TunnelerID, &row.ResourceID, &row.Destination, &row.Protocol, &row.Port, &row.Decision, &row.Reason, &row.ConnectionID, &row.CreatedAt); err != nil {
			http.Error(w, "failed to read audit logs", http.StatusInternalServerError)
			return
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ACLs == nil || s.ACLs.DB() == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	db := s.ACLs.DB()
	rows, err := db.Query(`SELECT id, version, hostname, connector_id, remote_network_id, last_seen FROM tunnelers`)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	now := time.Now().UTC().Unix()
	type respAgent struct {
		ID              string `json:"id"`
		Status          string `json:"status"`
		Version         string `json:"version"`
		Hostname        string `json:"hostname"`
		ConnectorID     string `json:"connector_id"`
		RemoteNetworkID string `json:"remote_network_id"`
		LastSeen        string `json:"last_seen"`
	}
	resp := make([]respAgent, 0)
	for rows.Next() {
		var id, version, hostname, connectorID, remoteNetworkID string
		var lastSeen int64
		if err := rows.Scan(&id, &version, &hostname, &connectorID, &remoteNetworkID, &lastSeen); err != nil {
			continue
		}
		status := "OFFLINE"
		if now-lastSeen < 30 {
			status = "ONLINE"
		}
		resp = append(resp, respAgent{
			ID:              id,
			Status:          status,
			Version:         version,
			Hostname:        hostname,
			ConnectorID:     connectorID,
			RemoteNetworkID: remoteNetworkID,
			LastSeen:        humanizeDuration(time.Duration(now-lastSeen) * time.Second),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAgentSubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/agents/")
	if id == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	if s.Agents != nil {
		s.Agents.Delete(id)
	}
	if s.ACLs != nil && s.ACLs.DB() != nil {
		db := s.ACLs.DB()
		_, _ = db.Exec(`DELETE FROM tunnelers WHERE id = ?`, id)
		// Revoke the enrollment token so the agent cannot re-enroll after deletion.
		_, _ = db.Exec(`DELETE FROM tokens WHERE connector_id = ?`, id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	if s.ACLs == nil {
		http.Error(w, "acl store not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		stateSnap := s.ACLs.Snapshot()
		writeJSON(w, http.StatusOK, stateSnap)
	case http.MethodPost:
		var res state.Resource
		if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := s.ACLs.UpsertResource(res); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if s.ACLs != nil && s.ACLs.DB() != nil {
			_ = state.SaveResourceToDB(s.ACLs.DB(), res)
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyResourceUpsert(res)
		}
		writeJSON(w, http.StatusOK, res)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleResourceSubroutes(w http.ResponseWriter, r *http.Request) {
	if s.ACLs == nil {
		http.Error(w, "acl store not configured", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/resources/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "resource id required", http.StatusBadRequest)
		return
	}
	resourceID := parts[0]
	if len(parts) == 1 {
		if r.Method == http.MethodDelete {
			s.ACLs.DeleteResource(resourceID)
			if s.ACLs != nil && s.ACLs.DB() != nil {
				_ = state.DeleteResourceFromDB(s.ACLs.DB(), resourceID)
			}
			if s.ACLNotify != nil {
				s.ACLNotify.NotifyResourceRemoved(resourceID)
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch parts[1] {
	case "filters":
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var filters []state.Filter
		if err := json.NewDecoder(r.Body).Decode(&filters); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := s.ACLs.UpdateFilters(resourceID, filters); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if s.ACLs != nil && s.ACLs.DB() != nil {
			stateSnap := s.ACLs.Snapshot()
			for _, auth := range stateSnap.Authorizations {
				if auth.ResourceID == resourceID {
					_ = state.SaveAuthorizationToDB(s.ACLs.DB(), auth)
				}
			}
		}
		if s.ACLNotify != nil {
			stateSnap := s.ACLs.Snapshot()
			for _, auth := range stateSnap.Authorizations {
				if auth.ResourceID == resourceID {
					s.ACLNotify.NotifyAuthorizationUpsert(auth)
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case "assign_principal":
		if r.Method == http.MethodPost {
			var req struct {
				PrincipalSPIFFE string         `json:"principal_spiffe"`
				Filters         []state.Filter `json:"filters,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if err := s.ACLs.AssignPrincipal(resourceID, req.PrincipalSPIFFE, req.Filters); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			auth := state.Authorization{PrincipalSPIFFE: req.PrincipalSPIFFE, ResourceID: resourceID, Filters: req.Filters}
			if s.ACLs != nil && s.ACLs.DB() != nil {
				_ = state.SaveAuthorizationToDB(s.ACLs.DB(), auth)
			}
			if s.ACLNotify != nil {
				s.ACLNotify.NotifyAuthorizationUpsert(auth)
			}
			writeJSON(w, http.StatusOK, auth)
			return
		}
		if r.Method == http.MethodDelete && len(parts) >= 3 {
			principal := parts[2]
			s.ACLs.RemoveAssignment(resourceID, principal)
			if s.ACLs != nil && s.ACLs.DB() != nil {
				_ = state.DeleteAuthorizationFromDB(s.ACLs.DB(), resourceID, principal)
			}
			if s.ACLNotify != nil {
				s.ACLNotify.NotifyAuthorizationRemoved(resourceID, principal)
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	default:
		http.Error(w, "unknown subresource", http.StatusNotFound)
	}
}

func (s *Server) handleCACert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if len(s.CACertPEM) == 0 {
		http.Error(w, "CA cert not available", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="ca.crt"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.CACertPEM)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d.Seconds())
	switch {
	case seconds < 5:
		return "just now"
	case seconds < 60:
		return fmt.Sprintf("%d seconds ago", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%d minutes ago", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%d hours ago", seconds/3600)
	default:
		return fmt.Sprintf("%d days ago", seconds/86400)
	}
}
