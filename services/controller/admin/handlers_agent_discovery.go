package admin

import (
	"net/http"
	"strconv"
	"strings"

	"controller/state"
)

// handleAgentDiscoveryResults returns or purges services discovered by agents.
// GET /api/admin/agent-discovery/results?agent_id=xxx&include_dismissed=true
// DELETE /api/admin/agent-discovery/results?agent_id=xxx
func (s *Server) handleAgentDiscoveryResults(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.handleAgentDiscoveryPurge(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db() == nil {
		writeJSON(w, http.StatusOK, []state.AgentDiscoveredService{})
		return
	}

	wsID := workspaceIDFromContext(r.Context())
	agentID := r.URL.Query().Get("agent_id")
	includeDismissed := r.URL.Query().Get("include_dismissed") == "true"

	var results []state.AgentDiscoveredService
	var err error
	if agentID != "" {
		if includeDismissed {
			results, err = state.ListAgentDiscoveredServicesAll(s.db(), agentID, wsID)
		} else {
			results, err = state.ListAgentDiscoveredServices(s.db(), agentID, wsID)
		}
	} else {
		if includeDismissed {
			results, err = state.ListAllAgentDiscoveredServicesIncludingDismissed(s.db(), wsID)
		} else {
			results, err = state.ListAllAgentDiscoveredServices(s.db(), wsID)
		}
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []state.AgentDiscoveredService{}
	}
	writeJSON(w, http.StatusOK, results)
}

// handleAgentDiscoverySummary returns aggregate discovery stats.
// GET /api/admin/agent-discovery/summary
func (s *Server) handleAgentDiscoverySummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db() == nil {
		writeJSON(w, http.StatusOK, state.DiscoverySummary{})
		return
	}
	wsID := workspaceIDFromContext(r.Context())
	summary, err := state.GetDiscoverySummary(s.db(), wsID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// handleAgentDiscoveryDismiss handles dismiss/undismiss for a discovered service.
// PATCH /api/admin/agent-discovery/results/{id}/dismiss
// PATCH /api/admin/agent-discovery/results/{id}/undismiss
func (s *Server) handleAgentDiscoveryDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db() == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	// Path: /api/admin/agent-discovery/results/{id}/dismiss or /undismiss
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/agent-discovery/results/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	wsID := workspaceIDFromContext(r.Context())
	action := parts[1]
	switch action {
	case "dismiss":
		err = state.DismissService(s.db(), id, wsID)
	case "undismiss":
		err = state.UndismissService(s.db(), id, wsID)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAgentDiscoveryPurge deletes all discovered services, optionally filtered by agent_id.
// DELETE /api/admin/agent-discovery/results?agent_id=xxx
func (s *Server) handleAgentDiscoveryPurge(w http.ResponseWriter, r *http.Request) {
	if s.db() == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	wsID := workspaceIDFromContext(r.Context())
	agentID := r.URL.Query().Get("agent_id")
	n, err := state.PurgeDiscoveredServices(s.db(), agentID, wsID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "deleted": n})
}
