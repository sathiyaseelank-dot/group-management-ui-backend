package admin

import (
	"log"
	"net/http"
	"strings"
)

// handleSessions handles GET /api/admin/sessions
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		http.Error(w, "session store not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wsID := s.workspaceIDFromRequest(r)
	if wsID == "" {
		http.Error(w, "workspace context required", http.StatusBadRequest)
		return
	}
	sessions, err := s.Sessions.ListForWorkspace(wsID)
	if err != nil {
		http.Error(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

// handleSessionSubroutes handles DELETE /api/admin/sessions/{id}
func (s *Server) handleSessionSubroutes(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		http.Error(w, "session store not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/sessions/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	// Support /api/admin/sessions/user/{uid} for revoking all sessions for a user
	if strings.HasPrefix(path, "user/") {
		userID := strings.TrimPrefix(path, "user/")
		if err := s.Sessions.RevokeAllForUser(userID); err != nil {
			log.Printf("session revoke all for user: %v", err)
			http.Error(w, "failed to revoke user sessions", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
		return
	}
	if err := s.Sessions.Revoke(path); err != nil {
		log.Printf("session revoke: %v", err)
		http.Error(w, "failed to revoke session", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
