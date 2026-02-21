package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

type Server struct {
	Tokens    *state.TokenStore
	Reg       *state.Registry
	Tunnelers *state.TunnelerStatusRegistry
	ACLs      *state.ACLStore
	ACLNotify ACLNotifier
	Users     *state.UserStore
	RemoteNet *state.RemoteNetworkStore

	AdminAuthToken    string
	InternalAuthToken string
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/api/admin/tokens", s.adminAuth(http.HandlerFunc(s.handleCreateToken)))
	mux.Handle("/api/admin/connectors", s.adminAuth(http.HandlerFunc(s.handleListConnectors)))
	mux.Handle("/api/admin/connectors/", s.adminAuth(http.HandlerFunc(s.handleConnectorSubroutes)))
	mux.Handle("/api/admin/tunnelers", s.adminAuth(http.HandlerFunc(s.handleListTunnelers)))
	mux.Handle("/api/admin/resources", s.adminAuth(http.HandlerFunc(s.handleResources)))
	mux.Handle("/api/admin/resources/", s.adminAuth(http.HandlerFunc(s.handleResourceSubroutes)))
	mux.Handle("/api/admin/audit", s.adminAuth(http.HandlerFunc(s.handleAuditLog)))
	mux.Handle("/api/admin/users", s.adminAuth(http.HandlerFunc(s.handleUsers)))
	mux.Handle("/api/admin/user-groups", s.adminAuth(http.HandlerFunc(s.handleUserGroups)))
	mux.Handle("/api/admin/user-groups/", s.adminAuth(http.HandlerFunc(s.handleUserGroupMembers)))
	mux.Handle("/api/admin/remote-networks", s.adminAuth(http.HandlerFunc(s.handleRemoteNetworks)))
	mux.Handle("/api/admin/remote-networks/", s.adminAuth(http.HandlerFunc(s.handleRemoteNetworkConnectors)))
	mux.Handle("/api/internal/consume-token", s.internalAuth(http.HandlerFunc(s.handleConsumeToken)))
}

type ACLNotifier interface {
	NotifyACLInit()
	NotifyResourceUpsert(res state.Resource)
	NotifyResourceRemoved(resourceID string)
	NotifyAuthorizationUpsert(auth state.Authorization)
	NotifyAuthorizationRemoved(resourceID, principalSPIFFE string)
}

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.AdminAuthToken == "" {
			http.Error(w, "admin auth not configured", http.StatusServiceUnavailable)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+s.AdminAuthToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
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

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token, expires, err := s.Tokens.CreateToken()
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

func (s *Server) handleListTunnelers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Tunnelers == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	records := s.Tunnelers.List()
	now := time.Now().UTC()
	type respTunneler struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		ConnectorID string `json:"connector_id"`
		LastSeen    string `json:"last_seen"`
	}
	resp := make([]respTunneler, 0, len(records))
	for _, rec := range records {
		status := "OFFLINE"
		if now.Sub(rec.LastSeen) < 30*time.Second {
			status = "ONLINE"
		}
		resp = append(resp, respTunneler{
			ID:          rec.ID,
			Status:      status,
			ConnectorID: rec.ConnectorID,
			LastSeen:    humanizeDuration(now.Sub(rec.LastSeen)),
		})
	}
	writeJSON(w, http.StatusOK, resp)
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
