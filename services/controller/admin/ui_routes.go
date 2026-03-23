package admin

import (
	"net/http"
	"strings"
	"time"
)

// Package-level rate limiters for OAuth endpoints.
var (
	oauthCallbackRL *rateLimiter
	inviteTokenRL   *rateLimiter
)

// Package-level CORS configuration, set via SetCORSOrigins before routes are registered.
var (
	corsAllowedOrigins []string
	corsDashboardURL   string
)

// SetCORSOrigins configures the allowed origins for CORS. Called from main.go.
func SetCORSOrigins(origins []string, dashboardURL string) {
	corsAllowedOrigins = origins
	corsDashboardURL = dashboardURL
}

func init() {
	oauthCallbackRL = newRateLimiter(1*time.Minute, 5)
	inviteTokenRL = newRateLimiter(1*time.Minute, 3)
}

func (s *Server) RegisterOAuthRoutes(mux *http.ServeMux) {
	// OAuth login / callback / logout — no auth required (they establish auth).
	mux.Handle("/oauth/google/login", withCORS(http.HandlerFunc(s.handleOAuthLogin)))
	mux.Handle("/oauth/google/callback", withRateLimit(oauthCallbackRL, withCORS(http.HandlerFunc(s.handleOAuthCallback))))

	// GitHub OAuth routes
	mux.Handle("/oauth/github/login", withCORS(s.handleProviderLogin("GitHub", s.GitHubOAuthConfig)))
	mux.Handle("/oauth/github/callback", withCORS(s.handleProviderCallback("GitHub", s.GitHubOAuthConfig, fetchGitHubEmail)))

	mux.Handle("/oauth/logout", withCORS(http.HandlerFunc(s.handleOAuthLogout)))
	// Invite acceptance page — public (token validates itself).
	mux.Handle("/invite", withCORS(http.HandlerFunc(s.handleInviteAccept)))
	// Invite send + admin audit logs — require admin auth.
	mux.Handle("/api/admin/users/invite", withCORS(s.adminAuth(http.HandlerFunc(s.handleInviteUser))))
	mux.Handle("/api/admin/audit-logs", withCORS(s.adminAuth(http.HandlerFunc(s.handleAdminAuditLogs))))

	// Invite PKCE flow (browser-based registration with PKCE + ID token validation)
	mux.Handle("/api/invite/authorize", withCORS(http.HandlerFunc(s.handleInviteAuthorize)))
	mux.Handle("/api/invite/callback", http.HandlerFunc(s.handleInviteCallback)) // no CORS: browser navigates here
	mux.Handle("/api/invite/token", withRateLimit(inviteTokenRL, withCORS(http.HandlerFunc(s.handleInviteToken))))
}

func (s *Server) RegisterUIRoutes(mux *http.ServeMux) {
	// wsAdmin requires a valid workspace JWT with admin or owner role.
	// Used for all dashboard management endpoints.
	wsAdmin := func(next http.Handler) http.Handler {
		return s.withWorkspaceContext(requireWorkspaceRole("admin", csrfProtect(next)))
	}
	// wsMember requires a valid workspace JWT (any role).
	// Used for endpoints that regular members need access to (e.g. JIT access requests).
	wsMember := func(next http.Handler) http.Handler {
		return s.withWorkspaceContext(csrfProtect(next))
	}
	mux.Handle("/api/users", withCORS(wsAdmin(http.HandlerFunc(s.handleUIUsers))))
	mux.Handle("/api/groups", withCORS(wsAdmin(http.HandlerFunc(s.handleUIGroups))))
	mux.Handle("/api/groups/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIGroupsSubroutes))))
	mux.Handle("/api/resources", withCORS(wsAdmin(http.HandlerFunc(s.handleUIResources))))
	mux.Handle("/api/resources/batch", withCORS(wsAdmin(http.HandlerFunc(s.handleUIResourcesBatch))))
	mux.Handle("/api/resources/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIResourcesSubroutes))))
	mux.Handle("/api/access-rules", withCORS(wsAdmin(http.HandlerFunc(s.handleUIAccessRules))))
	mux.Handle("/api/access-rules/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIAccessRulesSubroutes))))
	mux.Handle("/api/remote-networks", withCORS(wsAdmin(http.HandlerFunc(s.handleUIRemoteNetworks))))
	mux.Handle("/api/remote-networks/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIRemoteNetworksSubroutes))))
	mux.Handle("/api/connectors", withCORS(wsAdmin(http.HandlerFunc(s.handleUIConnectors))))
	mux.Handle("/api/connectors/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIConnectorsSubroutes))))
	mux.Handle("/api/agents", withCORS(wsAdmin(http.HandlerFunc(s.handleUIAgents))))
	mux.Handle("/api/agents/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIAgentsSubroutes))))
	mux.Handle("/api/subjects", withCORS(wsAdmin(http.HandlerFunc(s.handleUISubjects))))
	mux.Handle("/api/service-accounts", withCORS(wsAdmin(http.HandlerFunc(s.handleUIServiceAccounts))))
	mux.Handle("/api/policy/compile/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIPolicyCompile))))
	mux.Handle("/api/policy/acl/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIPolicyACL))))
	mux.Handle("/api/diagnostics", withCORS(wsAdmin(http.HandlerFunc(s.handleUIDiagnostics))))
	mux.Handle("/api/diagnostics/ping/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIDiagnosticsPing))))
	mux.Handle("/api/diagnostics/trace", withCORS(wsAdmin(http.HandlerFunc(s.handleUIDiagnosticsTrace))))

	// Discovery routes (admin-authed with CORS)
	mux.Handle("/api/admin/discovery/scan", withCORS(s.adminAuth(http.HandlerFunc(s.handleStartScan))))
	mux.Handle("/api/admin/discovery/scan/", withCORS(s.adminAuth(http.HandlerFunc(s.handleScanStatus))))
	mux.Handle("/api/admin/discovery/results", withCORS(s.adminAuth(http.HandlerFunc(s.handleDiscoveryResults))))

	// Agent discovery routes
	mux.Handle("/api/admin/agent-discovery/results", withCORS(wsAdmin(http.HandlerFunc(s.handleAgentDiscoveryResults))))
	mux.Handle("/api/admin/agent-discovery/results/", withCORS(wsAdmin(http.HandlerFunc(s.handleAgentDiscoveryDismiss))))
	mux.Handle("/api/admin/agent-discovery/summary", withCORS(wsAdmin(http.HandlerFunc(s.handleAgentDiscoverySummary))))

	// Identity providers (Phase 1)
	mux.Handle("/api/admin/identity-providers", withCORS(s.workspaceAuth(requireWorkspace(http.HandlerFunc(s.handleIdentityProviders)))))
	mux.Handle("/api/admin/identity-providers/", withCORS(s.workspaceAuth(requireWorkspace(http.HandlerFunc(s.handleIdentityProviderSubroutes)))))

	// Sessions (Phase 2)
	mux.Handle("/api/admin/sessions", withCORS(s.workspaceAuth(requireWorkspace(http.HandlerFunc(s.handleSessions)))))
	mux.Handle("/api/admin/sessions/", withCORS(s.workspaceAuth(requireWorkspace(http.HandlerFunc(s.handleSessionSubroutes)))))

	// Device auth routes (Phase 3)
	s.RegisterDeviceAuthRoutes(mux)

	// Device posture & trusted profiles
	mux.Handle("/api/device-trusted-profiles", withCORS(wsAdmin(http.HandlerFunc(s.handleUIDeviceTrustedProfiles))))
	mux.Handle("/api/device-trusted-profiles/", withCORS(wsAdmin(http.HandlerFunc(s.handleUIDeviceTrustedProfilesSubroutes))))
	mux.Handle("/api/device-posture", withCORS(wsAdmin(http.HandlerFunc(s.handleUIDevicePosture))))
	mux.Handle("/api/devices", withCORS(wsAdmin(http.HandlerFunc(s.handleUIDevices))))

	// JIT access requests — members can create/view; approval requires admin (enforced in handler)
	mux.Handle("/api/access-requests", withCORS(wsMember(http.HandlerFunc(s.handleAccessRequests))))
	mux.Handle("/api/access-requests/", withCORS(wsMember(http.HandlerFunc(s.handleAccessRequestSubroutes))))
}

// withWorkspaceContext is a middleware that requires a valid JWT and extracts
// workspace claims from it into the request context. Returns 401 if no valid token.
func (s *Server) withWorkspaceContext(next http.Handler) http.Handler {
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

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if len(corsAllowedOrigins) > 0 {
			// Explicit allowlist configured — only allow matching origins.
			for _, allowed := range corsAllowedOrigins {
				if allowed == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					break
				}
			}
		} else if corsDashboardURL != "" {
			// No explicit allowlist, but dashboard URL is set — allow only that.
			if origin == corsDashboardURL {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		} else {
			// Neither configured — dev mode fallback.
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// corsHandler wraps a handler with CORS headers, respecting AllowedOrigins if set.
func (s *Server) corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if len(s.AllowedOrigins) > 0 {
			for _, allowed := range s.AllowedOrigins {
				if allowed == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					break
				}
			}
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
