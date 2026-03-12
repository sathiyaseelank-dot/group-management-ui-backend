package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"controller/state"
)

// handleIdentityProviders handles GET/POST /api/admin/identity-providers
func (s *Server) handleIdentityProviders(w http.ResponseWriter, r *http.Request) {
	if s.IdPs == nil {
		http.Error(w, "identity provider store not configured", http.StatusServiceUnavailable)
		return
	}
	wsID := s.workspaceIDFromRequest(r)
	if wsID == "" {
		http.Error(w, "workspace context required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		idps, err := s.IdPs.ListForWorkspace(wsID)
		if err != nil {
			http.Error(w, "failed to list identity providers", http.StatusInternalServerError)
			return
		}
		// Mask the encrypted secret in the response
		type idpResponse struct {
			ID           string `json:"id"`
			WorkspaceID  string `json:"workspace_id"`
			ProviderType string `json:"provider_type"`
			ClientID     string `json:"client_id"`
			RedirectURI  string `json:"redirect_uri"`
			IssuerURL    string `json:"issuer_url"`
			Enabled      bool   `json:"enabled"`
			CreatedAt    string `json:"created_at"`
			UpdatedAt    string `json:"updated_at"`
		}
		out := make([]idpResponse, 0, len(idps))
		for _, idp := range idps {
			out = append(out, idpResponse{
				ID:           idp.ID,
				WorkspaceID:  idp.WorkspaceID,
				ProviderType: idp.ProviderType,
				ClientID:     idp.ClientID,
				RedirectURI:  idp.RedirectURI,
				IssuerURL:    idp.IssuerURL,
				Enabled:      idp.Enabled,
				CreatedAt:    idp.CreatedAt,
				UpdatedAt:    idp.UpdatedAt,
			})
		}
		writeJSON(w, http.StatusOK, out)

	case http.MethodPost:
		var req struct {
			ProviderType string `json:"provider_type"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RedirectURI  string `json:"redirect_uri"`
			IssuerURL    string `json:"issuer_url"`
			Enabled      bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.ProviderType == "" || req.ClientID == "" || req.ClientSecret == "" {
			http.Error(w, "provider_type, client_id, and client_secret are required", http.StatusBadRequest)
			return
		}

		idp := &state.IdentityProvider{
			WorkspaceID:  wsID,
			ProviderType: req.ProviderType,
			ClientID:     req.ClientID,
			RedirectURI:  req.RedirectURI,
			IssuerURL:    req.IssuerURL,
			Enabled:      req.Enabled,
		}
		if err := s.IdPs.EncryptSecret(idp, req.ClientSecret); err != nil {
			http.Error(w, "failed to encrypt client secret", http.StatusInternalServerError)
			return
		}
		if err := s.IdPs.Create(idp); err != nil {
			http.Error(w, "failed to create identity provider: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": idp.ID, "status": "created"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleIdentityProviderSubroutes handles PUT/DELETE /api/admin/identity-providers/{id}
func (s *Server) handleIdentityProviderSubroutes(w http.ResponseWriter, r *http.Request) {
	if s.IdPs == nil {
		http.Error(w, "identity provider store not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/identity-providers/")
	id = strings.Trim(id, "/")
	if id == "" {
		http.Error(w, "identity provider id required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req struct {
			ProviderType string `json:"provider_type"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RedirectURI  string `json:"redirect_uri"`
			IssuerURL    string `json:"issuer_url"`
			Enabled      bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		idp := &state.IdentityProvider{
			ID:           id,
			ProviderType: req.ProviderType,
			ClientID:     req.ClientID,
			RedirectURI:  req.RedirectURI,
			IssuerURL:    req.IssuerURL,
			Enabled:      req.Enabled,
		}
		// Only re-encrypt if a new secret is provided
		if req.ClientSecret != "" {
			if err := s.IdPs.EncryptSecret(idp, req.ClientSecret); err != nil {
				http.Error(w, "failed to encrypt client secret", http.StatusInternalServerError)
				return
			}
		}
		if err := s.IdPs.Update(idp); err != nil {
			http.Error(w, "failed to update identity provider: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	case http.MethodDelete:
		if err := s.IdPs.Delete(id); err != nil {
			http.Error(w, "failed to delete identity provider: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
