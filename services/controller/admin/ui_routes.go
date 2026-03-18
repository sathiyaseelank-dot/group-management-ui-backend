package admin

import (
	"net/http"
	"strings"
)

func (s *Server) RegisterOAuthRoutes(mux *http.ServeMux) {
	// OAuth login / callback / logout — no auth required (they establish auth).
	mux.Handle("/oauth/google/login", withCORS(http.HandlerFunc(s.handleOAuthLogin)))
	mux.Handle("/oauth/google/callback", withCORS(http.HandlerFunc(s.handleOAuthCallback)))

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
	mux.Handle("/api/invite/token", withCORS(http.HandlerFunc(s.handleInviteToken)))
}

func (s *Server) RegisterUIRoutes(mux *http.ServeMux) {
	ws := s.withWorkspaceContext // shorthand middleware
	mux.Handle("/api/users", withCORS(ws(http.HandlerFunc(s.handleUIUsers))))
	mux.Handle("/api/groups", withCORS(ws(http.HandlerFunc(s.handleUIGroups))))
	mux.Handle("/api/groups/", withCORS(ws(http.HandlerFunc(s.handleUIGroupsSubroutes))))
	mux.Handle("/api/resources", withCORS(ws(http.HandlerFunc(s.handleUIResources))))
	mux.Handle("/api/resources/batch", withCORS(ws(http.HandlerFunc(s.handleUIResourcesBatch))))
	mux.Handle("/api/resources/", withCORS(ws(http.HandlerFunc(s.handleUIResourcesSubroutes))))
	mux.Handle("/api/access-rules", withCORS(ws(http.HandlerFunc(s.handleUIAccessRules))))
	mux.Handle("/api/access-rules/", withCORS(ws(http.HandlerFunc(s.handleUIAccessRulesSubroutes))))
	mux.Handle("/api/remote-networks", withCORS(ws(http.HandlerFunc(s.handleUIRemoteNetworks))))
	mux.Handle("/api/remote-networks/", withCORS(ws(http.HandlerFunc(s.handleUIRemoteNetworksSubroutes))))
	mux.Handle("/api/connectors", withCORS(ws(http.HandlerFunc(s.handleUIConnectors))))
	mux.Handle("/api/connectors/", withCORS(ws(http.HandlerFunc(s.handleUIConnectorsSubroutes))))
	mux.Handle("/api/agents", withCORS(ws(http.HandlerFunc(s.handleUIAgents))))
	mux.Handle("/api/agents/", withCORS(ws(http.HandlerFunc(s.handleUIAgentsSubroutes))))
	mux.Handle("/api/subjects", withCORS(ws(http.HandlerFunc(s.handleUISubjects))))
	mux.Handle("/api/service-accounts", withCORS(ws(http.HandlerFunc(s.handleUIServiceAccounts))))
	mux.Handle("/api/policy/compile/", withCORS(ws(http.HandlerFunc(s.handleUIPolicyCompile))))
	mux.Handle("/api/policy/acl/", withCORS(ws(http.HandlerFunc(s.handleUIPolicyACL))))
	mux.Handle("/api/diagnostics", withCORS(ws(http.HandlerFunc(s.handleUIDiagnostics))))
	mux.Handle("/api/diagnostics/ping/", withCORS(ws(http.HandlerFunc(s.handleUIDiagnosticsPing))))
	mux.Handle("/api/diagnostics/trace", withCORS(ws(http.HandlerFunc(s.handleUIDiagnosticsTrace))))

	// Discovery routes (admin-authed with CORS)
	mux.Handle("/api/admin/discovery/scan", withCORS(s.adminAuth(http.HandlerFunc(s.handleStartScan))))
	mux.Handle("/api/admin/discovery/scan/", withCORS(s.adminAuth(http.HandlerFunc(s.handleScanStatus))))
	mux.Handle("/api/admin/discovery/results", withCORS(s.adminAuth(http.HandlerFunc(s.handleDiscoveryResults))))

	// Agent discovery routes
	mux.Handle("/api/admin/agent-discovery/results", withCORS(ws(http.HandlerFunc(s.handleAgentDiscoveryResults))))
	mux.Handle("/api/admin/agent-discovery/results/", withCORS(ws(http.HandlerFunc(s.handleAgentDiscoveryDismiss))))
	mux.Handle("/api/admin/agent-discovery/summary", withCORS(ws(http.HandlerFunc(s.handleAgentDiscoverySummary))))

	// Identity providers (Phase 1)
	mux.Handle("/api/admin/identity-providers", withCORS(s.workspaceAuth(requireWorkspace(http.HandlerFunc(s.handleIdentityProviders)))))
	mux.Handle("/api/admin/identity-providers/", withCORS(s.workspaceAuth(requireWorkspace(http.HandlerFunc(s.handleIdentityProviderSubroutes)))))

	// Sessions (Phase 2)
	mux.Handle("/api/admin/sessions", withCORS(s.workspaceAuth(requireWorkspace(http.HandlerFunc(s.handleSessions)))))
	mux.Handle("/api/admin/sessions/", withCORS(s.workspaceAuth(requireWorkspace(http.HandlerFunc(s.handleSessionSubroutes)))))

	// Device auth routes (Phase 3)
	s.RegisterDeviceAuthRoutes(mux)

	// Device posture & trusted profiles
	mux.Handle("/api/device-trusted-profiles", withCORS(ws(http.HandlerFunc(s.handleUIDeviceTrustedProfiles))))
	mux.Handle("/api/device-trusted-profiles/", withCORS(ws(http.HandlerFunc(s.handleUIDeviceTrustedProfilesSubroutes))))
	mux.Handle("/api/device-posture", withCORS(ws(http.HandlerFunc(s.handleUIDevicePosture))))
	mux.Handle("/api/devices", withCORS(ws(http.HandlerFunc(s.handleUIDevices))))
}

// withWorkspaceContext is a middleware that extracts workspace claims from JWT
// and adds them to the request context. It does NOT require workspace claims —
// requests without them proceed with empty workspace context (backward compatible).
func (s *Server) withWorkspaceContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.JWTSecret) > 0 {
			tokenStr := ""
			if cookie, err := r.Cookie(sessionCookieName); err == nil {
				tokenStr = cookie.Value
			} else {
				auth := r.Header.Get("Authorization")
				if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
					tokenStr = after
				}
			}
			if tokenStr != "" {
				if email, userID, wsID, wsSlug, wsRole, err := workspaceClaimsFromJWT(tokenStr, s.JWTSecret); err == nil {
					ctx := withSessionEmail(r.Context(), email)
					ctx = withWorkspace(ctx, userID, wsID, wsSlug, wsRole)
					r = r.WithContext(ctx)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
